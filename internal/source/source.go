// Package source defines what a media server integration must provide.
package source

import (
	"context"

	"github.com/Dystilest/PlayPresence/internal/session"
)

// Source watches one media server (or one account on it).
//
// Watch sends a session on out every time something changes (state, position
// jump, new item) and blocks until ctx is cancelled or the connection fails.
// A session in the stopped state means nothing is playing any more. A
// non-nil error return means the source lost its backend; the caller restarts it.
type Source interface {
	Name() string
	Watch(ctx context.Context, out chan<- session.Session) error
}

// Func adapts a function to Source; handy for tests and tiny sources.
type Func struct {
	ID string
	Fn func(ctx context.Context, out chan<- session.Session) error
}

// Name returns the source name.
func (f Func) Name() string { return f.ID }

// Watch runs the wrapped function.
func (f Func) Watch(ctx context.Context, out chan<- session.Session) error { return f.Fn(ctx, out) }
