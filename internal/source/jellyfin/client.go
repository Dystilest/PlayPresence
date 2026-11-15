// Package jellyfin implements a media server source for Jellyfin.
package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	clientName     = "PlayPresence"
	requestTimeout = 10 * time.Second
	maxBody        = 4 << 20 // 4 MiB
)

// ErrUnauthorized means the server rejected the token.
var ErrUnauthorized = errors.New("jellyfin: token rejected")

// Device identifies this installation to the server.
type Device struct {
	Name    string
	ID      string
	Version string
}

// Client is a small read-only Jellyfin REST client.
type Client struct {
	base  *url.URL
	dev   Device
	token string
	hc    *http.Client
}

// NewClient builds a client for the server at baseURL (http or https, with an
// optional path prefix). hc may be nil. Errors never echo baseURL.
func NewClient(baseURL string, dev Device, token string, hc *http.Client) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("jellyfin: server URL must be an http or https address")
	}
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	return &Client{base: u, dev: dev, token: token, hc: hc}, nil
}

// url joins path (without leading slash) under the server's base path.
func (c *Client) url(path string, q url.Values) string {
	u := *c.base
	u.Path = u.Path + "/" + strings.TrimLeft(path, "/")
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// authHeader builds the MediaBrowser Authorization header; the token is
// omitted for anonymous calls.
func (c *Client) authHeader(withToken bool) string {
	clean := func(s string) string { return strings.ReplaceAll(s, `"`, "") }
	parts := []string{}
	if withToken && c.token != "" {
		parts = append(parts, fmt.Sprintf(`Token="%s"`, clean(c.token)))
	}
	parts = append(parts,
		fmt.Sprintf(`Client="%s"`, clientName),
		fmt.Sprintf(`Device="%s"`, clean(c.dev.Name)),
		fmt.Sprintf(`DeviceId="%s"`, clean(c.dev.ID)),
		fmt.Sprintf(`Version="%s"`, clean(c.dev.Version)),
	)
	return "MediaBrowser " + strings.Join(parts, ", ")
}

// do performs a request and decodes the JSON answer into out (may be nil).
// Errors carry the method, path and status but never the token or the host.
func (c *Client) do(ctx context.Context, method, path string, q url.Values, body, out any, withToken bool) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path, q), rd)
	if err != nil {
		return fmt.Errorf("jellyfin: %s %s: %w", method, path, errors.New("invalid request"))
	}
	req.Header.Set("Authorization", c.authHeader(withToken))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("jellyfin: %s %s: request failed: %w", method, path, redact(err))
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("jellyfin: %s %s: %w", method, path, ErrUnauthorized)
	case resp.StatusCode >= 300:
		return fmt.Errorf("jellyfin: %s %s: %w", method, path, statusError(resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return fmt.Errorf("jellyfin: %s %s: read response: %w", method, path, err)
	}
	if len(data) > maxBody {
		return fmt.Errorf("jellyfin: %s %s: response too large", method, path)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("jellyfin: %s %s: decode response: %w", method, path, err)
	}
	return nil
}

// PublicInfo is what the server tells anonymous callers about itself.
type PublicInfo struct {
	ServerName string `json:"ServerName"`
	Version    string `json:"Version"`
}

// PublicInfo calls GET /System/Info/Public without credentials.
func (c *Client) PublicInfo(ctx context.Context) (PublicInfo, error) {
	var info PublicInfo
	err := c.do(ctx, http.MethodGet, "System/Info/Public", nil, nil, &info, false)
	return info, err
}

type rawItem struct {
	ID                string            `json:"Id"`
	Name              string            `json:"Name"`
	Type              string            `json:"Type"`
	SeriesName        string            `json:"SeriesName"`
	SeriesID          string            `json:"SeriesId"`
	SeasonID          string            `json:"SeasonId"`
	ParentIndexNumber int               `json:"ParentIndexNumber"`
	IndexNumber       int               `json:"IndexNumber"`
	ProductionYear    int               `json:"ProductionYear"`
	RunTimeTicks      int64             `json:"RunTimeTicks"`
	ProviderIDs       map[string]string `json:"ProviderIds"`
	Artists           []string          `json:"Artists"`
	Album             string            `json:"Album"`
	AlbumArtist       string            `json:"AlbumArtist"`
	AlbumID           string            `json:"AlbumId"`
}

type rawPlayState struct {
	PositionTicks int64 `json:"PositionTicks"`
	IsPaused      bool  `json:"IsPaused"`
}

type rawSession struct {
	ID               string        `json:"Id"`
	UserID           string        `json:"UserId"`
	UserName         string        `json:"UserName"`
	Client           string        `json:"Client"`
	DeviceName       string        `json:"DeviceName"`
	LastActivityDate string        `json:"LastActivityDate"`
	NowPlayingItem   *rawItem      `json:"NowPlayingItem"`
	PlayState        *rawPlayState `json:"PlayState"`
}

// Sessions calls GET /Sessions.
func (c *Client) Sessions(ctx context.Context) ([]rawSession, error) {
	var out []rawSession
	err := c.do(ctx, http.MethodGet, "Sessions", nil, nil, &out, true)
	return out, err
}

// SeriesIDs returns the provider ids (Tmdb, Imdb, Tvdb...) of a series, or an
// empty map when the series is unknown.
func (c *Client) SeriesIDs(ctx context.Context, userID, seriesID string) (map[string]string, error) {
	q := url.Values{"ids": {seriesID}, "fields": {"ProviderIds"}}
	if userID != "" {
		q.Set("userId", userID)
	}
	var out struct {
		Items []struct {
			ProviderIDs map[string]string `json:"ProviderIds"`
		} `json:"Items"`
	}
	if err := c.do(ctx, http.MethodGet, "Items", q, nil, &out, true); err != nil {
		return nil, err
	}
	if len(out.Items) == 0 || out.Items[0].ProviderIDs == nil {
		return map[string]string{}, nil
	}
	return out.Items[0].ProviderIDs, nil
}

// User is the account a token belongs to.
type User struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

// Me calls GET /Users/Me: the owner of the client's token.
func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	err := c.do(ctx, http.MethodGet, "Users/Me", nil, nil, &u, true)
	return u, err
}

// statusError is an unexpected HTTP status; it carries no host or token.
type statusError int

func (s statusError) Error() string { return fmt.Sprintf("unexpected status %d", int(s)) }

// maxArtURL is the longest image URL Discord accepts.
const maxArtURL = 256

// PosterURL asks the server for the public poster URLs it knows for an item
// (GET /Items/{id}/RemoteImages) and returns the first https one, or "" when
// there is none. These are third-party image hosts, never the server itself.
func (c *Client) PosterURL(ctx context.Context, itemID string) (string, error) {
	q := url.Values{"type": {"Primary"}, "limit": {"10"}}
	var out struct {
		Images []struct {
			URL string `json:"Url"`
		} `json:"Images"`
	}
	if err := c.do(ctx, http.MethodGet, "Items/"+url.PathEscape(itemID)+"/RemoteImages", q, nil, &out, true); err != nil {
		return "", err
	}
	for _, im := range out.Images {
		if strings.HasPrefix(im.URL, "https://") && len(im.URL) <= maxArtURL {
			return im.URL, nil
		}
	}
	return "", nil
}
