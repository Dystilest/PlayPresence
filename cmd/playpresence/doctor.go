package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/Dystilest/PlayPresence/internal/presence"
)

const doctorDiscordTimeout = 3 * time.Second

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

	enabled := 0
	for _, s := range cfg.Sources {
		if !s.Disabled {
			enabled++
		}
	}
	if enabled == 0 {
		report("WARN", "sources", "none configured (see docs/configuration.md)")
	} else {
		report("WARN", "sources", fmt.Sprintf("%d configured; connectivity checks arrive with the server integrations", enabled))
	}

	if failed {
		return 1
	}
	return 0
}
