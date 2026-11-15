// Package fake provides a scripted source for demos and tests.
package fake

import (
	"context"
	"time"

	"github.com/Dystilest/PlayPresence/internal/session"
)

// Step is one scripted event: wait Delay, then fail with Err or emit Session.
type Step struct {
	Delay   time.Duration
	Session session.Session
	Err     error
}

// Source plays a script and then idles until cancelled.
type Source struct {
	name   string
	script []Step
}

// New returns a source named name that plays script.
func New(name string, script []Step) *Source { return &Source{name: name, script: script} }

// Name returns the source name.
func (s *Source) Name() string { return s.name }

// Watch plays the script, then blocks until ctx is cancelled.
func (s *Source) Watch(ctx context.Context, out chan<- session.Session) error {
	for _, step := range s.script {
		if step.Delay > 0 {
			select {
			case <-time.After(step.Delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if step.Err != nil {
			return step.Err
		}
		select {
		case out <- step.Session:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	<-ctx.Done()
	return ctx.Err()
}
