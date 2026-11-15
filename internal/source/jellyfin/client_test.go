package jellyfin

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

var testDevice = Device{Name: "My PC é", ID: "dev-123", Version: "0.1.0"}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func client(t *testing.T, f *fakeServer, token string) *Client {
	t.Helper()
	c, err := NewClient(f.URL(), testDevice, token, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestPublicInfo(t *testing.T) {
	f := newFakeServer(t, "")

	info, err := client(t, f, "").PublicInfo(ctx(t))

	if err != nil {
		t.Fatalf("PublicInfo: %v", err)
	}
	if info.ServerName != "Test Server" || info.Version != "10.10.0" {
		t.Errorf("info = %+v", info)
	}
	if auth := f.seen()[0].Auth; strings.Contains(auth, "Token=") {
		t.Errorf("PublicInfo must be anonymous, got Authorization %q", auth)
	}
}

func TestSessionsSendsMediaBrowserHeaderAndNoTokenInURL(t *testing.T) {
	f := newFakeServer(t, "")
	f.setSessions(`[{"Id":"s1","UserName":"samy"}]`)

	sessions, err := client(t, f, "good-token").Sessions(ctx(t))

	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].UserName != "samy" {
		t.Fatalf("sessions = %+v", sessions)
	}
	req := f.seen()[0]
	for _, want := range []string{`MediaBrowser `, `Token="good-token"`, `Client="PlayPresence"`, `Device="My PC é"`, `DeviceId="dev-123"`, `Version="0.1.0"`} {
		if !strings.Contains(req.Auth, want) {
			t.Errorf("Authorization %q lacks %q", req.Auth, want)
		}
	}
	if strings.Contains(req.RawQuery, "api_key") || strings.Contains(req.RawQuery, "good-token") {
		t.Errorf("token leaked into the URL: %q", req.RawQuery)
	}
}

func TestSessionsUnauthorized(t *testing.T) {
	f := newFakeServer(t, "")

	_, err := client(t, f, "wrong").Sessions(ctx(t))

	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestBaseURLWithPathPrefixAndTrailingSlash(t *testing.T) {
	f := newFakeServer(t, "/jellyfin")
	c, err := NewClient(f.URL()+"/", testDevice, "good-token", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.Sessions(ctx(t)); err != nil {
		t.Fatalf("Sessions: %v", err)
	}

	if got := f.seen()[0].Path; got != "/jellyfin/Sessions" {
		t.Errorf("path = %q, want /jellyfin/Sessions", got)
	}
}

func TestErrorsNeverContainTheToken(t *testing.T) {
	const token = "SUPER-SECRET-TOKEN"
	f := newFakeServer(t, "")
	f.validToken = token
	f.sessionsCode = http.StatusInternalServerError
	c := client(t, f, token)

	_, serverErr := c.Sessions(ctx(t))

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := "http://" + ln.Addr().String()
	_ = ln.Close()
	c2, _ := NewClient(dead, testDevice, token, nil)
	_, netErr := c2.Sessions(ctx(t))

	for name, err := range map[string]error{"server error": serverErr, "network error": netErr} {
		if err == nil {
			t.Fatalf("%s: expected an error", name)
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("%s leaks the token: %v", name, err)
		}
	}
}

func TestResponseOverCapIsRejected(t *testing.T) {
	f := newFakeServer(t, "")
	f.bigBody = true

	_, err := client(t, f, "good-token").Sessions(ctx(t))

	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v, want a response-too-large error", err)
	}
}

func TestNewClientRejectsBadURLs(t *testing.T) {
	for _, u := range []string{"", "ftp://host", "host:8096", "SECRET-no-scheme", "http://"} {
		_, err := NewClient(u, testDevice, "t", nil)
		if err == nil {
			t.Errorf("NewClient(%q) accepted a bad URL", u)
			continue
		}
		if u != "" && strings.Contains(err.Error(), u) {
			t.Errorf("error %q echoes the URL", err)
		}
	}
}

func TestSeriesIDsParsesProviderIds(t *testing.T) {
	f := newFakeServer(t, "")
	f.series["series-1"] = map[string]string{"Tmdb": "95396", "Imdb": "tt11280740"}

	ids, err := client(t, f, "good-token").SeriesIDs(ctx(t), "user-1", "series-1")

	if err != nil {
		t.Fatalf("SeriesIDs: %v", err)
	}
	if ids["Tmdb"] != "95396" || ids["Imdb"] != "tt11280740" {
		t.Errorf("ids = %v", ids)
	}
	q := f.seen()[0].RawQuery
	for _, want := range []string{"ids=series-1", "userId=user-1", "fields=ProviderIds"} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q lacks %q", q, want)
		}
	}
}

func TestSeriesIDsUnknownSeriesIsEmptyNotAnError(t *testing.T) {
	f := newFakeServer(t, "")

	ids, err := client(t, f, "good-token").SeriesIDs(ctx(t), "", "nope")

	if err != nil || len(ids) != 0 {
		t.Fatalf("ids = %v, err = %v, want empty and nil", ids, err)
	}
}
