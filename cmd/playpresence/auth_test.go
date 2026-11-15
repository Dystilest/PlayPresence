package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/secrets"
)

const (
	jfSecret = "SECRET-qc-0123456789"
	jfCode   = "424242"
	jfToken  = "issued-token"
)

// fakeJellyfin imitates the endpoints the CLI touches. Payloads are written
// by hand from the API's JSON shape.
type fakeJellyfin struct {
	srv *httptest.Server

	mu         sync.Mutex
	qcEnabled  bool
	pending    int // Connect answers "not yet" this many times
	polls      int
	validToken string
	sessions   string
	meStatus   int // non-zero: /Users/Me answers with it (API keys have no user)
}

var jfTokenRE = regexp.MustCompile(`Token="([^"]*)"`)

func newFakeJellyfin(t *testing.T) *fakeJellyfin {
	t.Helper()
	f := &fakeJellyfin{qcEnabled: true, pending: 1, validToken: jfToken, sessions: "[]"}
	mux := http.NewServeMux()
	mux.HandleFunc("/System/Info/Public", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ServerName": "Test", "Version": "10.10.0"})
	})
	mux.HandleFunc("/QuickConnect/Enabled", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.qcEnabled)
	})
	mux.HandleFunc("/QuickConnect/Initiate", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"Secret": jfSecret, "Code": jfCode, "Authenticated": false})
	})
	mux.HandleFunc("/QuickConnect/Connect", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.polls++
		done := f.polls > f.pending
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"Authenticated": done})
	})
	mux.HandleFunc("/Users/AuthenticateWithQuickConnect", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"AccessToken": jfToken, "User": map[string]string{"Id": "u1", "Name": "Samy"},
		})
	})
	mux.HandleFunc("/Users/Me", func(w http.ResponseWriter, r *http.Request) {
		m := jfTokenRE.FindStringSubmatch(r.Header.Get("Authorization"))
		f.mu.Lock()
		ok := m != nil && m[1] == f.validToken
		f.mu.Unlock()
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		ms := f.meStatus
		f.mu.Unlock()
		if ms != 0 {
			http.Error(w, "no user", ms)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "u1", "Name": "Samy"})
	})
	mux.HandleFunc("/Sessions", func(w http.ResponseWriter, r *http.Request) {
		m := jfTokenRE.FindStringSubmatch(r.Header.Get("Authorization"))
		f.mu.Lock()
		ok, body := m != nil && m[1] == f.validToken, f.sessions
		f.mu.Unlock()
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(body))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeJellyfin) setSessions(js string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = js
}

// writeConfig writes a config with a jellyfin source "home" and a plex one.
func writeJFConfig(t *testing.T, jellyfinURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	body := fmt.Sprintf("[discord]\napp_id = \"123456789012345678\"\n\n[sources.home]\nkind = \"jellyfin\"\nurl = %q\n\n[sources.living]\nkind = \"plex\"\nurl = \"http://127.0.0.1:32400\"\n", jellyfinURL)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func authEnv(mem secrets.Store, stdin string) (env, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	e := testEnv(nil, &out, &errOut)
	e.secrets = mem
	e.stdin = strings.NewReader(stdin)
	e.qcPoll = 2 * time.Millisecond
	return e, &out, &errOut
}

func runAuth(t *testing.T, mem secrets.Store, stdin string, args ...string) (int, string, string) {
	t.Helper()
	e, out, errOut := authEnv(mem, stdin)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code := runWith(ctx, append([]string{"auth"}, args...), e)
	return code, out.String(), errOut.String()
}

func get(t *testing.T, s secrets.Store, key string) string {
	t.Helper()
	v, err := s.Get(secretService, key)
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal(err)
	}
	return v
}

func TestAuthQuickConnectStoresCredentialsAndReusesDeviceID(t *testing.T) {
	f := newFakeJellyfin(t)
	cfg := writeJFConfig(t, f.srv.URL)
	mem := secrets.NewMemory()

	code, out, errOut := runAuth(t, mem, "", "jellyfin", "--source", "home", "--config", cfg)

	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, jfCode) {
		t.Errorf("output lacks the Quick Connect code: %q", out)
	}
	if strings.Contains(out+errOut, jfToken) || strings.Contains(out+errOut, jfSecret) {
		t.Errorf("output leaks a credential: %q %q", out, errOut)
	}
	if get(t, mem, "source.home.token") != jfToken || get(t, mem, "source.home.user_id") != "u1" {
		t.Error("token or user id not stored")
	}
	dev1 := get(t, mem, "device_id")
	if dev1 == "" {
		t.Fatal("device id not stored")
	}
	f.mu.Lock()
	f.polls = 0
	f.mu.Unlock()
	if code, _, errOut := runAuth(t, mem, "", "jellyfin", "--source", "home", "--config", cfg); code != 0 {
		t.Fatalf("second run exit %d: %s", code, errOut)
	}
	if dev2 := get(t, mem, "device_id"); dev2 != dev1 {
		t.Errorf("device id changed: %q -> %q", dev1, dev2)
	}
}

func TestAuthUnknownOrNonJellyfinSourceFails(t *testing.T) {
	f := newFakeJellyfin(t)
	cfg := writeJFConfig(t, f.srv.URL)

	for _, name := range []string{"nope", "living"} {
		code, _, errOut := runAuth(t, secrets.NewMemory(), "", "jellyfin", "--source", name, "--config", cfg)
		if code != 1 || !strings.Contains(errOut, name) {
			t.Errorf("source %q: exit %d, stderr %q", name, code, errOut)
		}
		if strings.Contains(errOut, f.srv.URL) {
			t.Errorf("stderr echoes the server URL: %q", errOut)
		}
	}
}

func TestAuthQuickConnectDisabledExplainsTheAlternative(t *testing.T) {
	f := newFakeJellyfin(t)
	f.qcEnabled = false
	cfg := writeJFConfig(t, f.srv.URL)
	mem := secrets.NewMemory()

	code, _, errOut := runAuth(t, mem, "", "jellyfin", "--source", "home", "--config", cfg)

	if code != 1 || !strings.Contains(errOut, "--token-stdin") || !strings.Contains(errOut, "Quick Connect") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	if get(t, mem, "source.home.token") != "" {
		t.Error("something was stored")
	}
}

func TestAuthTokenStdinVerifiesBeforeStoring(t *testing.T) {
	f := newFakeJellyfin(t)
	cfg := writeJFConfig(t, f.srv.URL)
	mem := secrets.NewMemory()

	code, _, errOut := runAuth(t, mem, "wrong-token\n", "jellyfin", "--source", "home", "--config", cfg, "--token-stdin")
	if code != 1 || strings.Contains(errOut, "wrong-token") {
		t.Errorf("bad token: exit %d, stderr %q", code, errOut)
	}
	if get(t, mem, "source.home.token") != "" {
		t.Error("a rejected token was stored")
	}

	code, out, errOut := runAuth(t, mem, jfToken+"\n", "jellyfin", "--source", "home", "--config", cfg, "--token-stdin")
	if code != 0 {
		t.Fatalf("good token: exit %d: %s", code, errOut)
	}
	if get(t, mem, "source.home.token") != jfToken {
		t.Error("good token not stored")
	}
	if strings.Contains(out+errOut, jfToken) {
		t.Errorf("output leaks the token: %q", out+errOut)
	}
}

func TestAuthUsageErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"plex", "--source", "home"},
		{"jellyfin"},
	}
	for _, args := range cases {
		if code, _, _ := runAuth(t, secrets.NewMemory(), "", args...); code != 2 {
			t.Errorf("auth %v: exit %d, want 2", args, code)
		}
	}
}
