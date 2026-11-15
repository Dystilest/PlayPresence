package session

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)

func playing() Session {
	return Session{
		Source:   "fake",
		Kind:     KindMovie,
		State:    StatePlaying,
		Title:    "Heat",
		Position: 10 * time.Minute,
		Duration: 100 * time.Minute,
	}
}

func TestWindowPlaying(t *testing.T) {
	start, end, ok := playing().Window(t0)

	if !ok {
		t.Fatal("ok = false, want true")
	}
	if want := t0.Add(-10 * time.Minute); !start.Equal(want) {
		t.Errorf("start = %v, want %v", start, want)
	}
	if want := t0.Add(90 * time.Minute); !end.Equal(want) {
		t.Errorf("end = %v, want %v", end, want)
	}
}

func TestWindowPausedOrNoDuration(t *testing.T) {
	paused := playing()
	paused.State = StatePaused
	if _, _, ok := paused.Window(t0); ok {
		t.Error("paused session: ok = true, want false")
	}

	noDuration := playing()
	noDuration.Duration = 0
	if _, _, ok := noDuration.Window(t0); ok {
		t.Error("zero duration: ok = true, want false")
	}
}

func TestWindowClampsPosition(t *testing.T) {
	s := playing()
	s.Position = 150 * time.Minute

	start, end, ok := s.Window(t0)

	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !end.Equal(t0) {
		t.Errorf("end = %v, want now (%v) when position exceeds duration", end, t0)
	}
	if want := t0.Add(-100 * time.Minute); !start.Equal(want) {
		t.Errorf("start = %v, want %v", start, want)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Session)
		wantErr bool
	}{
		{"valid", func(*Session) {}, false},
		{"empty title", func(s *Session) { s.Title = "" }, true},
		{"negative position", func(s *Session) { s.Position = -time.Second }, true},
		{"negative duration", func(s *Session) { s.Duration = -time.Second }, true},
		{"unknown kind", func(s *Session) { s.Kind = "podcast" }, true},
		{"unknown state", func(s *Session) { s.State = "buffering" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := playing()
			tt.mutate(&s)

			err := s.Validate()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
