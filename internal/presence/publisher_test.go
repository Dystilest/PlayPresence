package presence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/render"
)

type call struct {
	clear   bool
	details string
	at      time.Time
}

// fakeClient records calls and replays queued errors, one per call.
type fakeClient struct {
	mu    sync.Mutex
	calls []call
	errs  []error
}

func (f *fakeClient) record(c call) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c.at = time.Now()
	f.calls = append(f.calls, c)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return err
	}
	return nil
}

func (f *fakeClient) SetActivity(_ context.Context, a render.Activity) error {
	return f.record(call{details: a.Details})
}
func (f *fakeClient) Clear(context.Context) error { return f.record(call{clear: true}) }
func (f *fakeClient) Close() error                { return nil }

func (f *fakeClient) snapshot() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func act(details string) *render.Activity { return &render.Activity{Details: details} }

func start(t *testing.T, p *Publisher) (cancel func(), done <-chan error) {
	t.Helper()
	ctx, cancelCtx := context.WithCancel(context.Background())
	ch := make(chan error, 1)
	go func() { ch <- p.Run(ctx) }()
	t.Cleanup(cancelCtx)
	return cancelCtx, ch
}

func newPub(c Client, interval time.Duration) *Publisher {
	p := NewPublisher(c, interval, RealClock{})
	p.backoffBase = 4 * time.Millisecond
	p.backoffCap = 16 * time.Millisecond
	p.refresh = time.Hour
	return p
}

func TestUpdatesBeforeRunAreCoalescedToTheLast(t *testing.T) {
	f := &fakeClient{}
	p := newPub(f, 40*time.Millisecond)
	for i := 0; i < 10; i++ {
		p.Update(act(string(rune('a' + i))))
	}

	start(t, p)

	eventually(t, "one call", func() bool { return len(f.snapshot()) >= 1 })
	time.Sleep(120 * time.Millisecond)
	calls := f.snapshot()
	if len(calls) != 1 || calls[0].details != "j" {
		t.Fatalf("calls = %+v, want exactly one call with the last value", calls)
	}
}

func TestRapidUpdatesRespectIntervalAndLastWins(t *testing.T) {
	f := &fakeClient{}
	p := newPub(f, 60*time.Millisecond)
	start(t, p)
	p.Update(act("first"))
	eventually(t, "first sent", func() bool { return len(f.snapshot()) == 1 })

	for _, d := range []string{"b1", "b2", "b3", "b4", "b5"} {
		p.Update(act(d))
	}

	eventually(t, "second sent", func() bool { return len(f.snapshot()) == 2 })
	time.Sleep(100 * time.Millisecond)
	calls := f.snapshot()
	if len(calls) != 2 || calls[1].details != "b5" {
		t.Fatalf("calls = %+v, want first then b5 only", calls)
	}
	if gap := calls[1].at.Sub(calls[0].at); gap < 55*time.Millisecond {
		t.Errorf("gap between updates = %v, want >= interval", gap)
	}
}

func TestIdenticalActivityIsNotResent(t *testing.T) {
	f := &fakeClient{}
	p := newPub(f, 10*time.Millisecond)
	start(t, p)
	p.Update(act("same"))
	eventually(t, "sent", func() bool { return len(f.snapshot()) == 1 })

	p.Update(act("same"))
	time.Sleep(60 * time.Millisecond)

	if n := len(f.snapshot()); n != 1 {
		t.Fatalf("calls = %d, want 1", n)
	}
}

func TestNilUpdateClearsAfterASend(t *testing.T) {
	f := &fakeClient{}
	p := newPub(f, 10*time.Millisecond)
	start(t, p)
	p.Update(act("playing"))
	eventually(t, "sent", func() bool { return len(f.snapshot()) == 1 })

	p.Update(nil)

	eventually(t, "clear", func() bool {
		c := f.snapshot()
		return len(c) == 2 && c[1].clear
	})
}

func TestNilUpdateWithNothingSentDoesNothing(t *testing.T) {
	f := &fakeClient{}
	p := newPub(f, 10*time.Millisecond)
	start(t, p)

	p.Update(nil)
	time.Sleep(50 * time.Millisecond)

	if n := len(f.snapshot()); n != 0 {
		t.Fatalf("calls = %d, want 0", n)
	}
}

func TestDiscordNotRunningRetriesWithBackoffThenSends(t *testing.T) {
	f := &fakeClient{errs: []error{ErrNotRunning, ErrNotRunning, ErrNotRunning}}
	p := newPub(f, time.Millisecond)
	begin := time.Now()
	start(t, p)

	p.Update(act("late"))

	eventually(t, "four attempts", func() bool { return len(f.snapshot()) == 4 })
	calls := f.snapshot()
	if calls[3].details != "late" {
		t.Errorf("final call = %+v", calls[3])
	}
	// backoff 4ms + 8ms + 16ms between the four attempts
	if elapsed := calls[3].at.Sub(begin); elapsed < 25*time.Millisecond {
		t.Errorf("retried after %v, want exponential backoff (>= 28ms)", elapsed)
	}
}

func TestCurrentActivityIsResentAfterAConnectionProblem(t *testing.T) {
	// ok, then a failure on the periodic refresh, then recovery: no new Update needed.
	f := &fakeClient{errs: []error{nil, ErrNotRunning, nil}}
	p := newPub(f, time.Millisecond)
	p.refresh = 20 * time.Millisecond
	start(t, p)

	p.Update(act("steady"))

	eventually(t, "three attempts and recovery", func() bool { return len(f.snapshot()) >= 3 })
	for i, c := range f.snapshot()[:3] {
		if c.details != "steady" {
			t.Errorf("call %d = %+v, want the current activity each time", i, c)
		}
	}
}

func TestRejectedActivityIsDroppedAndStaleOneCleared(t *testing.T) {
	f := &fakeClient{errs: []error{nil, &RejectedError{Code: 4000, Message: "bad details"}}}
	p := newPub(f, 10*time.Millisecond)
	start(t, p)
	p.Update(act("A"))
	eventually(t, "A sent", func() bool { return len(f.snapshot()) == 1 })

	p.Update(act("bad"))

	eventually(t, "bad attempted then stale A cleared", func() bool {
		c := f.snapshot()
		return len(c) == 3 && c[1].details == "bad" && c[2].clear
	})
	time.Sleep(80 * time.Millisecond)
	if n := len(f.snapshot()); n != 3 {
		t.Fatalf("calls = %d, want 3: a rejected payload must not be retried forever", n)
	}

	p.Update(act("good"))

	eventually(t, "next different activity goes through", func() bool {
		c := f.snapshot()
		return len(c) == 4 && c[3].details == "good"
	})
}

func TestOnErrorReceivesPublishFailures(t *testing.T) {
	f := &fakeClient{errs: []error{ErrNotRunning, ErrNotRunning}}
	p := newPub(f, time.Millisecond)
	var mu sync.Mutex
	var got []error
	p.OnError = func(err error) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, err)
	}
	start(t, p)

	p.Update(act("x"))

	eventually(t, "two reported errors then a success", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 2 && len(f.snapshot()) == 3
	})
	if !errors.Is(got[0], ErrNotRunning) {
		t.Errorf("first error = %v", got[0])
	}
}

func TestRunStopsOnCancelAndClearsPresence(t *testing.T) {
	f := &fakeClient{}
	p := newPub(f, 10*time.Millisecond)
	cancel, done := start(t, p)
	p.Update(act("playing"))
	eventually(t, "sent", func() bool { return len(f.snapshot()) == 1 })

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	c := f.snapshot()
	if len(c) != 2 || !c[1].clear {
		t.Fatalf("calls = %+v, want a final clear", c)
	}
}
