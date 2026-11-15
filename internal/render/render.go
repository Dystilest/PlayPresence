// Package render turns a session into the fields of a Discord activity.
package render

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Dystilest/PlayPresence/internal/session"
)

// Discord activity types and status display modes.
const (
	TypeListening = 2
	TypeWatching  = 3

	StatusDisplayDetails = 2
)

// Discord field limits.
const (
	// MaxTextRunes bounds details and state so they stay on one line.
	MaxTextRunes   = 128
	maxButtons     = 2
	maxButtonLabel = 32
	maxButtonURL   = 512
)

// Button is a Rich Presence button.
type Button struct {
	Label string
	URL   string
}

// Activity holds the Discord activity fields produced from a session.
type Activity struct {
	Type          int
	StatusDisplay int
	Details       string
	State         string
	LargeImage    string
	LargeText     string
	SmallImage    string
	SmallText     string
	Start, End    *time.Time
	Buttons       []Button
}

// Render maps a session to an activity. art is the resolved public image URL
// (may be empty). ok is false when nothing should be displayed (stopped).
func Render(s session.Session, art string, now time.Time) (Activity, bool) {
	if s.State == session.StateStopped {
		return Activity{}, false
	}

	title := s.Title
	if title == "" {
		title = "Unknown"
	}

	a := Activity{
		Type:          TypeWatching,
		StatusDisplay: StatusDisplayDetails,
		LargeImage:    art,
		Buttons:       buttons(s),
	}

	switch s.Kind {
	case session.KindTrack:
		a.Type = TypeListening
		a.Details = title
		a.State = s.Artist
		a.LargeText = s.Album
	case session.KindEpisode:
		a.Details = title
		a.State = episodeLabel(s)
		a.LargeText = title
	default:
		a.Details = title
		if s.Year > 0 {
			a.Details = fmt.Sprintf("%s (%d)", title, s.Year)
		}
		a.LargeText = title
	}

	if s.State == session.StatePaused {
		a.State = "Paused · " + clock(s.Position)
	} else if start, end, ok := s.Window(now); ok {
		a.Start, a.End = &start, &end
	}

	a.Details = truncate(a.Details, MaxTextRunes)
	a.State = truncate(a.State, MaxTextRunes)
	a.LargeText = truncate(a.LargeText, MaxTextRunes)
	return a, true
}

func episodeLabel(s session.Session) string {
	label := fmt.Sprintf("S%02dE%02d", s.Season, s.Episode)
	if s.EpisodeTitle != "" {
		label += " · " + s.EpisodeTitle
	}
	return label
}

func buttons(s session.Session) []Button {
	var out []Button
	add := func(label, url string) {
		if label == "" || url == "" || utf8.RuneCountInString(label) > maxButtonLabel ||
			utf8.RuneCountInString(url) > maxButtonURL || len(out) >= maxButtons {
			return
		}
		out = append(out, Button{Label: label, URL: url})
	}

	switch s.Kind {
	case session.KindTrack:
		if s.IDs.MusicBrainz != "" {
			add("MusicBrainz", "https://musicbrainz.org/recording/"+s.IDs.MusicBrainz)
		}
	default:
		if s.IDs.TMDB != "" {
			path := "movie"
			if s.Kind == session.KindEpisode {
				path = "tv"
			}
			add("TMDb", "https://www.themoviedb.org/"+path+"/"+s.IDs.TMDB)
		}
		if s.IDs.IMDB != "" {
			add("IMDb", "https://www.imdb.com/title/"+s.IDs.IMDB+"/")
		}
	}
	return out
}

// clock formats a position as m:ss, or h:mm:ss from one hour.
func clock(d time.Duration) string {
	total := int(d.Seconds())
	if total < 0 {
		total = 0
	}
	h, m, sec := total/3600, total%3600/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}

// truncate shortens s to at most max runes, ending with an ellipsis, without
// ever splitting a multi-byte character.
func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimRight(string(runes[:max-1]), " ") + "…"
}
