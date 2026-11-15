package presence

import (
	"testing"
	"time"
)

func TestLimiterEnforcesMinimumInterval(t *testing.T) {
	now := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	l := NewLimiter(15*time.Second, 1, func() time.Time { return now })

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
	l := NewLimiter(15*time.Second, 1, func() time.Time { return now })
	l.Allow()

	now = now.Add(10 * time.Second)
	l.Allow() // refused
	now = now.Add(5 * time.Second)

	if ok, _ := l.Allow(); !ok {
		t.Fatal("interval must be measured from the last allowed call")
	}
}

func TestLimiterAllowsABurstThenRefillsOnePerInterval(t *testing.T) {
	now := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	l := NewLimiter(15*time.Second, 3, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow(); !ok {
			t.Fatalf("call %d of the burst refused", i+1)
		}
		now = now.Add(time.Second)
	}
	ok, wait := l.Allow()
	if ok {
		t.Fatal("the fourth call right after the burst must be refused")
	}
	if wait != 12*time.Second {
		t.Errorf("wait = %v, want 12s (one token needs 15s, 3s already passed)", wait)
	}

	now = now.Add(12 * time.Second)
	if ok, _ := l.Allow(); !ok {
		t.Error("a token must have refilled after the wait")
	}
	if ok, _ := l.Allow(); ok {
		t.Error("only one token should have refilled")
	}
}

func TestLimiterNeverExceedsTheBurstAfterALongPause(t *testing.T) {
	now := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	l := NewLimiter(15*time.Second, 3, func() time.Time { return now })
	l.Allow()

	now = now.Add(time.Hour)
	allowed := 0
	for i := 0; i < 10; i++ {
		if ok, _ := l.Allow(); ok {
			allowed++
		}
	}

	if allowed != 3 {
		t.Errorf("allowed %d calls after a long pause, want the burst of 3", allowed)
	}
}
