package presence

import (
	"sync"
	"time"
)

// Limiter enforces a minimum interval between allowed calls.
type Limiter struct {
	mu   sync.Mutex
	min  time.Duration
	now  func() time.Time
	last time.Time
}

// NewLimiter returns a limiter that allows one call per min interval.
func NewLimiter(interval time.Duration, now func() time.Time) *Limiter {
	return &Limiter{min: interval, now: now}
}

// Allow reports whether a call may proceed now. When it may not, wait is the
// time left until the next allowed call. Only allowed calls start the interval.
func (l *Limiter) Allow() (ok bool, wait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if !l.last.IsZero() {
		if elapsed := now.Sub(l.last); elapsed < l.min {
			return false, l.min - elapsed
		}
	}
	l.last = now
	return true, 0
}
