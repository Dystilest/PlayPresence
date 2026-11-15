package jellyfin

import (
	"strings"
	"time"

	"github.com/Dystilest/PlayPresence/internal/session"
)

// ticksToDuration converts Jellyfin ticks (100 ns) to a duration.
func ticksToDuration(ticks int64) time.Duration { return time.Duration(ticks) * 100 }

func supportedType(t string) bool {
	return t == "Movie" || t == "Episode" || t == "Audio"
}

func parseActivity(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// pick chooses the session to display: only sessions playing a supported item,
// restricted to user (case-insensitive) when non-empty, the most recently
// active one first. A session with an unparsable activity date loses to one
// with a valid date.
func pick(sessions []rawSession, user string) (rawSession, bool) {
	var best rawSession
	var bestTime time.Time
	var bestValid, found bool
	for _, rs := range sessions {
		if rs.NowPlayingItem == nil || !supportedType(rs.NowPlayingItem.Type) {
			continue
		}
		if user != "" && !strings.EqualFold(rs.UserName, user) {
			continue
		}
		t, valid := parseActivity(rs.LastActivityDate)
		better := !found ||
			(valid && !bestValid) ||
			(valid && bestValid && t.After(bestTime))
		if better {
			best, bestTime, bestValid, found = rs, t, valid, true
		}
	}
	return best, found
}

// needsSeries reports the series id to look up for an episode, whose own
// provider ids describe the episode and not the series.
func needsSeries(rs rawSession) (string, bool) {
	it := rs.NowPlayingItem
	if it == nil || it.Type != "Episode" || it.SeriesID == "" {
		return "", false
	}
	return it.SeriesID, true
}

// toSession maps a raw session to the source-agnostic model. series holds the
// provider ids of the episode's series (nil or empty when unknown).
func toSession(rs rawSession, series map[string]string, sourceName string) session.Session {
	it := rs.NowPlayingItem
	s := session.Session{
		Source: sourceName,
		State:  session.StatePlaying,
		Title:  it.Name,
		Year:   it.ProductionYear,
		User:   rs.UserName,
	}

	switch it.Type {
	case "Episode":
		s.Kind = session.KindEpisode
		if it.SeriesName != "" {
			s.Title = it.SeriesName
		}
		s.Season, s.Episode, s.EpisodeTitle = it.ParentIndexNumber, it.IndexNumber, it.Name
		s.IDs = session.IDs{TMDB: series["Tmdb"], IMDB: series["Imdb"], TVDB: series["Tvdb"]}
	case "Audio":
		s.Kind = session.KindTrack
		s.Album = it.Album
		if len(it.Artists) > 0 {
			s.Artist = it.Artists[0]
		} else {
			s.Artist = it.AlbumArtist
		}
		s.IDs = session.IDs{MusicBrainz: it.ProviderIDs["MusicBrainzRecording"]}
	default:
		s.Kind = session.KindMovie
		s.IDs = session.IDs{TMDB: it.ProviderIDs["Tmdb"], IMDB: it.ProviderIDs["Imdb"]}
	}

	if rs.PlayState != nil {
		if rs.PlayState.IsPaused {
			s.State = session.StatePaused
		}
		s.Position = ticksToDuration(rs.PlayState.PositionTicks)
	}
	if it.RunTimeTicks > 0 {
		s.Duration = ticksToDuration(it.RunTimeTicks)
	}
	if s.Position < 0 {
		s.Position = 0
	}
	if s.Duration > 0 && s.Position > s.Duration {
		s.Position = s.Duration
	}
	return s
}
