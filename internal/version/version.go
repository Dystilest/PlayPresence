// Package version exposes build metadata injected at link time.
package version

// Version and Commit are set with -ldflags "-X" at build time.
var (
	Version = "dev"
	Commit  = "none"
)
