package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/secrets"
)

const (
	samyPlaying = `{"Id":"s1","UserId":"u1","UserName":"Samy","LastActivityDate":"2026-11-15T10:00:00Z",
	 "NowPlayingItem":{"Id":"m1","Name":"Amélie","Type":"Movie","ProductionYear":2001,"RunTimeTicks":7320000000},
	 "PlayState":{"PositionTicks":600000000}}`
	alicePlaying = `{"Id":"s2","UserId":"u2","UserName":"Alice","LastActivityDate":"2026-11-15T11:00:00Z",
	 "NowPlayingItem":{"Id":"m2","Name":"Other Movie","Type":"Movie","RunTimeTicks":7200000000},
	 "PlayState":{"PositionTicks":0}}`
)

// jfOnlyConfig writes a config with a single jellyfin source named name.
func jfOnlyConfig(t *testing.T, name, url, extra string) string {
	t.Helper()
	body := fmt.Sprintf("[discord]\napp_id = \"123456789012345678\"\n\n[sources.%q]\nkind = \"jellyfin\"\nurl = %q\n%s", name, url, extra)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func memWithToken(name, token string) *secrets.Memory {
	m := secrets.NewMemory()
	_ = m.Set(secretService, "device_id", "dev-1")
	if token != "" {
		_ = m.Set(secretService, tokenKey(name), token)
	}
	return m
}

// startRun runs `playpresence run` against fake Discord in the background.
func startRun(t *testing.T, cfg string, mem secrets.Store) (*fakeDiscord, *syncBuffer, func() int) {
	t.Helper()
	d := newFakeDiscord()
	var out bytes.Buffer
	errOut := &syncBuffer{}
	e := testEnv(d, &out, nil)
	e.stderr = errOut
	e.secrets = mem
	e.pollInterval = 20 * time.Millisecond
	e.minInterval = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(cancel)
	done := make(chan int, 1)
	go func() { done <- runWith(ctx, []string{"run", "--config", cfg}, e) }()
	return d, errOut, func() int {
		cancel()
		select {
		case c := <-done:
			return c
		case <-time.After(4 * time.Second):
			t.Fatal("run did not return")
			return -1
		}
	}
}

func waitActivity(t *testing.T, d *fakeDiscord, errOut *syncBuffer, pred func(any) bool) any {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		acts := d.activities()
		if len(acts) > 0 && pred(acts[len(acts)-1]) {
			return acts[len(acts)-1]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected activity not seen; got %v; stderr=%q", d.activities(), errOut.String())
	return nil
}

func TestRunPublishesJellyfinPlaybackAndClearsOnStop(t *testing.T) {
	f := newFakeJellyfin(t)
	f.setSessions("[" + samyPlaying + "]")
	// A source key with a space and an accent must work.
	cfg := jfOnlyConfig(t, "Salon é", f.srv.URL, "")
	d, errOut, stop := startRun(t, cfg, memWithToken("Salon é", jfToken))

	act := waitActivity(t, d, errOut, func(a any) bool { return a != nil })
	m := act.(map[string]any)
	if m["type"] != float64(3) || !strings.Contains(fmt.Sprint(m["details"]), "Amélie") {
		t.Errorf("activity = %v, want a Watching activity for the movie", m)
	}

	f.setSessions("[]")
	waitActivity(t, d, errOut, func(a any) bool { return a == nil })

	if code := stop(); code != 0 {
		t.Errorf("exit %d; stderr=%q", code, errOut.String())
	}
}

func TestRunHonoursTheUserFilterFromConfig(t *testing.T) {
	f := newFakeJellyfin(t)
	f.setSessions("[" + alicePlaying + "," + samyPlaying + "]")
	cfg := jfOnlyConfig(t, "home", f.srv.URL, "user = \"samy\"\n")
	d, errOut, stop := startRun(t, cfg, memWithToken("home", jfToken))

	act := waitActivity(t, d, errOut, func(a any) bool { return a != nil })

	if !strings.Contains(fmt.Sprint(act.(map[string]any)["details"]), "Amélie") {
		t.Errorf("activity = %v, want Samy's movie, not Alice's", act)
	}
	stop()
}

func TestRunWithoutATokenPointsToAuth(t *testing.T) {
	f := newFakeJellyfin(t)
	cfg := jfOnlyConfig(t, "home", f.srv.URL, "")
	var out, errOut bytes.Buffer
	e := testEnv(newFakeDiscord(), &out, &errOut)
	e.secrets = memWithToken("home", "")

	code := runWith(context.Background(), []string{"run", "--config", cfg}, e)

	if code != 1 || !strings.Contains(errOut.String(), "playpresence auth jellyfin --source home") {
		t.Errorf("exit %d, stderr %q", code, errOut.String())
	}
}

func doctorLines(t *testing.T, cfg string, mem secrets.Store) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	e := testEnv(newFakeDiscord(), &out, &errOut)
	e.secrets = mem
	code := runWith(context.Background(), []string{"doctor", "--config", cfg}, e)
	return code, out.String()
}

func TestDoctorChecksJellyfinSources(t *testing.T) {
	f := newFakeJellyfin(t)
	cfg := jfOnlyConfig(t, "home", f.srv.URL, "")

	code, out := doctorLines(t, cfg, memWithToken("home", jfToken))
	if code != 0 || !strings.Contains(out, "OK   source:home") || !strings.Contains(out, "Jellyfin 10.10.0 reachable, token valid for Samy") {
		t.Errorf("good: exit %d\n%s", code, out)
	}
	if strings.Contains(out, "arrive with") {
		t.Errorf("the generic warning must not cover jellyfin sources:\n%s", out)
	}

	code, out = doctorLines(t, cfg, memWithToken("home", "revoked"))
	if code != 1 || !strings.Contains(out, "FAIL source:home") || !strings.Contains(out, "token rejected") || strings.Contains(out, "revoked") {
		t.Errorf("rejected: exit %d\n%s", code, out)
	}

	code, out = doctorLines(t, cfg, memWithToken("home", ""))
	if code != 1 || !strings.Contains(out, "playpresence auth jellyfin --source home") {
		t.Errorf("no token: exit %d\n%s", code, out)
	}
}

func TestDoctorReportsAnUnreachableJellyfinServer(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	cfg := jfOnlyConfig(t, "home", "http://"+addr, "")

	code, out := doctorLines(t, cfg, memWithToken("home", jfToken))

	if code != 1 || !strings.Contains(out, "FAIL source:home") || !strings.Contains(out, "unreachable") {
		t.Errorf("exit %d\n%s", code, out)
	}
	if strings.Contains(out, addr) {
		t.Errorf("doctor output echoes the server address:\n%s", out)
	}
}

func TestDoctorAcceptsAnAPIKeyThatHasNoUser(t *testing.T) {
	f := newFakeJellyfin(t)
	f.meStatus = 400 // Jellyfin answers /Users/Me with 400 for an API key
	cfg := jfOnlyConfig(t, "home", f.srv.URL, "")

	code, out := doctorLines(t, cfg, memWithToken("home", jfToken))

	if code != 0 || !strings.Contains(out, "OK   source:home") || !strings.Contains(out, "token valid") {
		t.Errorf("exit %d\n%s", code, out)
	}
}
