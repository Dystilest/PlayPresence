package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/Dystilest/PlayPresence/internal/config"
	"github.com/Dystilest/PlayPresence/internal/presence"
	"github.com/Dystilest/PlayPresence/internal/secrets"
	"github.com/Dystilest/PlayPresence/internal/source/jellyfin"
)

const doctorDiscordTimeout = 3 * time.Second

// checkJellyfin probes one Jellyfin source: reachable, then token valid.
func checkJellyfin(ctx context.Context, name string, sc config.SourceConfig, st secrets.Store) (status, msg string) {
	c, err := jellyfinClient(name, sc, st)
	if err != nil {
		return "FAIL", err.Error()
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	info, err := c.PublicInfo(cctx)
	if err != nil {
		return "FAIL", fmt.Sprintf("unreachable (%v)", err)
	}
	u, err := c.Me(cctx)
	switch {
	case errors.Is(err, jellyfin.ErrUnauthorized):
		return "FAIL", fmt.Sprintf("token rejected (run `playpresence auth jellyfin --source %s`)", name)
	case err != nil:
		return "FAIL", fmt.Sprintf("unreachable (%v)", err)
	}
	return "OK", fmt.Sprintf("Jellyfin %s reachable, token valid for %s", info.Version, u.Name)
}

func cmdDoctor(ctx context.Context, args []string, e env) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	cfgPath := fs.String("config", "", "path to the configuration file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	failed := false
	report := func(status, check, msg string) {
		fmt.Fprintf(e.stdout, "%-4s %-8s %s\n", status, check, msg)
		if status == "FAIL" {
			failed = true
		}
	}

	cfg, path, err := loadConfig(*cfgPath, false)
	if err != nil {
		report("FAIL", "config", err.Error())
	} else {
		report("OK", "config", path)
	}

	switch {
	case cfg.Discord.AppID == "":
		report("WARN", "discord", "skipped: discord.app_id is not set")
	default:
		dctx, cancel := context.WithTimeout(ctx, doctorDiscordTimeout)
		client := presence.NewIPCClient(cfg.Discord.AppID, e.dial, e.candidates)
		err := client.Connect(dctx)
		_ = client.Close()
		cancel()
		switch {
		case err == nil:
			report("OK", "discord", "reachable")
		case errors.Is(err, presence.ErrNotRunning):
			report("FAIL", "discord", "Discord is not running (start the desktop app and enable Settings > Activity Privacy > share your activity)")
		default:
			report("FAIL", "discord", fmt.Sprintf("%v (check that discord.app_id is a valid application ID from the Discord Developer Portal)", err))
		}
	}

	names := enabledSources(cfg)
	if len(names) == 0 {
		report("WARN", "sources", "none configured (see docs/configuration.md)")
	}
	var st secrets.Store
	others := 0
	for _, name := range names {
		sc := cfg.Sources[name]
		if sc.Kind != "jellyfin" {
			others++
			continue
		}
		if st == nil {
			var serr error
			if st, serr = openSecrets(e); serr != nil {
				report("FAIL", "source:"+name, serr.Error())
				continue
			}
		}
		status, msg := checkJellyfin(ctx, name, sc, st)
		report(status, "source:"+name, msg)
	}
	if others > 0 {
		report("WARN", "sources", fmt.Sprintf("%d configured for plex or emby; their connectivity checks arrive with those integrations", others))
	}

	if failed {
		return 1
	}
	return 0
}
