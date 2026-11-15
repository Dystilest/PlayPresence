package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/config"
	"github.com/Dystilest/PlayPresence/internal/presence"
)

// fakeDiscord answers the IPC handshake and acknowledges every command.
type fakeDiscord struct {
	mu       sync.Mutex
	received []map[string]any
	got      chan map[string]any
}

func newFakeDiscord() *fakeDiscord { return &fakeDiscord{got: make(chan map[string]any, 32)} }

func (f *fakeDiscord) dial(string) (net.Conn, error) {
	client, server := net.Pipe()
	go func() {
		defer func() { _ = server.Close() }()
		for {
			op, body, err := presence.DecodeFrame(server)
			if err != nil {
				return
			}
			var msg map[string]any
			if json.Unmarshal(body, &msg) != nil {
				return
			}
			f.mu.Lock()
			f.received = append(f.received, msg)
			f.mu.Unlock()
			select {
			case f.got <- msg:
			default:
			}
			resp := map[string]any{"cmd": msg["cmd"], "evt": nil}
			if op == presence.OpHandshake {
				resp = map[string]any{"evt": "READY", "cmd": "DISPATCH"}
			}
			raw, _ := presence.EncodeFrame(presence.OpFrame, resp)
			if _, err := server.Write(raw); err != nil {
				return
			}
		}
	}()
	return client, nil
}

func (f *fakeDiscord) activities() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []any
	for _, m := range f.received {
		if m["cmd"] == "SET_ACTIVITY" {
			out = append(out, m["args"].(map[string]any)["activity"])
		}
	}
	return out
}

func testEnv(f *fakeDiscord, out, errOut *bytes.Buffer) env {
	e := env{stdout: out, stderr: errOut, candidates: []string{"fake"}}
	if f != nil {
		e.dial = f.dial
	} else {
		e.dial = func(string) (net.Conn, error) { return nil, os.ErrNotExist }
	}
	return e
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mon dossier é", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunDemoPublishesAWatchingActivityAndClearsOnExit(t *testing.T) {
	f := newFakeDiscord()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan int, 1)

	noConfig := filepath.Join(t.TempDir(), "none.toml")
	go func() {
		done <- runWith(ctx, []string{"run", "--demo", "--app-id", "1234567890", "--config", noConfig}, testEnv(f, &out, &errOut))
	}()

	deadline := time.After(3 * time.Second)
	for len(f.activities()) == 0 {
		select {
		case <-f.got:
		case <-deadline:
			t.Fatalf("no activity received; stderr=%q", errOut.String())
		}
	}
	acts := f.activities()
	act := acts[0].(map[string]any)
	if act["type"] != float64(3) || act["details"] != "Severance" {
		t.Errorf("activity = %v, want a Watching activity for the demo episode", act)
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code = %d, want 0 on a clean stop; stderr=%q", code, errOut.String())
		}
	case <-time.After(4 * time.Second):
		t.Fatal("run did not return after cancel")
	}
	acts = f.activities()
	if last := acts[len(acts)-1]; last != nil {
		t.Errorf("last activity = %v, want it cleared on exit", last)
	}
}

// syncBuffer is a bytes.Buffer safe for a writer goroutine and a reading test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestRunTellsYouWhyNothingShowsWhenDiscordIsUnreachable(t *testing.T) {
	var out bytes.Buffer
	errOut := &syncBuffer{}
	e := env{stdout: &out, stderr: errOut, candidates: []string{"fake"},
		dial: func(string) (net.Conn, error) { return nil, os.ErrNotExist }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	noConfig := filepath.Join(t.TempDir(), "none.toml")

	go func() {
		done <- runWith(ctx, []string{"run", "--demo", "--app-id", "1234567890", "--config", noConfig}, e)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(errOut.String(), "not running") {
		if time.Now().After(deadline) {
			t.Fatalf("stderr = %q, want a message saying Discord is not running", errOut.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if n := strings.Count(errOut.String(), "not running"); n != 1 {
		t.Errorf("message printed %d times, want it deduplicated to 1", n)
	}
	cancel()
	if code := <-done; code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRunWithInvalidConfigExitsOneWithoutStackTrace(t *testing.T) {
	path := writeConfig(t, "[presence]\nmin_interval = \"1s\"\n")
	var out, errOut bytes.Buffer

	code := runWith(context.Background(), []string{"run", "--config", path}, testEnv(nil, &out, &errOut))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	msg := errOut.String()
	if !strings.Contains(msg, "min_interval") {
		t.Errorf("stderr = %q, want the validation message", msg)
	}
	if strings.Contains(msg, "goroutine") || strings.Contains(msg, ".go:") {
		t.Errorf("stderr must not contain a stack trace: %q", msg)
	}
}

func TestRunWithMissingConfigExplainsWhatToDo(t *testing.T) {
	var out, errOut bytes.Buffer

	code := runWith(context.Background(), []string{"run", "--config", filepath.Join(t.TempDir(), "nope.toml")}, testEnv(nil, &out, &errOut))

	if code != 1 || !strings.Contains(errOut.String(), "configuration.md") {
		t.Fatalf("code=%d stderr=%q, want exit 1 pointing to docs/configuration.md", code, errOut.String())
	}
}

func TestRunFallsBackToTheBuiltInApplicationWhenNoAppIDIsSet(t *testing.T) {
	path := writeConfig(t, "")
	f := newFakeDiscord()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	go func() { _ = runWith(ctx, []string{"run", "--demo", "--config", path}, testEnv(f, &out, &errOut)) }()

	select {
	case m := <-f.got:
		if m["client_id"] != config.GenericAppID {
			t.Errorf("handshake client_id = %v, want the built-in application %s", m["client_id"], config.GenericAppID)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no handshake; stderr=%q", errOut.String())
	}
}

func TestDoctorAllGoodExitsZeroWithOneLinePerCheck(t *testing.T) {
	path := writeConfig(t, "[discord]\napp_id = \"1234567890\"\n")
	var out, errOut bytes.Buffer

	code := runWith(context.Background(), []string{"doctor", "--config", path}, testEnv(newFakeDiscord(), &out, &errOut))

	if code != 0 {
		t.Fatalf("exit code = %d; output:\n%s%s", code, out.String(), errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 (config, discord, sources):\n%s", len(lines), out.String())
	}
	for i, want := range []string{"OK   config", "OK   discord", "WARN sources"} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], want)
		}
	}
}

func TestDoctorReportsDiscordNotRunningWithAHint(t *testing.T) {
	path := writeConfig(t, "[discord]\napp_id = \"1234567890\"\n")
	var out, errOut bytes.Buffer

	code := runWith(context.Background(), []string{"doctor", "--config", path}, testEnv(nil, &out, &errOut))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "FAIL discord") || !strings.Contains(out.String(), "not running") {
		t.Errorf("output = %q, want a FAIL discord line saying it is not running", out.String())
	}
	if !strings.Contains(out.String(), "Activity Privacy") {
		t.Errorf("output = %q, want a hint about Discord's activity privacy setting", out.String())
	}
}

func TestDoctorHintsAtTheAppIDWhenDiscordRejectsTheHandshake(t *testing.T) {
	path := writeConfig(t, "[discord]\napp_id = \"1234567890\"\n")
	var out, errOut bytes.Buffer
	e := testEnv(nil, &out, &errOut)
	e.dial = func(string) (net.Conn, error) {
		client, server := net.Pipe()
		_ = server.Close() // Discord reachable but hangs up on the handshake
		return client, nil
	}

	code := runWith(context.Background(), []string{"doctor", "--config", path}, e)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	got := out.String()
	if !strings.Contains(got, "FAIL discord") || !strings.Contains(got, "discord.app_id") {
		t.Errorf("output = %q, want a FAIL discord line hinting at discord.app_id", got)
	}
	if strings.Contains(got, "not running") {
		t.Errorf("output = %q, a reachable Discord must not be reported as not running", got)
	}
}

func TestDoctorFlagsAMissingConfig(t *testing.T) {
	var out, errOut bytes.Buffer

	code := runWith(context.Background(), []string{"doctor", "--config", filepath.Join(t.TempDir(), "nope.toml")}, testEnv(newFakeDiscord(), &out, &errOut))

	if code != 1 || !strings.Contains(out.String(), "FAIL config") {
		t.Fatalf("code=%d output=%q, want exit 1 and a FAIL config line", code, out.String())
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	var out, errOut bytes.Buffer

	code := runWith(context.Background(), []string{"run", "--nope"}, testEnv(nil, &out, &errOut))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
