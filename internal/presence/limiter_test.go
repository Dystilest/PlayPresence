package presence

import (
	"testing"
	"time"
)

func TestLimiterEnforcesMinimumInterval(t *testing.T) {
	now := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	l := NewLimiter(15*time.Second, func() time.Time { return now })

	if ok, _ := l.Allow(); !ok {
		t.Fatal("first call must be allowed")
	}

	now = now.Add(4 * time.Second)
	ok, wait := l.Allow()
	if ok {
		t.Fatal("second call within the interval must be refused")
	}
	if wait != 11*time.Second {
		t.Errorf("wait = %v, want 11s", wait)
	}

	now = now.Add(11 * time.Second)
	if ok, _ := l.Allow(); !ok {
		t.Fatal("call after the interval must be allowed")
	}
}

func TestLimiterRefusedCallDoesNotResetTheClock(t *testing.T) {
	now := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	l := NewLimiter(15*time.Second, func() time.Time { return now })
	l.Allow()

	now = now.Add(10 * time.Second)
	l.Allow() // refused
	now = now.Add(5 * time.Second)

	if ok, _ := l.Allow(); !ok {
		t.Fatal("interval must be measured from the last allowed call")
	}
}
