package fake

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/session"
)

func TestWatchEmitsScriptThenBlocksUntilCancel(t *testing.T) {
	s1 := session.Session{Kind: session.KindMovie, State: session.StatePlaying, Title: "A"}
	s2 := session.Session{Kind: session.KindMovie, State: session.StateStopped, Title: "A"}
	src := New("demo", []Step{{Session: s1}, {Delay: time.Millisecond, Session: s2}})
	out := make(chan session.Session, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- src.Watch(ctx, out) }()

	for _, want := range []session.State{session.StatePlaying, session.StateStopped} {
		select {
		case got := <-out:
			if got.State != want {
				t.Fatalf("state = %s, want %s", got.State, want)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for a scripted session")
		}
	}
	select {
	case err := <-done:
		t.Fatalf("Watch returned early: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
	if src.Name() != "demo" {
		t.Errorf("Name = %q", src.Name())
	}
}

func TestWatchReturnsScriptedError(t *testing.T) {
	boom := errors.New("boom")
	src := New("demo", []Step{{Err: boom}})

	err := src.Watch(context.Background(), make(chan session.Session, 1))

	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
