package jellyfin

import (
	"encoding/json"
	"net/http"
)

// quickConnectState drives the fake Quick Connect endpoints (guarded by
// fakeServer.mu).
type quickConnectState struct {
	enabled         bool
	pendingPolls    int // Connect answers "not authenticated" this many times
	polls           int
	initiated       int
	initiateGetOnly bool // POST /Initiate answers 405 (older servers)
	authStatus      int  // non-zero: AuthenticateWithQuickConnect answers with it
}

const (
	qcSecret = "SECRET-qc-0123456789"
	qcCode   = "424242"
)

func (f *fakeServer) registerQuickConnect(mux *http.ServeMux) {
	mux.HandleFunc("/QuickConnect/Enabled", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		en := f.qc.enabled
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(en)
	})
	mux.HandleFunc("/QuickConnect/Initiate", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		getOnly := f.qc.initiateGetOnly
		f.mu.Unlock()
		if (getOnly && r.Method != http.MethodGet) || (!getOnly && r.Method != http.MethodPost) {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		f.mu.Lock()
		f.qc.initiated++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"Secret": qcSecret, "Code": qcCode, "Authenticated": false})
	})
	mux.HandleFunc("/QuickConnect/Connect", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secret") != qcSecret {
			http.Error(w, "unknown secret", http.StatusNotFound)
			return
		}
		f.mu.Lock()
		f.qc.polls++
		done := f.qc.polls > f.qc.pendingPolls
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"Secret": qcSecret, "Code": qcCode, "Authenticated": done})
	})
	mux.HandleFunc("/Users/AuthenticateWithQuickConnect", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Secret string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		status := f.qc.authStatus
		f.mu.Unlock()
		if status != 0 {
			http.Error(w, "denied", status)
			return
		}
		if body.Secret != qcSecret {
			http.Error(w, "bad secret", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"AccessToken": "issued-token",
			"User":        map[string]string{"Id": "u1", "Name": "Samy"},
		})
	})
}
