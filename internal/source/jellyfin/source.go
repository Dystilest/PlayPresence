package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/coder/websocket"

	"github.com/Dystilest/PlayPresence/internal/session"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultRefresh      = 30 * time.Second
	readIdleTimeout     = 90 * time.Second // server sends ForceKeepAlive every ~60 s
	seriesRetryAfter    = 30 * time.Second
)

// Options configures a Source.
type Options struct {
	Name         string        // source name shown in sessions (default "jellyfin")
	User         string        // user name to show; empty means the token's own user, "*" every visible session
	PollInterval time.Duration // REST polling period when WebSocket is unavailable (default 5 s)
	Refresh      time.Duration // heartbeat re-send period (default 30 s)
	Art          bool          // ask the server for public poster URLs
}

// Source watches a Jellyfin server over WebSocket, falling back to polling.
type Source struct {
	c   *Client
	opt Options
	now func() time.Time

	series       map[string]map[string]string // series id -> provider ids
	failed       map[string]time.Time         // series id -> last failed lookup
	posters      map[string]string            // item id -> poster URL ("" when none)
	posterFailed map[string]time.Time
	shown        string // Id of the session currently displayed
	owner        string // user id of the token (empty: unknown, e.g. an API key)
}

// New builds a Source around c.
func New(c *Client, opt Options) *Source {
	if opt.Name == "" {
		opt.Name = "jellyfin"
	}
	if opt.PollInterval <= 0 {
		opt.PollInterval = defaultPollInterval
	}
	if opt.Refresh <= 0 {
		opt.Refresh = defaultRefresh
	}
	return &Source{
		c: c, opt: opt, now: time.Now,
		series:  map[string]map[string]string{},
		failed:  map[string]time.Time{},
		posters: map[string]string{}, posterFailed: map[string]time.Time{},
	}
}

// Name returns the source name.
func (s *Source) Name() string { return s.opt.Name }

// redact drops the request URL and the dialed address (so the host) from
// network errors.
func redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Err != nil {
		return fmt.Errorf("%s: %w", oe.Op, oe.Err)
	}
	return err
}

// Watch implements source.Source.
func (s *Source) Watch(ctx context.Context, out chan<- session.Session) error {
	em := newEmitter(s.now, s.opt.Refresh)
	if err := s.resolveOwner(ctx); err != nil {
		return err
	}

	conn, resp, err := websocket.Dial(ctx, s.c.url("socket", nil), &websocket.DialOptions{
		HTTPClient: s.c.hc,
		HTTPHeader: map[string][]string{"Authorization": {s.c.authHeader(true)}},
	})
	if err != nil {
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return fmt.Errorf("jellyfin: websocket: %w", ErrUnauthorized)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.poll(ctx, out, em) // WebSocket unavailable: poll instead
	}
	defer func() { _ = conn.CloseNow() }()
	return s.listen(ctx, conn, out, em)
}

func (s *Source) listen(ctx context.Context, conn *websocket.Conn, out chan<- session.Session, em *emitter) error {
	conn.SetReadLimit(maxBody)
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"MessageType":"SessionsStart","Data":"0,1500"}`)); err != nil {
		return s.socketErr(ctx, err)
	}
	for {
		rctx, cancel := context.WithTimeout(ctx, readIdleTimeout)
		_, data, err := conn.Read(rctx)
		cancel()
		if err != nil {
			return s.socketErr(ctx, err)
		}
		var msg struct {
			MessageType string          `json:"MessageType"`
			Data        json.RawMessage `json:"Data"`
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.MessageType {
		case "ForceKeepAlive":
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"MessageType":"KeepAlive"}`)); err != nil {
				return s.socketErr(ctx, err)
			}
		case "Sessions":
			var sessions []rawSession
			if json.Unmarshal(msg.Data, &sessions) != nil {
				continue
			}
			if err := s.handle(ctx, em, sessions, out); err != nil {
				return err
			}
		}
	}
}

func (s *Source) socketErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("jellyfin: websocket: %w", redact(err))
}

func (s *Source) poll(ctx context.Context, out chan<- session.Session, em *emitter) error {
	t := time.NewTicker(s.opt.PollInterval)
	defer t.Stop()
	for {
		sessions, err := s.c.Sessions(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if err := s.handle(ctx, em, sessions, out); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// handle maps the visible sessions to at most one update and sends it.
func (s *Source) handle(ctx context.Context, em *emitter, sessions []rawSession, out chan<- session.Session) error {
	var cur *session.Session
	s.shown = ""
	if rs, ok := pick(s.visible(sessions), s.filterName(), s.shown); ok {
		var series map[string]string
		if id, need := needsSeries(rs); need {
			series = s.seriesIDs(ctx, rs.UserID, id)
		}
		s.shown = rs.ID
		v := toSession(rs, series, s.opt.Name)
		if s.opt.Art {
			for _, id := range artItemIDs(rs) {
				if v.ArtURL = s.poster(ctx, id); v.ArtURL != "" {
					break
				}
			}
		}
		cur = &v
	}
	sess, ok := em.next(cur)
	if !ok {
		return nil
	}
	select {
	case out <- sess:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// seriesIDs returns the series provider ids, cached per series. A failed
// lookup is not cached as a result and is retried after seriesRetryAfter.
func (s *Source) seriesIDs(ctx context.Context, userID, seriesID string) map[string]string {
	if ids, ok := s.series[seriesID]; ok {
		return ids
	}
	if at, bad := s.failed[seriesID]; bad && s.now().Sub(at) < seriesRetryAfter {
		return nil
	}
	ids, err := s.c.SeriesIDs(ctx, userID, seriesID)
	if err != nil {
		s.failed[seriesID] = s.now()
		return nil
	}
	delete(s.failed, seriesID)
	s.series[seriesID] = ids
	return ids
}

// resolveOwner learns which user the token belongs to, so that by default only
// that user's playback is shown. An API key has no user: the owner stays
// unknown and every visible session is considered.
func (s *Source) resolveOwner(ctx context.Context) error {
	s.owner = ""
	if s.opt.User != "" {
		return nil // an explicit filter (or "*") decides
	}
	u, err := s.c.Me(ctx)
	switch {
	case errors.Is(err, ErrUnauthorized):
		return fmt.Errorf("jellyfin: %w", ErrUnauthorized)
	case err != nil:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var se statusError
		if errors.As(err, &se) {
			return nil // no user behind this token
		}
		return err
	}
	s.owner = u.ID
	return nil
}

// filterName is the user name to match; "*" means any user.
func (s *Source) filterName() string {
	if s.opt.User == "*" {
		return ""
	}
	return s.opt.User
}

// visible keeps the token owner's sessions when the owner is known.
func (s *Source) visible(sessions []rawSession) []rawSession {
	if s.owner == "" {
		return sessions
	}
	var own []rawSession
	for _, rs := range sessions {
		if rs.UserID == s.owner {
			own = append(own, rs)
		}
	}
	return own
}

// artItemIDs lists the items whose poster can represent the playback, best
// first: season, series then episode for an episode; the album for a track;
// the item itself for a movie.
func artItemIDs(rs rawSession) []string {
	it := rs.NowPlayingItem
	switch it.Type {
	case "Episode":
		var ids []string
		for _, id := range []string{it.SeasonID, it.SeriesID, it.ID} {
			if id != "" {
				ids = append(ids, id)
			}
		}
		return ids
	case "Audio":
		if it.AlbumID != "" {
			return []string{it.AlbumID}
		}
		return nil
	}
	return []string{it.ID}
}

// poster returns the cached poster URL of an item, "" when unknown. A failed
// lookup is retried after seriesRetryAfter.
func (s *Source) poster(ctx context.Context, itemID string) string {
	if u, ok := s.posters[itemID]; ok {
		return u
	}
	if at, bad := s.posterFailed[itemID]; bad && s.now().Sub(at) < seriesRetryAfter {
		return ""
	}
	u, err := s.c.PosterURL(ctx, itemID)
	if err != nil {
		s.posterFailed[itemID] = s.now()
		return ""
	}
	delete(s.posterFailed, itemID)
	s.posters[itemID] = u
	return u
}
