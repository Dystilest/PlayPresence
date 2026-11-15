// Package app wires sources, artwork, rendering and the Discord publisher.
package app

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Dystilest/PlayPresence/internal/presence"
	"github.com/Dystilest/PlayPresence/internal/render"
	"github.com/Dystilest/PlayPresence/internal/session"
	"github.com/Dystilest/PlayPresence/internal/source"
)

const (
	defaultBackoffBase = 2 * time.Second
	defaultBackoffCap  = 30 * time.Second
	artTimeout         = 5 * time.Second
	// stableWindow is how far a progress window may drift between two
	// updates of the same item before it counts as a seek and is re-sent.
	stableWindow = 3 * time.Second
)

// ArtResolver finds a public image URL for a session.
type ArtResolver interface {
	Resolve(ctx context.Context, s session.Session) (string, error)
}

// Updater receives the activity to display; nil clears it.
type Updater interface {
	Update(a *render.Activity)
}

// Orchestrator picks the session to show among all sources and publishes it.
type Orchestrator struct {
	sources []source.Source
	order   []int // source indexes, most important first
	art     ArtResolver
	up      Updater
	clock   presence.Clock

	// OnSourceError, when set before Run, is called from a source's goroutine
	// after each failed Watch (never for context cancellation).
	OnSourceError func(name string, err error)

	backoffBase time.Duration
	backoffCap  time.Duration
}

// NewOrchestrator builds an orchestrator. priority lists source names, most
// important first; sources it does not name follow in alphabetical order.
func NewOrchestrator(sources []source.Source, priority []string, art ArtResolver, up Updater, clock presence.Clock) *Orchestrator {
	return &Orchestrator{
		sources:     sources,
		order:       priorityOrder(sources, priority),
		art:         art,
		up:          up,
		clock:       clock,
		backoffBase: defaultBackoffBase,
		backoffCap:  defaultBackoffCap,
	}
}

func priorityOrder(sources []source.Source, priority []string) []int {
	used := make([]bool, len(sources))
	var order []int
	for _, name := range priority {
		for i, s := range sources {
			if !used[i] && s.Name() == name {
				used[i] = true
				order = append(order, i)
			}
		}
	}
	var rest []int
	for i := range sources {
		if !used[i] {
			rest = append(rest, i)
		}
	}
	sort.SliceStable(rest, func(a, b int) bool { return sources[rest[a]].Name() < sources[rest[b]].Name() })
	return append(order, rest...)
}

type event struct {
	idx int
	s   *session.Session // nil: the source lost its backend, forget its session
}

type identity struct {
	idx            int
	kind           session.Kind
	title          string
	season, number int
}

// Run blocks until ctx is cancelled.
func (o *Orchestrator) Run(ctx context.Context) error {
	events := make(chan event)
	var wg sync.WaitGroup
	for i, src := range o.sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o.runSource(ctx, i, src, events)
		}()
	}
	defer wg.Wait()

	latest := make([]*session.Session, len(o.sources))
	var (
		shown    *render.Activity
		shownID  identity
		artURL   string
		haveShow bool
	)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			if ev.s != nil && ev.s.State != session.StateStopped {
				latest[ev.idx] = ev.s
			} else {
				latest[ev.idx] = nil
			}
		}

		chosen, idx := o.choose(latest)
		if chosen == nil {
			if haveShow {
				o.up.Update(nil)
				shown, haveShow = nil, false
			}
			continue
		}

		id := identity{idx, chosen.Kind, chosen.Title, chosen.Season, chosen.Episode}
		if !haveShow || id != shownID {
			artURL = o.resolveArt(ctx, *chosen)
		}
		act, ok := render.Render(*chosen, artURL, o.clock.Now())
		if !ok {
			continue
		}
		if haveShow && id == shownID && shown != nil && stable(*shown, act) {
			continue
		}
		o.up.Update(&act)
		shown, shownID, haveShow = &act, id, true
	}
}

func (o *Orchestrator) choose(latest []*session.Session) (*session.Session, int) {
	for _, i := range o.order {
		if latest[i] != nil {
			return latest[i], i
		}
	}
	return nil, -1
}

func (o *Orchestrator) resolveArt(ctx context.Context, s session.Session) string {
	if s.ArtURL != "" {
		return s.ArtURL // the source already knows a public image
	}
	if o.art != nil {
		rctx, cancel := context.WithTimeout(ctx, artTimeout)
		defer cancel()
		if url, err := o.art.Resolve(rctx, s); err == nil && url != "" {
			return url
		}
	}
	return placeholder(s.Kind)
}

// placeholder is the Discord application asset used when no artwork is found.
func placeholder(k session.Kind) string {
	switch k {
	case session.KindEpisode:
		return "tv"
	default:
		return "movie"
	}
}

// stable reports whether next only differs from prev by progress drift.
func stable(prev, next render.Activity) bool {
	if prev.Start == nil || prev.End == nil || next.Start == nil || next.End == nil {
		return false
	}
	if abs(next.Start.Sub(*prev.Start)) > stableWindow || abs(next.End.Sub(*prev.End)) > stableWindow {
		return false
	}
	next.Start, next.End = prev.Start, prev.End
	return reflect.DeepEqual(prev, next)
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// runSource keeps one source alive, restarting it with exponential backoff.
func (o *Orchestrator) runSource(ctx context.Context, idx int, src source.Source, events chan<- event) {
	failures := 0
	for ctx.Err() == nil {
		ch := make(chan session.Session)
		var emitted atomic.Bool
		fwdDone := make(chan struct{})
		go func() {
			defer close(fwdDone)
			for s := range ch {
				emitted.Store(true)
				select {
				case events <- event{idx: idx, s: &s}:
				case <-ctx.Done():
				}
			}
		}()

		err := src.Watch(ctx, ch)
		close(ch)
		<-fwdDone
		if ctx.Err() != nil {
			return
		}
		if err != nil && o.OnSourceError != nil {
			o.OnSourceError(src.Name(), err)
		}

		select {
		case events <- event{idx: idx}:
		case <-ctx.Done():
			return
		}
		if emitted.Load() {
			failures = 0
		}
		failures++
		select {
		case <-o.clock.After(o.backoff(failures)):
		case <-ctx.Done():
			return
		}
	}
}

func (o *Orchestrator) backoff(failures int) time.Duration {
	shift := failures - 1
	if shift > 16 {
		shift = 16
	}
	d := o.backoffBase << shift
	if d > o.backoffCap || d <= 0 {
		return o.backoffCap
	}
	return d
}
