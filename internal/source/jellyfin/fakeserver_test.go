package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// recorded is one request seen by the fake server.
type recorded struct {
	Method, Path, RawQuery, Auth string
}

// fakeServer imitates the parts of Jellyfin PlayPresence talks to. Payloads
// are written by hand from the API's JSON shape, not recorded from a server.
type fakeServer struct {
	t      *testing.T
	srv    *httptest.Server
	prefix string

	mu           sync.Mutex
	validToken   string
	sessionsJSON string
	series       map[string]map[string]string // series id -> provider ids
	itemsStatus  int                          // non-zero: /Items answers with it
	sessionsCode int                          // non-zero: /Sessions answers with it
	bigBody      bool                         // /Sessions answers with a 5 MiB body
	requests     []recorded
	qc           quickConnectState
	wsRefused    bool              // /socket refuses the upgrade
	sockets      []*websocket.Conn // connected WebSocket clients
	wsReceived   []string          // text frames received from clients
}

var tokenRE = regexp.MustCompile(`Token="([^"]*)"`)

func newFakeServer(t *testing.T, prefix string) *fakeServer {
	t.Helper()
	f := &fakeServer{
		t: t, prefix: prefix, validToken: "good-token",
		sessionsJSON: "[]", series: map[string]map[string]string{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/System/Info/Public", f.publicInfo)
	mux.HandleFunc("/Sessions", f.auth(f.sessions))
	mux.HandleFunc("/Items", f.auth(f.items))
	mux.HandleFunc("/socket", f.auth(f.socket))
	f.qc.enabled = true
	f.qc.pendingPolls = 1
	f.registerQuickConnect(mux)
	var h http.Handler = mux
	if prefix != "" {
		h = http.StripPrefix(prefix, mux)
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	t.Cleanup(f.dropSockets) // runs before srv.Close: hijacked conns are not tracked by httptest
	return f
}

// socket imitates Jellyfin's /socket: after SessionsStart it pushes the
// current sessions; later pushes are triggered by push().
func (f *fakeServer) socket(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	refused := f.wsRefused
	f.mu.Unlock()
	if refused {
		http.Error(w, "websocket blocked", http.StatusBadGateway)
		return
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.sockets = append(f.sockets, c)
	f.mu.Unlock()
	ctx := context.Background()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.wsReceived = append(f.wsReceived, string(data))
		f.mu.Unlock()
		if strings.Contains(string(data), "SessionsStart") {
			f.push()
		}
	}
}

// push sends the current sessions to every connected WebSocket client.
func (f *fakeServer) push() {
	f.mu.Lock()
	body := f.sessionsJSON
	socks := append([]*websocket.Conn(nil), f.sockets...)
	f.mu.Unlock()
	msg := `{"MessageType":"Sessions","Data":` + body + `}`
	for _, c := range socks {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.Write(ctx, websocket.MessageText, []byte(msg))
		cancel()
	}
}

// sendRaw sends one raw text frame to every connected client.
func (f *fakeServer) sendRaw(msg string) {
	f.mu.Lock()
	socks := append([]*websocket.Conn(nil), f.sockets...)
	f.mu.Unlock()
	for _, c := range socks {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.Write(ctx, websocket.MessageText, []byte(msg))
		cancel()
	}
}

func (f *fakeServer) dropSockets() {
	f.mu.Lock()
	socks := f.sockets
	f.sockets = nil
	f.mu.Unlock()
	for _, c := range socks {
		_ = c.CloseNow()
	}
}

func (f *fakeServer) setWSRefused(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wsRefused = v
}

func (f *fakeServer) setToken(tok string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validToken = tok
}

func (f *fakeServer) received() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.wsReceived...)
}

func (f *fakeServer) count(path string) int {
	n := 0
	for _, r := range f.seen() {
		if r.Path == path {
			n++
		}
	}
	return n
}

func (f *fakeServer) URL() string { return f.srv.URL + f.prefix }

func (f *fakeServer) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")})
}

func (f *fakeServer) seen() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.requests...)
}

func (f *fakeServer) setSessions(js string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessionsJSON = js
}

// auth rejects requests whose Authorization header lacks the valid token.
func (f *fakeServer) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := tokenRE.FindStringSubmatch(r.Header.Get("Authorization"))
		f.mu.Lock()
		ok := m != nil && m[1] == f.validToken
		f.mu.Unlock()
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (f *fakeServer) publicInfo(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"ServerName": "Test Server", "Version": "10.10.0"})
}

func (f *fakeServer) sessions(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	body, code, big := f.sessionsJSON, f.sessionsCode, f.bigBody
	f.mu.Unlock()
	if code != 0 {
		http.Error(w, "boom", code)
		return
	}
	if big {
		_, _ = w.Write([]byte(`[{"Id":"` + strings.Repeat("x", 5<<20) + `"}]`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func (f *fakeServer) items(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	status := f.itemsStatus
	ids := f.series[r.URL.Query().Get("ids")]
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, "boom", status)
		return
	}
	items := []map[string]any{}
	if ids != nil {
		items = append(items, map[string]any{"Id": r.URL.Query().Get("ids"), "ProviderIds": ids})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"Items": items})
}
