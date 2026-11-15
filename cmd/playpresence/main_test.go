package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Dystilest/PlayPresence/internal/version"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer

	code := run([]string{"version"}, &out, &errOut)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "playpresence ") || !strings.Contains(got, version.Version) {
		t.Fatalf("stdout = %q, want it to contain %q and %q", got, "playpresence ", version.Version)
	}
}

func TestRunUnknown(t *testing.T) {
	var out, errOut bytes.Buffer

	code := run([]string{"nope"}, &out, &errOut)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "usage") {
		t.Fatalf("stderr = %q, want it to contain %q", errOut.String(), "usage")
	}
}

func TestRunNoArgs(t *testing.T) {
	var out, errOut bytes.Buffer

	code := run(nil, &out, &errOut)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
