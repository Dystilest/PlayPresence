package render

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/Dystilest/PlayPresence/internal/session"
)

var now = time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)

func movie() session.Session {
	return session.Session{
		Kind: session.KindMovie, State: session.StatePlaying,
		Title: "Heat", Year: 1995,
		Position: 10 * time.Minute, Duration: 170 * time.Minute,
	}
}

func TestRenderMoviePlaying(t *testing.T) {
	a, ok := Render(movie(), "https://img.example/heat.jpg", now)

	if !ok {
		t.Fatal("ok = false")
	}
	if a.Type != TypeWatching || a.StatusDisplay != StatusDisplayDetails {
		t.Errorf("type/statusDisplay = %d/%d, want %d/%d", a.Type, a.StatusDisplay, TypeWatching, StatusDisplayDetails)
	}
	if a.Details != "Heat (1995)" {
		t.Errorf("details = %q", a.Details)
	}
	if a.LargeImage != "https://img.example/heat.jpg" {
		t.Errorf("large image = %q", a.LargeImage)
	}
	if a.Start == nil || a.End == nil {
		t.Fatal("start/end must be set while playing")
	}
	if want := now.Add(-10 * time.Minute); !a.Start.Equal(want) {
		t.Errorf("start = %v, want %v", *a.Start, want)
	}
	if want := now.Add(160 * time.Minute); !a.End.Equal(want) {
		t.Errorf("end = %v, want %v", *a.End, want)
	}
}

func TestRenderEpisode(t *testing.T) {
	s := session.Session{
		Kind: session.KindEpisode, State: session.StatePlaying,
		Title: "Severance", Season: 1, Episode: 2, EpisodeTitle: "Half Loop",
		Position: time.Minute, Duration: 50 * time.Minute,
	}

	a, _ := Render(s, "", now)

	if a.Details != "Severance" {
		t.Errorf("details = %q", a.Details)
	}
	if a.State != "S01E02 · Half Loop" {
		t.Errorf("state = %q", a.State)
	}

	s.EpisodeTitle = ""
	a, _ = Render(s, "", now)
	if a.State != "S01E02" {
		t.Errorf("state without episode title = %q", a.State)
	}
}

func TestRenderPausedHasNoTimestamps(t *testing.T) {
	s := movie()
	s.State = session.StatePaused
	s.Position = 12*time.Minute + 34*time.Second

	a, ok := Render(s, "", now)

	if !ok {
		t.Fatal("ok = false")
	}
	if a.Start != nil || a.End != nil {
		t.Error("paused activity must not carry timestamps")
	}
	if a.State != "Paused · 12:34" {
		t.Errorf("state = %q", a.State)
	}
}

func TestRenderPausedLongPositionUsesHours(t *testing.T) {
	s := movie()
	s.State = session.StatePaused
	s.Position = time.Hour + 2*time.Minute + 3*time.Second

	a, _ := Render(s, "", now)

	if a.State != "Paused · 1:02:03" {
		t.Errorf("state = %q", a.State)
	}
}

func TestRenderStoppedReturnsNotOK(t *testing.T) {
	s := movie()
	s.State = session.StateStopped

	if _, ok := Render(s, "", now); ok {
		t.Error("stopped session must not render")
	}
}

func TestRenderEmptyTitleFallsBackToUnknown(t *testing.T) {
	s := movie()
	s.Title = ""
	s.Year = 0

	a, _ := Render(s, "", now)

	if a.Details != "Unknown" {
		t.Errorf("details = %q", a.Details)
	}
}

func TestRenderButtons(t *testing.T) {
	tests := []struct {
		name string
		s    session.Session
		want []Button
	}{
		{
			"movie tmdb and imdb",
			withIDs(movie(), session.IDs{TMDB: "949", IMDB: "tt0113277"}),
			[]Button{
				{"TMDb", "https://www.themoviedb.org/movie/949"},
				{"IMDb", "https://www.imdb.com/title/tt0113277/"},
			},
		},
		{
			"episode uses tv path",
			withKind(withIDs(movie(), session.IDs{TMDB: "95396"}), session.KindEpisode),
			[]Button{{"TMDb", "https://www.themoviedb.org/tv/95396"}},
		},
		{
			"imdb only",
			withIDs(movie(), session.IDs{IMDB: "tt0113277"}),
			[]Button{{"IMDb", "https://www.imdb.com/title/tt0113277/"}},
		},
		{
			"no ids no buttons",
			movie(),
			nil,
		},
		{
			"url over 512 chars dropped",
			withIDs(movie(), session.IDs{TMDB: strings.Repeat("9", 600)}),
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := Render(tt.s, "", now)

			if len(a.Buttons) != len(tt.want) {
				t.Fatalf("buttons = %v, want %v", a.Buttons, tt.want)
			}
			for i := range tt.want {
				if a.Buttons[i] != tt.want[i] {
					t.Errorf("button %d = %v, want %v", i, a.Buttons[i], tt.want[i])
				}
			}
		})
	}
}

func TestRenderNeverExceedsTwoButtons(t *testing.T) {
	s := withIDs(movie(), session.IDs{TMDB: "1", IMDB: "tt1", TVDB: "5"})

	a, _ := Render(s, "", now)

	if len(a.Buttons) > 2 {
		t.Errorf("got %d buttons, want at most 2", len(a.Buttons))
	}
}

// units counts UTF-16 code units, the unit JavaScript clients (and likely
// Discord) use for string lengths.
func units(s string) int { return len(utf16.Encode([]rune(s))) }

func TestTruncateStaysWithinTheLimitInUTF16UnitsAndNeverSplitsARune(t *testing.T) {
	inputs := map[string]string{
		"cjk":   strings.Repeat("日本語", 100),
		"emoji": strings.Repeat("🎬", 300), // 2 UTF-16 units each
		"ascii": strings.Repeat("a", 300),
	}
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			got := truncate(in, MaxTextUnits)

			if n := units(got); n > MaxTextUnits || n < MaxTextUnits-1 {
				t.Errorf("UTF-16 length = %d, want %d or %d", n, MaxTextUnits-1, MaxTextUnits)
			}
			if !utf8.ValidString(got) {
				t.Error("result is not valid UTF-8")
			}
			if !strings.HasSuffix(got, "…") {
				t.Error("truncated text must end with an ellipsis")
			}
		})
	}
}

func TestTruncateLeavesShortTextAlone(t *testing.T) {
	if got := truncate("Heat", MaxTextUnits); got != "Heat" {
		t.Errorf("got %q", got)
	}
	exact := strings.Repeat("x", MaxTextUnits)
	if got := truncate(exact, MaxTextUnits); got != exact {
		t.Error("text of exactly the limit must be unchanged")
	}
	emojis := strings.Repeat("🎬", MaxTextUnits/2) // exactly 128 units
	if got := truncate(emojis, MaxTextUnits); got != emojis {
		t.Error("emoji text of exactly the limit in UTF-16 units must be unchanged")
	}
}

func TestRenderTruncatesLongTitle(t *testing.T) {
	s := movie()
	s.Title = strings.Repeat("🎬", 300)

	a, _ := Render(s, "", now)

	if n := units(a.Details); n > MaxTextUnits {
		t.Errorf("details has %d UTF-16 units, want <= %d", n, MaxTextUnits)
	}
}

func withIDs(s session.Session, ids session.IDs) session.Session { s.IDs = ids; return s }
func withKind(s session.Session, k session.Kind) session.Session { s.Kind = k; return s }
