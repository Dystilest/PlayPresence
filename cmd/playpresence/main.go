// Command playpresence shows what you are watching or listening to on Discord.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Dystilest/PlayPresence/internal/presence"
	"github.com/Dystilest/PlayPresence/internal/secrets"
	"github.com/Dystilest/PlayPresence/internal/version"
)

const usage = `usage: playpresence <command> [flags]

commands:
  run       watch your media servers and publish the presence (flags: --config, --demo, --app-id)
  doctor    check the configuration, Discord and sources (flag: --config)
  auth      sign in to a media server: auth jellyfin --source NAME (flags: --config, --token-stdin)
  version   print the version
`

// env holds everything the commands need from the outside world, so tests can
// replace it.
type env struct {
	stdout, stderr io.Writer
	stdin          io.Reader
	dial           func(path string) (net.Conn, error)
	candidates     []string
	secrets        secrets.Store // nil: opened on demand (OS keyring or encrypted file)
	qcPoll         time.Duration // Quick Connect poll period; zero means the default
	pollInterval   time.Duration // Jellyfin REST polling period; zero means the default
	minInterval    time.Duration // overrides presence.min_interval (tests only)
}

func defaultEnv(stdout, stderr io.Writer) env {
	return env{
		stdout:     stdout,
		stderr:     stderr,
		stdin:      os.Stdin,
		dial:       presence.DefaultDial,
		candidates: presence.PipeCandidates(runtime.GOOS, os.Getenv),
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := runWith(ctx, os.Args[1:], defaultEnv(os.Stdout, os.Stderr))
	stop()
	os.Exit(code)
}

// run executes the CLI with the real environment and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	return runWith(context.Background(), args, defaultEnv(stdout, stderr))
}

// runWith dispatches a command. Exit codes: 0 ok, 1 runtime error, 2 usage error.
func runWith(ctx context.Context, args []string, e env) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(e.stdout, "playpresence %s (%s)\n", version.Version, version.Commit)
		return 0
	case "run":
		return cmdRun(ctx, args[1:], e)
	case "doctor":
		return cmdDoctor(ctx, args[1:], e)
	case "auth":
		return cmdAuth(ctx, args[1:], e)
	default:
		fmt.Fprintf(e.stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}
