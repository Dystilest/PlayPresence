// Package session defines the source-agnostic description of what is playing.
package session

import (
	"errors"
	"fmt"
	"time"
)

// Kind is the type of media being played.
type Kind string

// Supported media kinds.
const (
	KindMovie   Kind = "movie"
	KindEpisode Kind = "episode"
	KindTrack   Kind = "track"
)

// State is the playback state.
type State string

// Supported playback states.
const (
	StatePlaying State = "playing"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
)

// IDs holds external identifiers used for links and artwork lookups.
type IDs struct {
	TMDB        string
	IMDB        string
	TVDB        string
	MusicBrainz string
}

// Session is a render-ready snapshot of one playback on one media server.
type Session struct {
	Source       string
	Kind         Kind
	State        State
	Title        string // movie title, series name or track title
	Year         int
	Season       int
	Episode      int
	EpisodeTitle string
	Artist       string
	Album        string
	IDs          IDs
	Position     time.Duration
	Duration     time.Duration
	ArtURL       string
	User         string
}

// Validate reports whether the session is well formed.
func (s Session) Validate() error {
	switch s.Kind {
	case KindMovie, KindEpisode, KindTrack:
	default:
		return fmt.Errorf("session: unknown kind %q", s.Kind)
	}
	switch s.State {
	case StatePlaying, StatePaused, StateStopped:
	default:
		return fmt.Errorf("session: unknown state %q", s.State)
	}
	if s.Title == "" {
		return errors.New("session: empty title")
	}
	if s.Position < 0 {
		return errors.New("session: negative position")
	}
	if s.Duration < 0 {
		return errors.New("session: negative duration")
	}
	return nil
}

// Window returns the start and end instants of the playback relative to now,
// suitable for a Discord progress bar. ok is false when the session is not
// playing or has no known duration. A position beyond the duration is clamped.
func (s Session) Window(now time.Time) (start, end time.Time, ok bool) {
	if s.State != StatePlaying || s.Duration <= 0 {
		return time.Time{}, time.Time{}, false
	}
	pos := s.Position
	if pos > s.Duration {
		pos = s.Duration
	}
	start = now.Add(-pos)
	end = start.Add(s.Duration)
	return start, end, true
}
