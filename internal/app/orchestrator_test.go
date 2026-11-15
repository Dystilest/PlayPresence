package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/presence"
	"github.com/Dystilest/PlayPresence/internal/render"
	"github.com/Dystilest/PlayPresence/internal/session"
	"github.com/Dystilest/PlayPresence/internal/source"
)

// recorder implements Updater.
type recorder struct {
	mu   sync.Mutex
	logs []*render.Activity
}

func (r *recorder) Update(a *render.Activity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a != nil {
		cp := *a
		a = &cp
	}
	r.logs = append(r.logs, a)
}

func (r *recorder) all() []*render.Activity {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*render.Activity(nil), r.logs...)
}

func (r *recorder) last() *render.Activity {
	l := r.all()
	if len(l) == 0 {
		return nil
	}
	return l[len(l)-1]
}

type fakeArt struct {
	url string
	err error
	n   atomic.Int32
}

func (f *fakeArt) Resolve(context.Context, session.Session) (string, error) {
	f.n.Add(1)
	return f.url, f.err
}

// feed is a source whose sessions are pushed by the test.
type feed struct {
	name string
	in   chan session.Session
	runs atomic.Int32
	fail chan error // if non-nil, a value makes Watch return that error
}

func newFeed(name string) *feed {
	return &feed{name: name, in: make(chan session.Session, 16), fail: make(chan error, 1)}
}

func (f *feed) Name() string { return f.name }
func (f *feed) Watch(ctx context.Context, out chan<- session.Session) error {
	f.runs.Add(1)
	for {
		select {
		case s := <-f.in:
			select {
			case out <- s:
			case <-ctx.Done():
				return ctx.Err()
			}
		case err := <-f.fail:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func movie(title string, state session.State, pos time.Duration) session.Session {
	return session.Session{
		Kind: session.KindMovie, State: state, Title: title, Year: 1995,
		Position: pos, Duration: 170 * time.Minute,
	}
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

func run(t *testing.T, o *Orchestrator) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = o.Run(ctx) }()
}

func orch(srcs []source.Source, prio []string, art ArtResolver, up Updater) *Orchestrator {
	o := NewOrchestrator(srcs, prio, art, up, presence.RealClock{})
	o.backoffBase = 3 * time.Millisecond
	o.backoffCap = 10 * time.Millisecond
	return o
}

func TestPlayingSessionIsRenderedWithResolvedArt(t *testing.T) {
	f, up, art := newFeed("home"), &recorder{}, &fakeArt{url: "https://img.example/heat.jpg"}
	run(t, orch([]source.Source{f}, nil, art, up))

	f.in <- movie("Heat", session.StatePlaying, 10*time.Minute)

	eventually(t, "an update", func() bool { return up.last() != nil })
	a := up.last()
	if a.Details != "Heat (1995)" || a.LargeImage != "https://img.example/heat.jpg" {
		t.Errorf("activity = %+v", a)
	}
	if a.Start == nil {
		t.Error("playing activity must carry a progress window")
	}
}

func TestPausedSessionHasNoTimestamps(t *testing.T) {
	f, up := newFeed("home"), &recorder{}
	run(t, orch([]source.Source{f}, nil, &fakeArt{}, up))

	f.in <- movie("Heat", session.StatePaused, 5*time.Minute)

	eventually(t, "an update", func() bool { return up.last() != nil })
	if a := up.last(); a.Start != nil || a.State != "Paused · 05:00" {
		t.Errorf("activity = %+v", a)
	}
}

func TestStoppedSessionClearsPresence(t *testing.T) {
	f, up := newFeed("home"), &recorder{}
	run(t, orch([]source.Source{f}, nil, &fakeArt{}, up))
	f.in <- movie("Heat", session.StatePlaying, time.Minute)
	eventually(t, "playing update", func() bool { return up.last() != nil })

	f.in <- movie("Heat", session.StateStopped, time.Minute)

	eventually(t, "clear", func() bool { l := up.all(); return len(l) >= 2 && l[len(l)-1] == nil })
}

func TestHigherPrioritySourceWinsAndOtherTakesOverOnStop(t *testing.T) {
	a, b, up := newFeed("a-server"), newFeed("b-server"), &recorder{}
	run(t, orch([]source.Source{a, b}, []string{"b-server", "a-server"}, &fakeArt{}, up))

	a.in <- movie("From A", session.StatePlaying, time.Minute)
	eventually(t, "A shown", func() bool { l := up.last(); return l != nil && l.Details == "From A (1995)" })
	b.in <- movie("From B", session.StatePlaying, time.Minute)
	eventually(t, "B wins", func() bool { l := up.last(); return l != nil && l.Details == "From B (1995)" })

	b.in <- movie("From B", session.StateStopped, 0)

	eventually(t, "A takes over", func() bool { l := up.last(); return l != nil && l.Details == "From A (1995)" })
}

func TestDefaultPriorityIsAlphabetical(t *testing.T) {
	z, a, up := newFeed("zeta"), newFeed("alpha"), &recorder{}
	run(t, orch([]source.Source{z, a}, nil, &fakeArt{}, up))

	z.in <- movie("Z", session.StatePlaying, time.Minute)
	a.in <- movie("A", session.StatePlaying, time.Minute)

	eventually(t, "alpha wins", func() bool { l := up.last(); return l != nil && l.Details == "A (1995)" })
}

func TestFailingSourceIsRestartedAndDoesNotAffectOthers(t *testing.T) {
	bad, good, up := newFeed("bad"), newFeed("good"), &recorder{}
	run(t, orch([]source.Source{bad, good}, nil, &fakeArt{}, up))
	good.in <- movie("Steady", session.StatePlaying, time.Minute)
	eventually(t, "good shown", func() bool { return up.last() != nil })

	bad.fail <- errors.New("connection refused")

	eventually(t, "bad restarted", func() bool { return bad.runs.Load() >= 2 })
	if a := up.last(); a == nil || a.Details != "Steady (1995)" {
		t.Errorf("good source must keep displaying, got %+v", a)
	}
}

func TestSessionOfAFailedSourceIsCleared(t *testing.T) {
	f, up := newFeed("home"), &recorder{}
	run(t, orch([]source.Source{f}, nil, &fakeArt{}, up))
	f.in <- movie("Heat", session.StatePlaying, time.Minute)
	eventually(t, "shown", func() bool { return up.last() != nil })

	f.fail <- errors.New("server went away")

	eventually(t, "cleared", func() bool { l := up.all(); return l[len(l)-1] == nil })
}

func TestArtFailureFallsBackToPlaceholder(t *testing.T) {
	f, up := newFeed("home"), &recorder{}
	run(t, orch([]source.Source{f}, nil, &fakeArt{err: errors.New("lookup failed")}, up))

	f.in <- movie("Heat", session.StatePlaying, time.Minute)

	eventually(t, "an update", func() bool { return up.last() != nil })
	if a := up.last(); a.LargeImage != "movie" {
		t.Errorf("large image = %q, want the movie placeholder", a.LargeImage)
	}
}

func TestPositionJitterDoesNotCauseNewUpdatesButSeeksDo(t *testing.T) {
	f, up, art := newFeed("home"), &recorder{}, &fakeArt{url: "u"}
	run(t, orch([]source.Source{f}, nil, art, up))
	f.in <- movie("Heat", session.StatePlaying, 10*time.Minute)
	eventually(t, "first update", func() bool { return len(up.all()) == 1 })

	// same pace: position advances with wall clock, within a couple of seconds
	time.Sleep(20 * time.Millisecond)
	f.in <- movie("Heat", session.StatePlaying, 10*time.Minute+20*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	if n := len(up.all()); n != 1 {
		t.Fatalf("updates after jitter = %d, want 1", n)
	}
	if n := art.n.Load(); n != 1 {
		t.Errorf("art resolved %d times, want 1 for the same item", n)
	}

	// seek by 30 minutes
	f.in <- movie("Heat", session.StatePlaying, 40*time.Minute)
	eventually(t, "update after seek", func() bool { return len(up.all()) == 2 })
}
