package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/Dystilest/PlayPresence/internal/app"
	"github.com/Dystilest/PlayPresence/internal/config"
	"github.com/Dystilest/PlayPresence/internal/presence"
	"github.com/Dystilest/PlayPresence/internal/secrets"
	"github.com/Dystilest/PlayPresence/internal/session"
	"github.com/Dystilest/PlayPresence/internal/source"
	"github.com/Dystilest/PlayPresence/internal/source/fake"
	"github.com/Dystilest/PlayPresence/internal/source/jellyfin"
)

// loadConfig reads the configuration at path (default location when empty).
// With tolerateMissing a missing file yields the defaults.
func loadConfig(path string, tolerateMissing bool) (config.Config, string, error) {
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return config.Config{}, "", err
		}
		path = p
	}
	cfg, err := config.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		if tolerateMissing {
			return config.Default(), path, nil
		}
		return config.Config{}, path, fmt.Errorf("config file not found at %s; create it (see docs/configuration.md)", path)
	}
	return cfg, path, err
}

func cmdRun(ctx context.Context, args []string, e env) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	cfgPath := fs.String("config", "", "path to the configuration file")
	demo := fs.Bool("demo", false, "show a canned episode instead of watching real servers")
	appID := fs.String("app-id", "", "Discord application ID (overrides discord.app_id)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	cfg, _, err := loadConfig(*cfgPath, *demo)
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}
	if *appID != "" {
		cfg.Discord.AppID = *appID
	}
	if err := cfg.Validate(); err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}
	if cfg.Discord.AppID == "" {
		_, _ = fmt.Fprintln(e.stderr, "playpresence: discord.app_id is not set: put it in the config file or pass --app-id")
		return 1
	}

	sources, err := buildSources(cfg, *demo, e)
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}

	client := presence.NewIPCClient(cfg.Discord.AppID, e.dial, e.candidates)
	defer func() { _ = client.Close() }()
	minInterval := time.Duration(cfg.Presence.MinInterval)
	if e.minInterval > 0 {
		minInterval = e.minInterval // test hook; the config enforces its own floor
	}
	pub := presence.NewPublisher(client, minInterval, presence.RealClock{})
	pub.OnError = publishErrorReporter(e.stderr)
	orch := app.NewOrchestrator(sources, cfg.Presence.Priority, nil, pub, presence.RealClock{})
	orch.OnSourceError = sourceErrorReporter(e.stderr)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pubDone := make(chan struct{})
	go func() {
		defer close(pubDone)
		_ = pub.Run(ctx)
	}()
	err = orch.Run(ctx)
	cancel()
	<-pubDone // the publisher clears the presence before returning

	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}
	return 0
}

// publishErrorReporter returns a callback that tells the user why nothing
// shows on Discord, printing each distinct problem once instead of on every
// retry. It is called from the single publishing goroutine.
func publishErrorReporter(w io.Writer) func(error) {
	var last string
	return func(err error) {
		msg := err.Error()
		if errors.Is(err, presence.ErrNotRunning) {
			msg = "Discord is not running; will keep retrying (run `playpresence doctor` for help)"
		}
		if msg == last {
			return
		}
		last = msg
		_, _ = fmt.Fprintf(w, "playpresence: %s\n", msg)
	}
}

// sourceErrorReporter returns a callback that tells the user why a source is
// not delivering, printing each distinct problem once per source instead of on
// every retry. It is called from the sources' goroutines.
func sourceErrorReporter(w io.Writer) func(name string, err error) {
	var mu sync.Mutex
	last := map[string]string{}
	return func(name string, err error) {
		msg := err.Error()
		if errors.Is(err, jellyfin.ErrUnauthorized) {
			msg = fmt.Sprintf("token rejected: run `playpresence auth jellyfin --source %s`", name)
		}
		mu.Lock()
		defer mu.Unlock()
		if last[name] == msg {
			return
		}
		last[name] = msg
		_, _ = fmt.Fprintf(w, "playpresence: source %s: %s\n", name, msg)
	}
}

// enabledSources returns the names of the sources that are not disabled, sorted.
func enabledSources(cfg config.Config) []string {
	var names []string
	for name, s := range cfg.Sources {
		if !s.Disabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func buildSources(cfg config.Config, demo bool, e env) ([]source.Source, error) {
	if demo {
		return []source.Source{fake.New("demo", []fake.Step{{Session: demoSession()}})}, nil
	}
	names := enabledSources(cfg)
	if len(names) == 0 {
		return nil, errors.New("no sources configured: add a [sources.<name>] section (see docs/configuration.md) or try --demo")
	}
	var st secrets.Store
	var out []source.Source
	for _, name := range names {
		sc := cfg.Sources[name]
		if sc.Kind != "jellyfin" {
			return nil, fmt.Errorf("source %q: the %s integration is not available in this build yet", name, sc.Kind)
		}
		if st == nil {
			var err error
			if st, err = openSecrets(e); err != nil {
				return nil, err
			}
		}
		src, err := buildJellyfin(name, sc, st, e.pollInterval, slices.Contains(cfg.Art.Resolvers, "server"))
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, nil
}

// jellyfinClient builds the client of a configured Jellyfin source from the
// credential store.
func jellyfinClient(name string, sc config.SourceConfig, st secrets.Store) (*jellyfin.Client, error) {
	token, err := st.Get(secretService, tokenKey(name))
	if errors.Is(err, secrets.ErrNotFound) || (err == nil && token == "") {
		return nil, fmt.Errorf("source %q: not signed in: run `playpresence auth jellyfin --source %s`", name, name)
	}
	if err != nil {
		return nil, fmt.Errorf("source %q: read token: %w", name, err)
	}
	dev, err := jellyfinDevice(st)
	if err != nil {
		return nil, err
	}
	c, err := jellyfin.NewClient(sc.URL, dev, token, nil)
	if err != nil {
		return nil, fmt.Errorf("source %q: %w", name, err)
	}
	return c, nil
}

func buildJellyfin(name string, sc config.SourceConfig, st secrets.Store, poll time.Duration, art bool) (source.Source, error) {
	c, err := jellyfinClient(name, sc, st)
	if err != nil {
		return nil, err
	}
	return jellyfin.New(c, jellyfin.Options{Name: name, User: sc.User, PollInterval: poll, Art: art}), nil
}

func demoSession() session.Session {
	return session.Session{
		Source: "demo", Kind: session.KindEpisode, State: session.StatePlaying,
		Title: "Severance", Season: 1, Episode: 2, EpisodeTitle: "Half Loop",
		IDs:      session.IDs{TMDB: "95396"},
		Position: time.Minute, Duration: 50 * time.Minute,
	}
}
