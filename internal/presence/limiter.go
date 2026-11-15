package presence

import (
	"sync"
	"time"
)

// Limiter is a token bucket: up to burst calls at once, then one more per
// interval. Discord accepts about five presence updates per twenty seconds, so
// a burst of three with a fifteen-second refill stays well inside it while a
// pause or a seek still shows up at once.
type Limiter struct {
	mu       sync.Mutex
	interval time.Duration
	burst    float64
	now      func() time.Time
	tokens   float64
	last     time.Time
}

// NewLimiter returns a limiter allowing burst calls at once and one more per
// interval afterwards. A burst below one is treated as one.
func NewLimiter(interval time.Duration, burst int, now func() time.Time) *Limiter {
	if burst < 1 {
		burst = 1
	}
	return &Limiter{interval: interval, burst: float64(burst), now: now, tokens: float64(burst)}
}

// Allow reports whether a call may proceed now. When it may not, wait is the
// time left until a token is available. Refused calls consume nothing.
func (l *Limiter) Allow() (ok bool, wait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if !l.last.IsZero() {
		l.tokens += float64(now.Sub(l.last)) / float64(l.interval)
		if l.tokens > l.burst {
			l.tokens = l.burst
		}
	}
	l.last = now
	if l.tokens >= 1 {
		l.tokens--
		return true, 0
	}
	wait = time.Duration((1 - l.tokens) * float64(l.interval)).Round(time.Millisecond)
	if wait < time.Millisecond {
		wait = time.Millisecond
	}
	return false, wait
}
