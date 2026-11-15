package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, content string) string {
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

func TestDefaults(t *testing.T) {
	c := Default()

	if time.Duration(c.Presence.MinInterval) != 15*time.Second {
		t.Errorf("min interval = %v", time.Duration(c.Presence.MinInterval))
	}
	if len(c.Art.Resolvers) == 0 {
		t.Error("default art resolvers must not be empty")
	}
	if err := c.Validate(); err != nil {
		t.Errorf("defaults must validate: %v", err)
	}
}

func TestLoadAppliesValuesOverDefaultsAndHandlesSpacesInPath(t *testing.T) {
	path := write(t, `
[discord]
app_id = "1234567890"

[presence]
min_interval = "30s"
priority = ["home"]

[sources.home]
kind = "jellyfin"
url = "http://192.168.1.10:8096"
user = "samy"
`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.Discord.AppID != "1234567890" {
		t.Errorf("app id = %q", c.Discord.AppID)
	}
	if time.Duration(c.Presence.MinInterval) != 30*time.Second {
		t.Errorf("min interval = %v", time.Duration(c.Presence.MinInterval))
	}
	if src := c.Sources["home"]; src.Kind != "jellyfin" || src.URL != "http://192.168.1.10:8096" || src.Disabled {
		t.Errorf("source = %+v", src)
	}
	if len(c.Art.Resolvers) == 0 {
		t.Error("omitted keys must keep their defaults")
	}
}

func TestLoadMissingFileReportsNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.toml"))

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want a not-exist error", err)
	}
}

func TestLoadUnknownKeyNamesTheKey(t *testing.T) {
	path := write(t, "[presence]\nmin_intervall = \"20s\"\n")

	_, err := Load(path)

	if err == nil || !strings.Contains(err.Error(), "min_intervall") {
		t.Fatalf("err = %v, want it to name min_intervall", err)
	}
}

func TestLoadRejectsCredentialsInTheFile(t *testing.T) {
	for _, line := range []string{`token = "abc"`, `api_key = "abc"`, `password = "abc"`, `tmdb_key = "abc"`} {
		t.Run(line, func(t *testing.T) {
			path := write(t, "[sources.home]\nkind = \"jellyfin\"\nurl = \"http://x\"\n"+line+"\n")

			_, err := Load(path)

			if err == nil || !strings.Contains(err.Error(), "credential store") {
				t.Fatalf("err = %v, want a message pointing to the credential store", err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string // substring of the error, "" for valid
	}{
		{"valid", func(*Config) {}, ""},
		{"interval too short", func(c *Config) { c.Presence.MinInterval = Duration(5 * time.Second) }, "min_interval"},
		{"non numeric app id", func(c *Config) { c.Discord.AppID = "abc" }, "app_id"},
		{"unknown source kind", func(c *Config) { c.Sources = map[string]SourceConfig{"a": {Kind: "netflix", URL: "http://x"}} }, "kind"},
		{"source without url", func(c *Config) { c.Sources = map[string]SourceConfig{"a": {Kind: "plex"}} }, "url"},
		{"source with bad url", func(c *Config) { c.Sources = map[string]SourceConfig{"a": {Kind: "plex", URL: "ftp://x"}} }, "url"},
		{"priority names unknown source", func(c *Config) { c.Presence.Priority = []string{"ghost"} }, "ghost"},
		{"unknown art resolver", func(c *Config) { c.Art.Resolvers = []string{"magic"} }, "magic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			tt.mutate(&c)

			err := c.Validate()

			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestSourceURLMustNotCarryCredentialsAndErrorsDoNotEchoThem(t *testing.T) {
	tests := []struct {
		name, url, secret, want string
	}{
		{"userinfo", "http://alice:SECRETPASS@host:8096", "SECRETPASS", "credential store"},
		{"user only", "http://SECRETUSER@host:8096", "SECRETUSER", "credential store"},
		{"token query", "http://host:32400/?X-Plex-Token=SECRETTOKEN", "SECRETTOKEN", "credential store"},
		{"api key query", "http://host:8096/?api_key=SECRETKEY", "SECRETKEY", "credential store"},
		{"malformed value", "SECRET-no-scheme", "SECRET-no-scheme", "http or https URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			c.Sources = map[string]SourceConfig{"home": {Kind: "plex", URL: tt.url}}

			err := c.Validate()

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
			if strings.Contains(err.Error(), tt.secret) {
				t.Errorf("error %q must not echo the secret value", err)
			}
		})
	}
}

func TestSourceURLWithPathAndPortIsAccepted(t *testing.T) {
	c := Default()
	c.Sources = map[string]SourceConfig{"home": {Kind: "jellyfin", URL: "https://media.example.org:8920/jellyfin"}}

	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRunsValidation(t *testing.T) {
	path := write(t, "[presence]\nmin_interval = \"1s\"\n")

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "min_interval") {
		t.Fatalf("err = %v, want a validation error", err)
	}
}

func TestDefaultPathEndsWithAppFolder(t *testing.T) {
	p, err := DefaultPath()
	if err != nil {
		t.Skipf("no user config dir in this environment: %v", err)
	}

	if filepath.Base(p) != "config.toml" || filepath.Base(filepath.Dir(p)) != "playpresence" {
		t.Errorf("path = %s", p)
	}
}

func TestReferenceIsUpToDate(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "docs", "configuration.md"))
	if err != nil {
		t.Fatalf("docs/configuration.md missing: %v (run `go run ./internal/config/gen`)", err)
	}

	if got := Reference(); got != string(want) {
		t.Error("docs/configuration.md is stale; run `go run ./internal/config/gen`")
	}
}

func TestResolveAppID(t *testing.T) {
	src := func(kinds ...string) map[string]SourceConfig {
		m := map[string]SourceConfig{}
		for i, k := range kinds {
			m[string(rune('a'+i))] = SourceConfig{Kind: k, URL: "http://h"}
		}
		return m
	}
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"override wins", Config{Discord: DiscordConfig{AppID: "42"}, Sources: src("jellyfin")}, "42"},
		{"one jellyfin source", Config{Sources: src("jellyfin")}, JellyfinAppID},
		{"two jellyfin sources", Config{Sources: src("jellyfin", "jellyfin")}, JellyfinAppID},
		{"mixed kinds use the generic app", Config{Sources: src("jellyfin", "plex")}, GenericAppID},
		{"a kind without its own app", Config{Sources: src("plex")}, GenericAppID},
		{"no sources (demo)", Config{}, GenericAppID},
		{"disabled sources are ignored", Config{Sources: map[string]SourceConfig{
			"a": {Kind: "jellyfin", URL: "http://h"}, "b": {Kind: "plex", URL: "http://h", Disabled: true}}}, JellyfinAppID},
	}
	for _, c := range cases {
		if got := c.cfg.ResolveAppID(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
