// Package render turns a session into the fields of a Discord activity.
package render

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf16"
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
	// MaxTextUnits bounds details, state and tooltips, counted in UTF-16 code
	// units (the conservative reading of Discord's limit: an emoji counts 2).
	MaxTextUnits   = 128
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

	a.Details = truncate(a.Details, MaxTextUnits)
	a.State = truncate(a.State, MaxTextUnits)
	a.LargeText = truncate(a.LargeText, MaxTextUnits)
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

// utf16Len counts the UTF-16 code units of s.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// truncate shortens s to at most max UTF-16 code units, ending with an
// ellipsis, without ever splitting a character.
func truncate(s string, limit int) string {
	if utf16Len(s) <= limit {
		return s
	}
	budget := limit - 1 // room for the ellipsis (one unit)
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := utf16.RuneLen(r)
		if used+w > budget {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return strings.TrimRight(b.String(), " ") + "…"
}
