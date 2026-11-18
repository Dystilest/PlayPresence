// Command playpresence shows what you are watching on Discord.
package main

import (
	"bufio"
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

New here? Run "playpresence setup" once, then "playpresence run".

commands:
  setup     connect your media server step by step (flag: --config)
  run       show what you watch on Discord until you stop it (flags: --config, --demo, --app-id)
  start     set up if needed, then run; what a double-click does on Windows (flag: --config)
  doctor    check the configuration, Discord and your servers, and say what to fix (flag: --config)
  auth      sign in to a media server again: auth jellyfin --source NAME (flags: --config, --token-stdin)
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
	args := os.Args[1:]
	// Opened with a double-click: there is no terminal to type commands in,
	// so set up if needed and run, and keep the window open on failure so the
	// message can be read.
	doubleClicked := len(args) == 0 && launchedByDoubleClick()
	if doubleClicked {
		args = []string{"start"}
	}
	code := runWith(ctx, args, defaultEnv(os.Stdout, os.Stderr))
	stop()
	if doubleClicked && code != 0 {
		_, _ = fmt.Fprint(os.Stdout, "\nPress Enter to close this window.")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
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
		_, _ = fmt.Fprintf(e.stdout, "playpresence %s (%s)\n", version.Version, version.Commit)
		return 0
	case "run":
		return cmdRun(ctx, args[1:], e)
	case "setup":
		return cmdSetup(ctx, args[1:], e)
	case "start":
		return cmdStart(ctx, args[1:], e)
	case "doctor":
		return cmdDoctor(ctx, args[1:], e)
	case "auth":
		return cmdAuth(ctx, args[1:], e)
	default:
		_, _ = fmt.Fprintf(e.stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}
