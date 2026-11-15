package jellyfin

import (
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/session"
)

type testClock struct{ t time.Time }

func (c *testClock) now() time.Time          { return c.t }
func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func playing(title string, pos time.Duration) *session.Session {
	return &session.Session{
		Source: "home", Kind: session.KindMovie, State: session.StatePlaying,
		Title: title, Position: pos, Duration: 2 * time.Hour,
	}
}

func newTestEmitter() (*emitter, *testClock) {
	c := &testClock{t: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)}
	return newEmitter(c.now, time.Minute), c
}

func TestEmitterSendsFirstSession(t *testing.T) {
	e, _ := newTestEmitter()

	out, ok := e.next(playing("A", 10*time.Second))

	if !ok || out.Title != "A" {
		t.Fatalf("ok=%v out=%+v", ok, out)
	}
}

func TestEmitterSkipsNormalProgress(t *testing.T) {
	e, c := newTestEmitter()
	e.next(playing("A", 10*time.Second))

	c.advance(5 * time.Second)
	if _, ok := e.next(playing("A", 15*time.Second+time.Second)); ok { // 1 s of drift
		t.Error("progress within drift must not be re-sent")
	}
}

func TestEmitterSendsAfterSeek(t *testing.T) {
	e, c := newTestEmitter()
	e.next(playing("A", 10*time.Second))

	c.advance(5 * time.Second)
	if _, ok := e.next(playing("A", 10*time.Minute)); !ok {
		t.Error("seek must be sent")
	}
}

func TestEmitterSendsPauseAndResume(t *testing.T) {
	e, c := newTestEmitter()
	e.next(playing("A", 10*time.Second))

	c.advance(5 * time.Second)
	p := playing("A", 15*time.Second)
	p.State = session.StatePaused
	if _, ok := e.next(p); !ok {
		t.Fatal("pause must be sent")
	}
	c.advance(30 * time.Second)
	if _, ok := e.next(p); ok {
		t.Error("an unchanged pause must not be re-sent")
	}
	if _, ok := e.next(playing("A", 15*time.Second)); !ok {
		t.Error("resume must be sent")
	}
}

func TestEmitterSendsDifferentItem(t *testing.T) {
	e, c := newTestEmitter()
	e.next(playing("A", 10*time.Second))

	c.advance(time.Second)
	if _, ok := e.next(playing("B", 11*time.Second)); !ok {
		t.Error("a different item must be sent")
	}
}

func TestEmitterHeartbeatResendsAfterRefresh(t *testing.T) {
	e, c := newTestEmitter()
	e.next(playing("A", 10*time.Second))

	c.advance(59 * time.Second)
	if _, ok := e.next(playing("A", 69*time.Second)); ok {
		t.Fatal("heartbeat sent too early")
	}
	c.advance(2 * time.Second)
	if _, ok := e.next(playing("A", 71*time.Second)); !ok {
		t.Error("heartbeat not sent after refresh")
	}
}

func TestEmitterStopSentOnceThenNothing(t *testing.T) {
	e, c := newTestEmitter()
	e.next(playing("A", 10*time.Second))

	c.advance(time.Second)
	out, ok := e.next(nil)
	if !ok || out.State != session.StateStopped || out.Title != "A" {
		t.Fatalf("stop: ok=%v out=%+v", ok, out)
	}
	if _, ok := e.next(nil); ok {
		t.Error("stop must be sent exactly once")
	}
	if _, ok := e.next(playing("A", 0)); !ok {
		t.Error("a new session after a stop must be sent")
	}
}

func TestEmitterNilWithoutPriorSessionSendsNothing(t *testing.T) {
	e, _ := newTestEmitter()

	if _, ok := e.next(nil); ok {
		t.Error("nothing was ever sent, nothing to stop")
	}
}

func TestEmitterDoesNotMutateItsInput(t *testing.T) {
	e, _ := newTestEmitter()
	in := playing("A", 10*time.Second)
	e.next(in)

	e.next(nil)

	if in.State != session.StatePlaying {
		t.Errorf("input mutated: %+v", in)
	}
}
