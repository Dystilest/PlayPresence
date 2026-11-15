package presence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/Dystilest/PlayPresence/internal/render"
)

// ErrNotRunning means no Discord client could be reached on any known endpoint.
var ErrNotRunning = errors.New("presence: discord is not running")

const defaultTimeout = 5 * time.Second

// Client publishes and clears the user's Discord activity.
type Client interface {
	SetActivity(ctx context.Context, a render.Activity) error
	Clear(ctx context.Context) error
	Close() error
}

// IPCClient implements Client over Discord's local IPC protocol.
type IPCClient struct {
	appID      string
	dial       func(path string) (net.Conn, error)
	candidates []string

	mu    sync.Mutex
	conn  net.Conn
	nonce int
}

// NewIPCClient builds a client that tries each candidate endpoint in order.
func NewIPCClient(appID string, dial func(path string) (net.Conn, error), candidates []string) *IPCClient {
	return &IPCClient{appID: appID, dial: dial, candidates: candidates}
}

// NewDefaultClient builds a client for the running OS using the standard endpoints.
func NewDefaultClient(appID string) *IPCClient {
	return NewIPCClient(appID, DefaultDial, PipeCandidates(runtime.GOOS, os.Getenv))
}

// SetActivity publishes a.
func (c *IPCClient) SetActivity(ctx context.Context, a render.Activity) error {
	return c.send(ctx, toPayload(a))
}

// Clear removes the activity.
func (c *IPCClient) Clear(ctx context.Context) error {
	return c.send(ctx, nil)
}

// Close drops the connection to Discord.
func (c *IPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnect()
}

// dropConnectionForTest simulates a connection lost behind the client's back.
func (c *IPCClient) dropConnectionForTest() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *IPCClient) send(ctx context.Context, activity *activityPayload) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	deadline := time.Now().Add(defaultTimeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}

	reused := c.conn != nil
	if !reused {
		if err := c.connect(deadline); err != nil {
			return err
		}
	}
	err := c.roundTrip(deadline, activity)
	if err != nil && reused && ctx.Err() == nil {
		// The connection may have gone stale (Discord restarted): retry once on a fresh one.
		_ = c.disconnect()
		if cerr := c.connect(deadline); cerr != nil {
			return cerr
		}
		err = c.roundTrip(deadline, activity)
	}
	return err
}

func (c *IPCClient) connect(deadline time.Time) error {
	for _, path := range c.candidates {
		conn, err := c.dial(path)
		if err != nil {
			continue
		}
		_ = conn.SetDeadline(deadline)
		hs, _ := EncodeFrame(OpHandshake, map[string]any{"v": 1, "client_id": c.appID})
		if _, err := conn.Write(hs); err != nil {
			_ = conn.Close()
			continue
		}
		resp, err := readResponse(conn)
		if err != nil || resp.Evt != "READY" {
			_ = conn.Close()
			continue
		}
		c.conn = conn
		return nil
	}
	return ErrNotRunning
}

func (c *IPCClient) disconnect() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *IPCClient) roundTrip(deadline time.Time, activity *activityPayload) error {
	_ = c.conn.SetDeadline(deadline)

	c.nonce++
	cmd := map[string]any{
		"cmd":   "SET_ACTIVITY",
		"nonce": strconv.Itoa(c.nonce),
		"args":  map[string]any{"pid": os.Getpid(), "activity": activity},
	}
	raw, err := EncodeFrame(OpFrame, cmd)
	if err != nil {
		return err
	}
	if _, err := c.conn.Write(raw); err != nil {
		_ = c.disconnect()
		return fmt.Errorf("presence: write: %w", err)
	}
	resp, err := readResponse(c.conn)
	if err != nil {
		_ = c.disconnect()
		return err
	}
	if resp.Evt == "ERROR" {
		return fmt.Errorf("presence: discord error %d: %s", resp.Data.Code, resp.Data.Message)
	}
	return nil
}

type response struct {
	Evt  string `json:"evt"`
	Data struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"data"`
}

// readResponse reads frames until a regular response arrives, answering pings.
func readResponse(conn net.Conn) (response, error) {
	for {
		op, body, err := DecodeFrame(conn)
		if err != nil {
			return response{}, err
		}
		switch op {
		case OpPing:
			pong, _ := EncodeFrame(OpPong, json.RawMessage(body))
			if _, err := conn.Write(pong); err != nil {
				return response{}, fmt.Errorf("presence: write pong: %w", err)
			}
		case OpClose:
			return response{}, errors.New("presence: discord closed the connection")
		default:
			var r response
			if err := json.Unmarshal(body, &r); err != nil {
				return response{}, fmt.Errorf("presence: decode response: %w", err)
			}
			return r, nil
		}
	}
}

type activityPayload struct {
	Type              int         `json:"type"`
	StatusDisplayType int         `json:"status_display_type"`
	Details           string      `json:"details,omitempty"`
	State             string      `json:"state,omitempty"`
	Timestamps        *timestamps `json:"timestamps,omitempty"`
	Assets            *assets     `json:"assets,omitempty"`
	Buttons           []button    `json:"buttons,omitempty"`
}

type timestamps struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

type assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

type button struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

func toPayload(a render.Activity) *activityPayload {
	p := &activityPayload{
		Type:              a.Type,
		StatusDisplayType: a.StatusDisplay,
		Details:           a.Details,
		State:             a.State,
	}
	if a.Start != nil || a.End != nil {
		ts := &timestamps{}
		if a.Start != nil {
			ts.Start = a.Start.UnixMilli()
		}
		if a.End != nil {
			ts.End = a.End.UnixMilli()
		}
		p.Timestamps = ts
	}
	if a.LargeImage != "" || a.LargeText != "" || a.SmallImage != "" || a.SmallText != "" {
		p.Assets = &assets{LargeImage: a.LargeImage, LargeText: a.LargeText, SmallImage: a.SmallImage, SmallText: a.SmallText}
	}
	for _, b := range a.Buttons {
		p.Buttons = append(p.Buttons, button{Label: b.Label, URL: b.URL})
	}
	return p
}
