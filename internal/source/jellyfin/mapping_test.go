package jellyfin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/session"
)

// Payloads below are written by hand from the shape of the Jellyfin API; they
// are not recorded from a real server.

func decode(t *testing.T, js string) rawSession {
	t.Helper()
	var rs rawSession
	if err := json.Unmarshal([]byte(js), &rs); err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	return rs
}

const movieJSON = `{"Id":"s1","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
 "NowPlayingItem":{"Id":"m1","Name":"Amélie","Type":"Movie","ProductionYear":2001,"RunTimeTicks":7320000000,
  "ProviderIds":{"Tmdb":"194","Imdb":"tt0211915"}},
 "PlayState":{"PositionTicks":600000000,"IsPaused":false}}`

const episodeJSON = `{"Id":"s2","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
 "NowPlayingItem":{"Id":"e1","Name":"Pilot","Type":"Episode","SeriesName":"Severance","SeriesId":"ser1",
  "ParentIndexNumber":1,"IndexNumber":2,"ProductionYear":2022,"RunTimeTicks":30000000000,
  "ProviderIds":{"Tmdb":"EPISODE-ID","Tvdb":"EPISODE-TVDB"}},
 "PlayState":{"PositionTicks":0,"IsPaused":true}}`

const trackJSON = `{"Id":"s3","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
 "NowPlayingItem":{"Id":"t1","Name":"Teardrop","Type":"Audio","Artists":["Massive Attack"],"Album":"Mezzanine",
  "RunTimeTicks":3300000000,"ProviderIds":{"MusicBrainzRecording":"rec-1","MusicBrainzTrack":"trk-1"}},
 "PlayState":{"PositionTicks":100000000}}`

func TestToSessionMovie(t *testing.T) {
	s := toSession(decode(t, movieJSON), nil, "home")

	want := session.Session{
		Source: "home", Kind: session.KindMovie, State: session.StatePlaying, Title: "Amélie", Year: 2001,
		IDs:      session.IDs{TMDB: "194", IMDB: "tt0211915"},
		Position: 60 * time.Second, Duration: 732 * time.Second, User: "Samy",
	}
	if s != want {
		t.Errorf("got  %+v\nwant %+v", s, want)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestToSessionEpisodeUsesSeriesIDsNeverTheEpisodes(t *testing.T) {
	series := map[string]string{"Tmdb": "95396", "Imdb": "tt11280740"}

	s := toSession(decode(t, episodeJSON), series, "home")

	if s.Kind != session.KindEpisode || s.Title != "Severance" || s.Season != 1 || s.Episode != 2 || s.EpisodeTitle != "Pilot" {
		t.Errorf("episode fields = %+v", s)
	}
	if s.State != session.StatePaused {
		t.Errorf("state = %q, want paused", s.State)
	}
	if s.IDs.TMDB != "95396" || s.IDs.IMDB != "tt11280740" {
		t.Errorf("ids = %+v, want the series ids", s.IDs)
	}
	if s.IDs.TVDB != "" {
		t.Errorf("episode TVDB id leaked: %q", s.IDs.TVDB)
	}
}

func TestToSessionEpisodeWithoutSeriesIDsHasNoIDs(t *testing.T) {
	s := toSession(decode(t, episodeJSON), map[string]string{}, "home")

	if s.IDs != (session.IDs{}) {
		t.Errorf("ids = %+v, want none (the episode's own ids must not be used)", s.IDs)
	}
}

func TestMusicIsOutOfScopeAndIgnored(t *testing.T) {
	if _, ok := pick([]rawSession{decode(t, trackJSON)}, "", ""); ok {
		t.Error("an audio item must not be picked: only movies and episodes are shown")
	}
}

func TestToSessionClampsAndHandlesMissingRuntime(t *testing.T) {
	cases := []struct {
		name           string
		pos, run       int64
		wantPos, wantD time.Duration
	}{
		{"no runtime", 5e8, 0, 50 * time.Second / 10 * 10, 0},
		{"position beyond runtime", 9e9, 6e9, 600 * time.Second, 600 * time.Second},
		{"negative position", -5, 6e9, 0, 600 * time.Second},
		{"negative runtime", 100, -7, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rs := decode(t, movieJSON)
			rs.PlayState.PositionTicks, rs.NowPlayingItem.RunTimeTicks = c.pos, c.run

			s := toSession(rs, nil, "home")

			if s.Duration != c.wantD {
				t.Errorf("Duration = %v, want %v", s.Duration, c.wantD)
			}
			if c.run > 0 && s.Position != c.wantPos {
				t.Errorf("Position = %v, want %v", s.Position, c.wantPos)
			}
			if s.Position < 0 || (s.Duration > 0 && s.Position > s.Duration) {
				t.Errorf("Position %v outside [0, %v]", s.Position, s.Duration)
			}
		})
	}
}

func TestToSessionMissingPlayState(t *testing.T) {
	rs := decode(t, movieJSON)
	rs.PlayState = nil

	s := toSession(rs, nil, "home")

	if s.State != session.StatePlaying || s.Position != 0 {
		t.Errorf("got %+v", s)
	}
}

func TestPickIgnoresUnsupportedTypesAndIdleSessions(t *testing.T) {
	book := decode(t, movieJSON)
	book.NowPlayingItem.Type = "Book"
	mv := decode(t, movieJSON)
	mv.NowPlayingItem.Type = "MusicVideo"
	idle := decode(t, movieJSON)
	idle.NowPlayingItem = nil

	if _, ok := pick([]rawSession{book, mv, idle}, "", ""); ok {
		t.Error("pick returned a session for unsupported or idle input")
	}
	if _, ok := pick(nil, "", ""); ok {
		t.Error("pick returned a session for no input")
	}
}

func TestPickMostRecentlyActiveDeviceWins(t *testing.T) {
	old := decode(t, movieJSON)
	old.ID, old.LastActivityDate = "old", "2026-11-15T09:00:00Z"
	recent := decode(t, episodeJSON)
	recent.ID, recent.LastActivityDate = "recent", "2026-11-15T10:30:00.1234567Z"

	got, ok := pick([]rawSession{recent, old}, "", "")
	if !ok || got.ID != "recent" {
		t.Fatalf("got %q ok=%v, want recent", got.ID, ok)
	}
	// Stopping the recent device keeps showing the other one.
	got, ok = pick([]rawSession{old}, "", "")
	if !ok || got.ID != "old" {
		t.Fatalf("got %q ok=%v, want old", got.ID, ok)
	}
}

func TestPickUnparsableDateLosesToParsable(t *testing.T) {
	bad := decode(t, movieJSON)
	bad.ID, bad.LastActivityDate = "bad", "not a date"
	good := decode(t, movieJSON)
	good.ID, good.LastActivityDate = "good", "2026-01-01T00:00:00Z"

	got, _ := pick([]rawSession{bad, good}, "", "")

	if got.ID != "good" {
		t.Errorf("got %q, want good", got.ID)
	}
}

func TestPickUserFilterIsCaseInsensitive(t *testing.T) {
	mine := decode(t, movieJSON)
	other := decode(t, episodeJSON)
	other.UserName = "Alice"

	got, ok := pick([]rawSession{other, mine}, "sAmY", "")
	if !ok || got.UserName != "Samy" {
		t.Fatalf("got %q ok=%v", got.UserName, ok)
	}
	if _, ok := pick([]rawSession{other, mine}, "nobody", ""); ok {
		t.Error("filter matched nobody but pick returned a session")
	}
	if _, ok := pick([]rawSession{other, mine}, "", ""); !ok {
		t.Error("empty filter must accept every visible session")
	}
}

func TestNeedsSeries(t *testing.T) {
	if id, ok := needsSeries(decode(t, episodeJSON)); !ok || id != "ser1" {
		t.Errorf("episode: id=%q ok=%v", id, ok)
	}
	noSeries := decode(t, episodeJSON)
	noSeries.NowPlayingItem.SeriesID = ""
	if _, ok := needsSeries(noSeries); ok {
		t.Error("episode without SeriesId must not need a lookup")
	}
	if _, ok := needsSeries(decode(t, movieJSON)); ok {
		t.Error("movie must not need a series lookup")
	}
}

func TestPickKeepsTheDisplayedSessionWhileItStillPlays(t *testing.T) {
	a := decode(t, movieJSON)
	a.ID, a.LastActivityDate = "a", "2026-11-15T10:00:00Z"
	b := decode(t, episodeJSON)
	b.ID, b.LastActivityDate = "b", "2026-11-15T10:00:05Z" // more recent

	got, _ := pick([]rawSession{a, b}, "", "a")
	if got.ID != "a" {
		t.Errorf("got %q, want the sticky session a", got.ID)
	}
	got, _ = pick([]rawSession{b}, "", "a") // a is gone: re-pick
	if got.ID != "b" {
		t.Errorf("got %q, want b after a disappeared", got.ID)
	}
	if got, _ = pick([]rawSession{a, b}, "nobody", "a"); got.ID != "" {
		t.Error("the user filter must beat stickiness")
	}
}
