package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/Dystilest/PlayPresence/internal/app"
	"github.com/Dystilest/PlayPresence/internal/config"
	"github.com/Dystilest/PlayPresence/internal/presence"
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
		fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}
	if *appID != "" {
		cfg.Discord.AppID = *appID
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}
	if cfg.Discord.AppID == "" {
		fmt.Fprintln(e.stderr, "playpresence: discord.app_id is not set: put it in the config file or pass --app-id")
		return 1
	}

	sources, err := buildSources(cfg, *demo)
	if err != nil {
		fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}

	client := presence.NewIPCClient(cfg.Discord.AppID, e.dial, e.candidates)
	defer client.Close()
	pub := presence.NewPublisher(client, time.Duration(cfg.Presence.MinInterval), presence.RealClock{})
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
		fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
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
		fmt.Fprintf(w, "playpresence: %s\n", msg)
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
		fmt.Fprintf(w, "playpresence: source %s: %s\n", name, msg)
	}
}

func buildSources(cfg config.Config, demo bool) ([]source.Source, error) {
	if demo {
		return []source.Source{fake.New("demo", []fake.Step{{Session: demoSession()}})}, nil
	}
	names := make([]string, 0, len(cfg.Sources))
	for name, s := range cfg.Sources {
		if !s.Disabled {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, errors.New("no sources configured: add a [sources.<name>] section (see docs/configuration.md) or try --demo")
	}
	sort.Strings(names)
	return nil, fmt.Errorf("source %q: the %s integration is not available in this build yet", names[0], cfg.Sources[names[0]].Kind)
}

func demoSession() session.Session {
	return session.Session{
		Source: "demo", Kind: session.KindEpisode, State: session.StatePlaying,
		Title: "Severance", Season: 1, Episode: 2, EpisodeTitle: "Half Loop",
		IDs:      session.IDs{TMDB: "95396"},
		Position: time.Minute, Duration: 50 * time.Minute,
	}
}
