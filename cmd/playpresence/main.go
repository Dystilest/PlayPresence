// Command playpresence shows what you are watching or listening to on Discord.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Dystilest/PlayPresence/internal/version"
)

const usage = "usage: playpresence <command>\n\ncommands:\n  version   print the version\n"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "playpresence %s (%s)\n", version.Version, version.Commit)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}
