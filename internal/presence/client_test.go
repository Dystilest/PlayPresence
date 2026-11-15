package presence

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Dystilest/PlayPresence/internal/render"
)

// fakeDiscord serves one in-memory IPC connection and records what it receives.
type fakeDiscord struct {
	t        *testing.T
	mu       sync.Mutex
	received []map[string]any
	reply    func(cmd map[string]any) map[string]any // nil -> default OK
	silent   bool                                    // never answers
	refuse   bool                                    // answers the handshake with an ERROR event
}

func (f *fakeDiscord) dial(string) (net.Conn, error) {
	client, server := net.Pipe()
	go f.serve(server)
	return client, nil
}

func (f *fakeDiscord) serve(c net.Conn) {
	defer c.Close()
	for {
		op, body, err := DecodeFrame(c)
		if err != nil {
			return
		}
		var msg map[string]any
		if err := json.Unmarshal(body, &msg); err != nil {
			return
		}
		f.mu.Lock()
		f.received = append(f.received, msg)
		f.mu.Unlock()
		if f.silent {
			continue
		}

		var resp map[string]any
		switch {
		case op == OpHandshake && f.refuse:
			resp = map[string]any{"evt": "ERROR", "data": map[string]any{"code": 4000, "message": "Invalid Client ID"}}
		case op == OpHandshake:
			resp = map[string]any{"evt": "READY", "cmd": "DISPATCH", "data": map[string]any{"user": map[string]any{"username": "tester"}}}
		case f.reply != nil:
			resp = f.reply(msg)
		default:
			resp = map[string]any{"cmd": msg["cmd"], "evt": nil, "nonce": msg["nonce"]}
		}
		raw, _ := EncodeFrame(OpFrame, resp)
		if _, err := c.Write(raw); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			return
		}
	}
}

func (f *fakeDiscord) messages() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.received...)
}

func newTestClient(f *fakeDiscord) *IPCClient {
	return NewIPCClient("1234567890", f.dial, []string{"fake-0"})
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestSetActivityHandshakeAndPayload(t *testing.T) {
	f := &fakeDiscord{t: t}
	c := newTestClient(f)
	defer c.Close()
	start := time.UnixMilli(1_700_000_000_000)
	end := start.Add(90 * time.Minute)
	a := render.Activity{
		Type: render.TypeWatching, StatusDisplay: render.StatusDisplayDetails,
		Details: "Heat (1995)", State: "", LargeImage: "https://img.example/h.jpg", LargeText: "Heat",
		Start: &start, End: &end,
		Buttons: []render.Button{{Label: "TMDb", URL: "https://www.themoviedb.org/movie/949"}},
	}

	if err := c.SetActivity(ctx(t), a); err != nil {
		t.Fatalf("SetActivity: %v", err)
	}

	msgs := f.messages()
	if len(msgs) != 2 {
		t.Fatalf("server received %d messages, want 2 (handshake + set activity)", len(msgs))
	}
	if msgs[0]["client_id"] != "1234567890" || msgs[0]["v"] != float64(1) {
		t.Errorf("handshake = %v", msgs[0])
	}
	if msgs[1]["cmd"] != "SET_ACTIVITY" {
		t.Fatalf("cmd = %v", msgs[1]["cmd"])
	}
	args := msgs[1]["args"].(map[string]any)
	if args["pid"] == nil {
		t.Error("pid missing")
	}
	act := args["activity"].(map[string]any)
	if act["type"] != float64(3) || act["status_display_type"] != float64(2) {
		t.Errorf("type/status_display_type = %v/%v", act["type"], act["status_display_type"])
	}
	if act["details"] != "Heat (1995)" {
		t.Errorf("details = %v", act["details"])
	}
	ts := act["timestamps"].(map[string]any)
	if ts["start"] != float64(start.UnixMilli()) || ts["end"] != float64(end.UnixMilli()) {
		t.Errorf("timestamps must be milliseconds, got %v", ts)
	}
	assets := act["assets"].(map[string]any)
	if assets["large_image"] != "https://img.example/h.jpg" || assets["large_text"] != "Heat" {
		t.Errorf("assets = %v", assets)
	}
	buttons := act["buttons"].([]any)
	if len(buttons) != 1 || buttons[0].(map[string]any)["label"] != "TMDb" {
		t.Errorf("buttons = %v", buttons)
	}
	if _, has := act["state"]; has {
		t.Error("empty state must be omitted")
	}
}

func TestConnectOnlyPerformsTheHandshake(t *testing.T) {
	f := &fakeDiscord{t: t}
	c := newTestClient(f)
	defer c.Close()

	if err := c.Connect(ctx(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	msgs := f.messages()
	if len(msgs) != 1 || msgs[0]["client_id"] != "1234567890" {
		t.Fatalf("server received %v, want only the handshake", msgs)
	}
}

func TestRefusedHandshakeIsNotReportedAsNotRunning(t *testing.T) {
	f := &fakeDiscord{t: t, refuse: true}
	c := newTestClient(f)
	defer c.Close()

	err := c.Connect(ctx(t))

	if err == nil || errors.Is(err, ErrNotRunning) {
		t.Fatalf("err = %v, want a handshake error distinct from ErrNotRunning", err)
	}
	if !contains(err.Error(), "Invalid Client ID") {
		t.Errorf("err = %v, want it to carry Discord's reason", err)
	}
}

func TestConnectWithoutDiscordReturnsErrNotRunning(t *testing.T) {
	c := NewIPCClient("1", func(string) (net.Conn, error) { return nil, errors.New("no pipe") }, []string{"a"})

	if err := c.Connect(ctx(t)); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("err = %v, want ErrNotRunning", err)
	}
}

func TestRejectedActivityIsReportedAndNotRetriedOnAFreshConnection(t *testing.T) {
	calls := 0
	f := &fakeDiscord{t: t, reply: func(msg map[string]any) map[string]any {
		calls++
		if calls == 1 {
			return map[string]any{"cmd": msg["cmd"], "evt": nil}
		}
		return map[string]any{"evt": "ERROR", "data": map[string]any{"code": 4000, "message": "bad details"}}
	}}
	c := newTestClient(f)
	defer c.Close()
	if err := c.SetActivity(ctx(t), render.Activity{Details: "fine"}); err != nil {
		t.Fatal(err)
	}

	err := c.SetActivity(ctx(t), render.Activity{Details: "refused"})

	var rej *RejectedError
	if !errors.As(err, &rej) || rej.Message != "bad details" {
		t.Fatalf("err = %v, want a *RejectedError carrying Discord's message", err)
	}
	setCalls, handshakes := 0, 0
	for _, m := range f.messages() {
		if m["cmd"] == "SET_ACTIVITY" {
			setCalls++
		}
		if m["client_id"] != nil {
			handshakes++
		}
	}
	if setCalls != 2 || handshakes != 1 {
		t.Errorf("SET_ACTIVITY=%d handshakes=%d, want 2 and 1: a rejection must not trigger a reconnect and resend", setCalls, handshakes)
	}
}

func TestClearSendsNullActivity(t *testing.T) {
	f := &fakeDiscord{t: t}
	c := newTestClient(f)
	defer c.Close()

	if err := c.Clear(ctx(t)); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	msgs := f.messages()
	args := msgs[len(msgs)-1]["args"].(map[string]any)
	if v, has := args["activity"]; !has || v != nil {
		t.Errorf("activity = %v (present=%v), want explicit null", v, has)
	}
}

func TestDiscordErrorEventIsReturned(t *testing.T) {
	f := &fakeDiscord{t: t, reply: func(map[string]any) map[string]any {
		return map[string]any{"evt": "ERROR", "data": map[string]any{"code": 4000, "message": "bad activity"}}
	}}
	c := newTestClient(f)
	defer c.Close()

	err := c.SetActivity(ctx(t), render.Activity{Details: "x"})

	if err == nil || !contains(err.Error(), "bad activity") {
		t.Fatalf("err = %v, want it to carry Discord's message", err)
	}
}

func TestNoPipeReturnsErrNotRunning(t *testing.T) {
	c := NewIPCClient("1", func(string) (net.Conn, error) { return nil, errors.New("no such pipe") }, []string{"a", "b"})

	err := c.SetActivity(ctx(t), render.Activity{Details: "x"})

	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("err = %v, want ErrNotRunning", err)
	}
}

func TestSilentServerTimesOut(t *testing.T) {
	f := &fakeDiscord{t: t, silent: true}
	c := newTestClient(f)
	defer c.Close()
	short, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := c.SetActivity(short, render.Activity{Details: "x"})

	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %v, want it bounded by the context deadline", time.Since(start))
	}
}

func TestReconnectsAfterConnectionLoss(t *testing.T) {
	f := &fakeDiscord{t: t}
	c := newTestClient(f)
	defer c.Close()
	if err := c.SetActivity(ctx(t), render.Activity{Details: "one"}); err != nil {
		t.Fatal(err)
	}
	c.dropConnectionForTest()

	if err := c.SetActivity(ctx(t), render.Activity{Details: "two"}); err != nil {
		t.Fatalf("second SetActivity after loss: %v", err)
	}

	handshakes := 0
	for _, m := range f.messages() {
		if m["client_id"] != nil {
			handshakes++
		}
	}
	if handshakes != 2 {
		t.Errorf("handshakes = %d, want 2 (one per connection)", handshakes)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
