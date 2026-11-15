package jellyfin

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/session"
)

const (
	playingMovie = `[{"Id":"s1","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
	 "NowPlayingItem":{"Id":"m1","Name":"Amélie","Type":"Movie","ProductionYear":2001,"RunTimeTicks":7320000000},
	 "PlayState":{"PositionTicks":600000000,"IsPaused":false}}]`
	pausedMovie = `[{"Id":"s1","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
	 "NowPlayingItem":{"Id":"m1","Name":"Amélie","Type":"Movie","ProductionYear":2001,"RunTimeTicks":7320000000},
	 "PlayState":{"PositionTicks":600000000,"IsPaused":true}}]`
	playingEpisode = `[{"Id":"s2","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
	 "NowPlayingItem":{"Id":"e1","Name":"Pilot","Type":"Episode","SeriesName":"Severance","SeriesId":"ser1",
	  "ParentIndexNumber":1,"IndexNumber":2,"RunTimeTicks":30000000000},
	 "PlayState":{"PositionTicks":0}}]`
	twoDevices = `[
	 {"Id":"old","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T09:00:00Z",
	  "NowPlayingItem":{"Id":"a","Name":"Old Movie","Type":"Movie","RunTimeTicks":7200000000},"PlayState":{"PositionTicks":0}},
	 {"Id":"new","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
	  "NowPlayingItem":{"Id":"b","Name":"New Movie","Type":"Movie","RunTimeTicks":7200000000},"PlayState":{"PositionTicks":0}}]`
	oneDeviceStopped = `[
	 {"Id":"old","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T09:00:00Z",
	  "NowPlayingItem":{"Id":"a","Name":"Old Movie","Type":"Movie","RunTimeTicks":7200000000},"PlayState":{"PositionTicks":0}},
	 {"Id":"new","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:05:00Z"}]`
)

// run starts the source and returns its output channel and a way to wait for
// Watch's result. The watcher is cancelled when the test ends.
func run(t *testing.T, f *fakeServer, token string, opt Options) (<-chan session.Session, <-chan error, context.CancelFunc) {
	t.Helper()
	if opt.Name == "" {
		opt.Name = "home"
	}
	if opt.PollInterval == 0 {
		opt.PollInterval = 20 * time.Millisecond
	}
	src := New(client(t, f, token), opt)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	out := make(chan session.Session)
	done := make(chan error, 1)
	go func() { done <- src.Watch(ctx, out) }()
	return out, done, cancel
}

func recv(t *testing.T, out <-chan session.Session) session.Session {
	t.Helper()
	select {
	case s := <-out:
		return s
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a session")
		return session.Session{}
	}
}

func expectNothing(t *testing.T, out <-chan session.Session) {
	t.Helper()
	select {
	case s := <-out:
		t.Fatalf("unexpected session: %+v", s)
	case <-time.After(150 * time.Millisecond):
	}
}

func waitDone(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("Watch did not return")
		return nil
	}
}

func TestPlayingEmitsASession(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(playingMovie)

	out, _, _ := run(t, f, "good-token", Options{})

	s := recv(t, out)
	if s.Source != "home" || s.Title != "Amélie" || s.State != session.StatePlaying || s.Position != 60*time.Second {
		t.Errorf("got %+v", s)
	}
}

func TestSourceName(t *testing.T) {
	f := newFakeServer(t, "")
	if got := New(client(t, f, "x"), Options{Name: "home"}).Name(); got != "home" {
		t.Errorf("Name() = %q", got)
	}
}

func TestStartsSessionsWithTheJellyfinHandshake(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(playingMovie)
	out, _, _ := run(t, f, "good-token", Options{})
	recv(t, out)

	got := strings.Join(f.received(), "|")

	if !strings.Contains(got, `"MessageType":"SessionsStart"`) || !strings.Contains(got, `"Data":"0,1500"`) {
		t.Errorf("client frames = %s", got)
	}
}

func TestPauseThenStopEmitsPausedThenStopped(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(playingMovie)
	out, _, _ := run(t, f, "good-token", Options{})
	recv(t, out)

	f.setSessions(pausedMovie)
	f.push()
	if s := recv(t, out); s.State != session.StatePaused {
		t.Fatalf("want paused, got %+v", s)
	}

	f.setSessions(`[]`)
	f.push()
	if s := recv(t, out); s.State != session.StateStopped || s.Title != "Amélie" {
		t.Fatalf("want stopped, got %+v", s)
	}
	f.push()
	expectNothing(t, out)
}

func TestStopWhileAnotherDevicePlaysKeepsTheOther(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(twoDevices)
	out, _, _ := run(t, f, "good-token", Options{})
	if s := recv(t, out); s.Title != "New Movie" {
		t.Fatalf("most recent device must win, got %+v", s)
	}

	f.setSessions(oneDeviceStopped)
	f.push()

	s := recv(t, out)
	if s.Title != "Old Movie" || s.State != session.StatePlaying {
		t.Errorf("want the other device playing, got %+v", s)
	}
}

func TestEpisodeUsesSeriesIDsAndTheyAreCached(t *testing.T) {
	f := newFakeServer(t, "")
	f.series["ser1"] = map[string]string{"Tmdb": "95396"}
	f.setSessions(playingEpisode)
	out, _, _ := run(t, f, "good-token", Options{})
	if s := recv(t, out); s.IDs.TMDB != "95396" || s.Title != "Severance" {
		t.Fatalf("got %+v", s)
	}

	f.setSessions(strings.Replace(playingEpisode, `"PositionTicks":0`, `"PositionTicks":0,"IsPaused":true`, 1))
	f.push()
	recv(t, out)

	if n := f.count("/Items"); n != 1 {
		t.Errorf("series looked up %d times, want 1 (cached)", n)
	}
}

func TestSeriesLookupFailureStillEmitsEpisodeWithoutTmdbID(t *testing.T) {
	f := newFakeServer(t, "")
	f.itemsStatus = 500
	f.setSessions(playingEpisode)

	out, _, _ := run(t, f, "good-token", Options{})

	s := recv(t, out)
	if s.Title != "Severance" || s.Season != 1 || s.IDs.TMDB != "" {
		t.Errorf("got %+v", s)
	}
}

func TestWebSocketRefusedFallsBackToPolling(t *testing.T) {
	f := newFakeServer(t, "")
	f.setWSRefused(true)
	f.setSessions(playingMovie)

	out, _, _ := run(t, f, "good-token", Options{})

	if s := recv(t, out); s.Title != "Amélie" {
		t.Fatalf("got %+v", s)
	}
	f.setSessions(`[]`)
	if s := recv(t, out); s.State != session.StateStopped {
		t.Fatalf("polling must also report the stop, got %+v", s)
	}
}

func TestUnauthorizedOnUpgradeReturnsErrUnauthorized(t *testing.T) {
	f := newFakeServer(t, "")

	_, done, _ := run(t, f, "wrong-token", Options{})

	if err := waitDone(t, done); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestTokenRevokedWhilePollingReturnsErrUnauthorized(t *testing.T) {
	f := newFakeServer(t, "")
	f.setWSRefused(true)
	f.setSessions(playingMovie)
	out, done, _ := run(t, f, "good-token", Options{})
	recv(t, out)

	f.setToken("rotated")

	if err := waitDone(t, done); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestSocketDroppedReturnsAnError(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(playingMovie)
	out, done, _ := run(t, f, "good-token", Options{})
	recv(t, out)

	f.dropSockets()

	err := waitDone(t, done)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want a connection error", err)
	}
}

func TestServerUnreachableReturnsAnErrorWithoutLeakingTheAddress(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	c, _ := NewClient("http://"+addr, testDevice, "tok", nil)
	src := New(c, Options{Name: "home", PollInterval: 20 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := src.Watch(ctx, make(chan session.Session))

	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), addr) || strings.Contains(err.Error(), "tok") && strings.Contains(err.Error(), "Token") {
		t.Errorf("error leaks connection details: %v", err)
	}
}

func TestContextCancelStopsPromptlyEvenWhileBlockedOnOut(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(playingMovie)
	_, done, cancel := run(t, f, "good-token", Options{}) // nobody reads out
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	cancel()

	_ = waitDone(t, done)
	if time.Since(start) > time.Second {
		t.Errorf("took %v to stop", time.Since(start))
	}
}

func TestForceKeepAliveIsAnswered(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(playingMovie)
	out, _, _ := run(t, f, "good-token", Options{})
	recv(t, out)

	f.sendRaw(`{"MessageType":"ForceKeepAlive","Data":60}`)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(strings.Join(f.received(), "|"), `"MessageType":"KeepAlive"`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("no KeepAlive answer; client frames = %v", f.received())
}

func TestMalformedFramesAreIgnored(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(playingMovie)
	out, done, _ := run(t, f, "good-token", Options{})
	recv(t, out)

	f.sendRaw(`not json`)
	f.sendRaw(`{"MessageType":"Sessions","Data":"oops"}`)
	f.sendRaw(`{"MessageType":"UserDataChanged","Data":{}}`)

	select {
	case err := <-done:
		t.Fatalf("Watch returned on malformed input: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestTokenNeverInURL(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(playingEpisode)
	out, _, _ := run(t, f, "good-token", Options{})
	recv(t, out)
	f.setWSRefused(true)
	f.dropSockets()

	for _, r := range f.seen() {
		if strings.Contains(r.RawQuery, "api_key") || strings.Contains(r.RawQuery, "good-token") || strings.Contains(r.Path, "good-token") {
			t.Errorf("token in URL: %s?%s", r.Path, r.RawQuery)
		}
		if !strings.Contains(r.Auth, `Token="good-token"`) {
			t.Errorf("request %s lacks the Authorization header", r.Path)
		}
	}
}

// A token that can see other people's sessions (an administrator's) must still
// show only its owner's playback unless told otherwise.
const (
	ownAndOthers = `[
	 {"Id":"mine","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
	  "NowPlayingItem":{"Id":"a","Name":"My Movie","Type":"Movie","RunTimeTicks":7200000000},"PlayState":{"PositionTicks":0}},
	 {"Id":"theirs","UserId":"u2","UserName":"Kenzy","LastActivityDate":"2026-11-15T11:00:00Z",
	  "NowPlayingItem":{"Id":"b","Name":"Their Movie","Type":"Movie","RunTimeTicks":7200000000},"PlayState":{"PositionTicks":0}}]`
)

func TestDefaultsToTheTokenOwnersSessions(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(ownAndOthers)

	out, _, _ := run(t, f, "good-token", Options{})

	if s := recv(t, out); s.Title != "My Movie" {
		t.Errorf("got %q, want the token owner's movie, not another user's", s.Title)
	}
}

func TestStarMeansEverySessionTheTokenCanSee(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(ownAndOthers)

	out, _, _ := run(t, f, "good-token", Options{User: "*"})

	if s := recv(t, out); s.Title != "Their Movie" {
		t.Errorf("got %q, want the most recent session of anyone", s.Title)
	}
}

func TestExplicitUserFilterBeatsTheOwner(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(ownAndOthers)

	out, _, _ := run(t, f, "good-token", Options{User: "kenzy"})

	if s := recv(t, out); s.Title != "Their Movie" {
		t.Errorf("got %q", s.Title)
	}
}

func TestAPIKeyWithoutAUserSeesEverySession(t *testing.T) {
	f := newFakeServer(t, "")
	f.meStatus = 400 // an API key has no user
	f.setSessions(ownAndOthers)

	out, _, _ := run(t, f, "good-token", Options{})

	if s := recv(t, out); s.Title != "Their Movie" {
		t.Errorf("got %q, want no owner filtering without a user", s.Title)
	}
}

func TestMoviePosterComesFromTheServerAndIsCached(t *testing.T) {
	f := newFakeServer(t, "")
	f.posters["m1"] = "https://image.example/amelie.jpg"
	f.setSessions(playingMovie)
	out, _, _ := run(t, f, "good-token", Options{Art: true})
	if s := recv(t, out); s.ArtURL != "https://image.example/amelie.jpg" {
		t.Fatalf("ArtURL = %q", s.ArtURL)
	}

	f.setSessions(pausedMovie)
	f.push()
	recv(t, out)

	if n := f.count("/Items/m1/RemoteImages"); n != 1 {
		t.Errorf("poster looked up %d times, want 1", n)
	}
}

func TestEpisodePosterIsTheSeriesPoster(t *testing.T) {
	f := newFakeServer(t, "")
	f.posters["ser1"] = "https://image.example/severance.jpg"
	f.setSessions(playingEpisode)

	out, _, _ := run(t, f, "good-token", Options{Art: true})

	if s := recv(t, out); s.ArtURL != "https://image.example/severance.jpg" {
		t.Errorf("ArtURL = %q", s.ArtURL)
	}
}

func TestPosterDisabledMakesNoRequest(t *testing.T) {
	f := newFakeServer(t, "")
	f.posters["m1"] = "https://image.example/amelie.jpg"
	f.setSessions(playingMovie)

	out, _, _ := run(t, f, "good-token", Options{})

	if s := recv(t, out); s.ArtURL != "" {
		t.Errorf("ArtURL = %q, want none", s.ArtURL)
	}
	if n := f.count("/Items/m1/RemoteImages"); n != 0 {
		t.Errorf("%d poster requests with art disabled", n)
	}
}
