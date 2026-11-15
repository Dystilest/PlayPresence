package jellyfin

import (
	"time"

	"github.com/Dystilest/PlayPresence/internal/session"
)

// driftTolerance is how far the reported position may stray from wall-clock
// progress before the change counts as a seek.
const driftTolerance = 3 * time.Second

// emitter turns a stream of polled snapshots into the updates worth sending:
// real changes (item, pause/resume, seek), a periodic heartbeat, and one stop.
type emitter struct {
	now     func() time.Time
	refresh time.Duration

	last   *session.Session
	sentAt time.Time
}

func newEmitter(now func() time.Time, refresh time.Duration) *emitter {
	return &emitter{now: now, refresh: refresh}
}

// next takes the current snapshot (nil when nothing is playing) and returns
// the session to send, or ok=false when nothing should be sent.
func (e *emitter) next(s *session.Session) (session.Session, bool) {
	now := e.now()
	if s == nil {
		if e.last == nil {
			return session.Session{}, false
		}
		stopped := *e.last
		stopped.State = session.StateStopped
		e.last = nil
		return stopped, true
	}
	if e.last == nil || e.changed(*s, now) {
		cp := *s
		e.last, e.sentAt = &cp, now
		return cp, true
	}
	return session.Session{}, false
}

func (e *emitter) changed(s session.Session, now time.Time) bool {
	l := *e.last
	if s.State != l.State {
		return true
	}
	// Everything but the playback position and state identifies the item.
	a, b := s, l
	a.Position, b.Position = 0, 0
	if a != b {
		return true
	}
	expected := l.Position
	if s.State == session.StatePlaying {
		expected += now.Sub(e.sentAt)
	}
	drift := s.Position - expected
	if drift < 0 {
		drift = -drift
	}
	if drift > driftTolerance {
		return true
	}
	return now.Sub(e.sentAt) >= e.refresh
}
