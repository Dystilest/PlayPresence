package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/config"
	"github.com/Dystilest/PlayPresence/internal/secrets"
)

func runSetup(t *testing.T, path string, mem secrets.Store, discord *fakeDiscord, stdin string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	e := testEnv(discord, &out, &errOut)
	e.secrets = mem
	e.stdin = strings.NewReader(stdin)
	e.qcPoll = 2 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code := runWith(ctx, []string{"setup", "--config", path}, e)
	return code, out.String(), errOut.String()
}

func setupPathIn(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "mon dossier é", "playpresence", "config.toml")
}

func TestSetupWritesTheConfigSignsInAndChecksDiscord(t *testing.T) {
	f := newFakeJellyfin(t)
	path := setupPathIn(t)
	mem := secrets.NewMemory()

	// What people copy from the browser's address bar, without the scheme.
	pasted := strings.TrimPrefix(f.srv.URL, "http://") + "/web/index.html#!/home.html"
	code, out, errOut := runSetup(t, path, mem, newFakeDiscord(), pasted+"\n")

	if code != 0 {
		t.Fatalf("exit code = %d\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("the written configuration does not load: %v", err)
	}
	src, ok := cfg.Sources["jellyfin"]
	if !ok || src.Kind != "jellyfin" || src.URL != f.srv.URL {
		t.Fatalf("sources = %+v, want jellyfin at %s", cfg.Sources, f.srv.URL)
	}
	if got := get(t, mem, tokenKey("jellyfin")); got != jfToken {
		t.Errorf("stored token = %q", got)
	}
	raw, _ := os.ReadFile(path) //nolint:gosec // a file in the test's temporary directory
	if strings.Contains(string(raw), jfToken) {
		t.Error("the token must never be written to the configuration file")
	}
	for _, want := range []string{jfCode, "Signed in as Samy", "Discord is running", "playpresence run"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestSetupAgainReusesTheSourceInsteadOfDuplicatingIt(t *testing.T) {
	f := newFakeJellyfin(t)
	path := setupPathIn(t)
	mem := secrets.NewMemory()

	for i := 0; i < 2; i++ {
		if code, out, errOut := runSetup(t, path, mem, newFakeDiscord(), f.srv.URL+"/\n"); code != 0 {
			t.Fatalf("run %d: exit code = %d\n%s%s", i, code, out, errOut)
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 {
		t.Errorf("sources = %+v, want exactly one", cfg.Sources)
	}
}

func TestSetupKeepsExistingSettingsAndPicksAFreeName(t *testing.T) {
	f := newFakeJellyfin(t)
	path := writeConfig(t, "# my notes\n[presence]\nmin_interval = \"30s\"\n\n[sources.jellyfin]\nkind = \"jellyfin\"\nurl = \"http://192.0.2.1:8096\"\n")
	mem := secrets.NewMemory()

	if code, out, errOut := runSetup(t, path, mem, newFakeDiscord(), f.srv.URL+"\n"); code != 0 {
		t.Fatalf("exit code = %d\n%s%s", code, out, errOut)
	}
	raw, _ := os.ReadFile(path) //nolint:gosec // a file in the test's temporary directory
	if !strings.HasPrefix(string(raw), "# my notes\n") {
		t.Errorf("existing content was not kept:\n%s", raw)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Duration(cfg.Presence.MinInterval) != 30*time.Second || cfg.Sources["jellyfin2"].URL != f.srv.URL {
		t.Errorf("config = %+v", cfg)
	}
}

func TestSetupFallsBackToAnAPIKeyWhenQuickConnectIsOff(t *testing.T) {
	f := newFakeJellyfin(t)
	f.mu.Lock()
	f.qcEnabled = false
	f.mu.Unlock()
	path := setupPathIn(t)
	mem := secrets.NewMemory()

	code, out, errOut := runSetup(t, path, mem, nil, f.srv.URL+"\n"+jfToken+"\n")

	if code != 0 {
		t.Fatalf("exit code = %d\n%s%s", code, out, errOut)
	}
	if got := get(t, mem, tokenKey("jellyfin")); got != jfToken {
		t.Errorf("stored token = %q", got)
	}
	if !strings.Contains(out, "API Keys") || !strings.Contains(out, "Discord is not open") {
		t.Errorf("output:\n%s", out)
	}
}

func TestSetupStopsAfterThreeUnreachableAddressesWithoutEchoingThem(t *testing.T) {
	path := setupPathIn(t)
	bad := "http://127.0.0.1:1/private-path"

	code, out, errOut := runSetup(t, path, secrets.NewMemory(), nil, strings.Repeat(bad+"\n", 3))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if strings.Count(out, "Could not reach") != 3 {
		t.Errorf("want three explanations:\n%s", out)
	}
	if strings.Contains(out+errOut, "private-path") {
		t.Error("the address must not be echoed back")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("nothing must be written when no server was found")
	}
}

func TestSetupRefusesAnAddressWithCredentials(t *testing.T) {
	code, out, _ := runSetup(t, setupPathIn(t), secrets.NewMemory(), nil, "http://me:SECRETPASS@host:8096\n")

	if code != 1 || !strings.Contains(out, "without any password") || strings.Contains(out, "SECRETPASS") {
		t.Errorf("code = %d, output:\n%s", code, out)
	}
}

func TestStartRunsSetupWhenNothingIsConfigured(t *testing.T) {
	var out, errOut bytes.Buffer
	e := testEnv(nil, &out, &errOut)
	e.secrets = secrets.NewMemory()
	e.stdin = strings.NewReader("") // the user closes the window without answering

	code := runWith(context.Background(), []string{"start", "--config", setupPathIn(t)}, e)

	if code != 1 || !strings.Contains(out.String(), "Welcome to PlayPresence") {
		t.Errorf("code = %d, stdout:\n%s", code, out.String())
	}
}

func TestNormalizeServerURL(t *testing.T) {
	tests := map[string]string{
		"192.168.1.10:8096":                           "http://192.168.1.10:8096",
		" http://host:8096/ ":                         "http://host:8096",
		"http://host:8096/web/index.html#!/home.html": "http://host:8096",
		"https://example.org/jellyfin/web/":           "https://example.org/jellyfin",
		"https://example.org/website":                 "https://example.org/website",
		"":                                            "",
	}
	for in, want := range tests {
		if got := normalizeServerURL(in); got != want {
			t.Errorf("normalizeServerURL(%q) = %q, want %q", in, got, want)
		}
	}
}
