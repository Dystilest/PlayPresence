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
