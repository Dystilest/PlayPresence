// Package config loads and validates the TOML configuration file.
//
// The file never contains credentials: they live in the OS credential store.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// MinPresenceInterval is the shortest allowed delay between Discord updates.
const MinPresenceInterval = 15 * time.Second

// SourceKinds lists the supported media server kinds.
var SourceKinds = []string{"jellyfin", "plex", "emby"}

// ArtResolvers lists the supported artwork resolvers, in default order first.
var ArtResolvers = []string{"server", "plex", "tvmaze", "tmdb", "fanart", "declared_url", "upload"}

// Duration is a time.Duration written as text in TOML (for example "15s").
type Duration time.Duration

// UnmarshalText parses a Go duration string.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", b, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalText formats the duration.
func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

// Config is the whole configuration file.
type Config struct {
	Discord  DiscordConfig           `toml:"discord"`
	Presence PresenceConfig          `toml:"presence"`
	Art      ArtConfig               `toml:"art"`
	Sources  map[string]SourceConfig `toml:"sources"`
}

// DiscordConfig configures the Discord side.
type DiscordConfig struct {
	AppID string `toml:"app_id" doc:"Numeric ID of your own Discord application (Developer Portal). Optional: by default the application named after your media server is used, so its name shows after \"Watching\"."`
}

// PresenceConfig configures how the presence is published.
type PresenceConfig struct {
	MinInterval Duration `toml:"min_interval" doc:"Refill time of the update allowance: up to three updates may go out at once, then one per this delay. Cannot be lower than 15s."`
	Priority    []string `toml:"priority" doc:"Source names, most important first, used when several sources play at once. Empty means alphabetical."`
}

// ArtConfig configures artwork lookup. API keys are never set here; optional
// keys (for example TMDb) are stored in the OS credential store.
type ArtConfig struct {
	Resolvers []string `toml:"resolvers" doc:"Artwork resolvers tried in order until one returns an image. Available: server, plex, tvmaze, tmdb, fanart, declared_url, upload."`
}

// SourceConfig describes one media server.
type SourceConfig struct {
	Kind     string `toml:"kind" doc:"Server type: jellyfin, plex or emby."`
	URL      string `toml:"url" doc:"Base URL of the server, http or https."`
	User     string `toml:"user" doc:"Server user whose activity is shown. Empty means the account you signed in with; use \"*\" to show every session the token can see."`
	Disabled bool   `toml:"disabled" doc:"Set to true to keep the entry but ignore it."`
}

// Default returns the configuration used when no file exists or keys are omitted.
func Default() Config {
	return Config{
		Presence: PresenceConfig{MinInterval: Duration(MinPresenceInterval)},
		Art:      ArtConfig{Resolvers: []string{"server", "plex", "tvmaze"}},
		Sources:  map[string]SourceConfig{},
	}
}

// DefaultPath returns the per-user configuration file location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: locate user config dir: %w", err)
	}
	return filepath.Join(dir, "playpresence", "config.toml"), nil
}

// Load reads, decodes (strictly) and validates the file at path.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the user's own config location
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}

	c := Default()
	dec := toml.NewDecoder(bytes.NewReader(raw)).DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, decodeError(path, err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("%w (in %s)", err, path)
	}
	return c, nil
}

func decodeError(path string, err error) error {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) {
		var msgs []string
		for _, de := range strict.Errors {
			key := strings.Join(de.Key(), ".")
			if looksSecret(de.Key()[len(de.Key())-1]) {
				msgs = append(msgs, fmt.Sprintf("config: %s: credentials must not be stored in the config file; they are kept in the OS credential store (see docs/privacy.md)", key))
			} else {
				msgs = append(msgs, fmt.Sprintf("config: unknown key %q", key))
			}
		}
		return fmt.Errorf("%s (in %s)", strings.Join(msgs, "; "), path)
	}
	var derr *toml.DecodeError
	if errors.As(err, &derr) {
		row, col := derr.Position()
		return fmt.Errorf("config: %s:%d:%d: %s", path, row, col, derr.Error())
	}
	return fmt.Errorf("config: decode %s: %w", path, err)
}

// hasSecretQuery reports whether the URL carries a parameter that looks like a
// credential (for example X-Plex-Token or api_key).
func hasSecretQuery(u *url.URL) bool {
	for key := range u.Query() {
		if looksSecret(key) {
			return true
		}
	}
	return false
}

func looksSecret(name string) bool {
	n := strings.ToLower(name)
	for _, frag := range []string{"token", "password", "passwd", "secret", "api_key", "apikey"} {
		if strings.Contains(n, frag) {
			return true
		}
	}
	return n == "key" || strings.HasSuffix(n, "_key")
}

// Validate checks the configuration for consistency.
func (c Config) Validate() error {
	if id := c.Discord.AppID; id != "" {
		for _, r := range id {
			if r < '0' || r > '9' {
				return fmt.Errorf("config: discord.app_id must be numeric, got %q", id)
			}
		}
	}
	if time.Duration(c.Presence.MinInterval) < MinPresenceInterval {
		return fmt.Errorf("config: presence.min_interval must be at least %s, got %s",
			MinPresenceInterval, time.Duration(c.Presence.MinInterval))
	}

	names := make([]string, 0, len(c.Sources))
	for name := range c.Sources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := c.Sources[name]
		if !slices.Contains(SourceKinds, s.Kind) {
			return fmt.Errorf("config: sources.%s.kind must be one of %s, got %q", name, strings.Join(SourceKinds, ", "), s.Kind)
		}
		// Error messages never echo the URL: it may be a mistyped secret.
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("config: sources.%s.url must be an http or https URL such as http://host:8096", name)
		}
		if u.User != nil || hasSecretQuery(u) {
			return fmt.Errorf("config: sources.%s.url must not contain credentials (user:password@ or a token or key parameter); credentials are kept in the OS credential store (see docs/privacy.md)", name)
		}
	}
	for _, name := range c.Presence.Priority {
		if _, ok := c.Sources[name]; !ok {
			return fmt.Errorf("config: presence.priority names unknown source %q", name)
		}
	}
	for _, r := range c.Art.Resolvers {
		if !slices.Contains(ArtResolvers, r) {
			return fmt.Errorf("config: art.resolvers has unknown resolver %q", r)
		}
	}
	return nil
}

// Discord applications shipped with the program, so that nobody has to create
// one. Their IDs are public identifiers, not secrets: the name of the
// application is what Discord shows after "Watching".
const (
	GenericAppID  = "1571603122856657027" // "PlayPresence"
	JellyfinAppID = "1571620090179489945" // "Jellyfin"
)

// kindApps maps a server kind to the application named after it.
var kindApps = map[string]string{"jellyfin": JellyfinAppID}

// ResolveAppID returns the Discord application to use: discord.app_id when
// set; otherwise the application of the server kind when every enabled source
// is of that same kind; otherwise the generic one.
func (c Config) ResolveAppID() string {
	if c.Discord.AppID != "" {
		return c.Discord.AppID
	}
	kind := ""
	for _, s := range c.Sources {
		if s.Disabled {
			continue
		}
		if kind != "" && s.Kind != kind {
			return GenericAppID
		}
		kind = s.Kind
	}
	if id, ok := kindApps[kind]; ok {
		return id
	}
	return GenericAppID
}
