package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Dystilest/PlayPresence/internal/source/jellyfin"
)

func TestSourceErrorReporterPrintsEachDistinctMessageOncePerSource(t *testing.T) {
	var out strings.Builder
	report := sourceErrorReporter(&out)

	report("home", errors.New("server unreachable"))
	report("home", errors.New("server unreachable"))
	report("other", errors.New("server unreachable"))
	report("home", errors.New("socket closed"))
	report("home", errors.New("server unreachable"))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		"playpresence: source home: server unreachable",
		"playpresence: source other: server unreachable",
		"playpresence: source home: socket closed",
		"playpresence: source home: server unreachable",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestSourceErrorReporterExplainsRejectedTokens(t *testing.T) {
	var out strings.Builder

	sourceErrorReporter(&out)("home", fmt.Errorf("jellyfin: websocket: %w", jellyfin.ErrUnauthorized))

	got := out.String()
	if !strings.Contains(got, "token rejected") || !strings.Contains(got, "playpresence auth jellyfin --source home") {
		t.Errorf("got %q", got)
	}
}
