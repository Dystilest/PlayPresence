package presence

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Dystilest/PlayPresence/internal/render"
)

// Clock abstracts time so callers can control it.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// RealClock is the wall clock.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// After waits for d to elapse.
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

const (
	defaultBackoffBase = 2 * time.Second
	defaultBackoffCap  = 30 * time.Second
	// defaultRefresh re-sends the current activity so it reappears after
	// Discord restarts; Discord drops the activity with the connection.
	defaultRefresh = 60 * time.Second
	clearTimeout   = 2 * time.Second
)

// Publisher sends the latest desired activity to a Client: updates are
// coalesced (last wins), rate limited, retried with backoff while Discord is
// unreachable, and the activity is cleared on shutdown.
type Publisher struct {
	c     Client
	lim   *Limiter
	clock Clock

	backoffBase time.Duration
	backoffCap  time.Duration
	refresh     time.Duration

	wake chan struct{}

	mu       sync.Mutex
	desired  *render.Activity // nil means "no activity"
	lastSent *render.Activity // nil means "nothing displayed"
	dirty    bool
}

// NewPublisher builds a publisher that sends at most one update per minInterval.
func NewPublisher(c Client, minInterval time.Duration, clock Clock) *Publisher {
	return &Publisher{
		c:           c,
		lim:         NewLimiter(minInterval, clock.Now),
		clock:       clock,
		backoffBase: defaultBackoffBase,
		backoffCap:  defaultBackoffCap,
		refresh:     defaultRefresh,
		wake:        make(chan struct{}, 1),
	}
}

// Update sets the desired activity; nil clears it. It never blocks.
func (p *Publisher) Update(a *render.Activity) {
	p.mu.Lock()
	if a != nil {
		cp := *a
		a = &cp
	}
	p.desired = a
	p.dirty = !sameActivity(p.desired, p.lastSent)
	dirty := p.dirty
	p.mu.Unlock()

	if dirty {
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
}

// Run publishes until ctx is cancelled, then clears the presence.
func (p *Publisher) Run(ctx context.Context) error {
	failures := 0
	for {
		p.mu.Lock()
		dirty, hasSent := p.dirty, p.lastSent != nil
		p.mu.Unlock()

		if !dirty {
			var refresh <-chan time.Time
			if hasSent {
				refresh = p.clock.After(p.refresh)
			}
			select {
			case <-ctx.Done():
				return p.shutdown(ctx)
			case <-p.wake:
			case <-refresh:
				p.mu.Lock()
				p.dirty = p.desired != nil
				p.mu.Unlock()
			}
			continue
		}

		if ok, wait := p.lim.Allow(); !ok {
			select {
			case <-ctx.Done():
				return p.shutdown(ctx)
			case <-p.clock.After(wait):
			case <-p.wake:
			}
			continue
		}

		p.mu.Lock()
		target := p.desired
		p.mu.Unlock()

		if err := p.publish(ctx, target); err != nil {
			failures++
			select {
			case <-ctx.Done():
				return p.shutdown(ctx)
			case <-p.clock.After(p.backoff(failures)):
			}
			continue
		}
		failures = 0

		p.mu.Lock()
		p.lastSent = target
		p.dirty = !sameActivity(p.desired, p.lastSent)
		p.mu.Unlock()
	}
}

func (p *Publisher) publish(ctx context.Context, a *render.Activity) error {
	if a == nil {
		return p.c.Clear(ctx)
	}
	return p.c.SetActivity(ctx, *a)
}

func (p *Publisher) backoff(failures int) time.Duration {
	shift := failures - 1
	if shift > 16 {
		shift = 16
	}
	d := p.backoffBase << shift
	if d > p.backoffCap || d <= 0 {
		return p.backoffCap
	}
	return d
}

func (p *Publisher) shutdown(ctx context.Context) error {
	p.mu.Lock()
	hasSent := p.lastSent != nil
	p.mu.Unlock()
	if hasSent {
		clearCtx, cancel := context.WithTimeout(context.Background(), clearTimeout)
		defer cancel()
		_ = p.c.Clear(clearCtx)
	}
	return ctx.Err()
}

func sameActivity(a, b *render.Activity) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(*a, *b)
}
