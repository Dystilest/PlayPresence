// Command gen writes docs/configuration.md from the Config struct.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Dystilest/PlayPresence/internal/config"
)

func main() {
	out := filepath.Join("docs", "configuration.md")
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(config.Reference()), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
