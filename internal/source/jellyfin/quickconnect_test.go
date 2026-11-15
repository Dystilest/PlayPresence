package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestQuickConnectPendingThenAuthorized(t *testing.T) {
	f := newFakeServer(t, "")
	f.qc.pendingPolls = 2
	var shown []string

	login, err := QuickConnect(ctx(t), f.URL(), testDevice, nil, func(code string) { shown = append(shown, code) }, 5*time.Millisecond)

	if err != nil {
		t.Fatalf("QuickConnect: %v", err)
	}
	if login.Token != "issued-token" || login.UserID != "u1" || login.UserName != "Samy" {
		t.Errorf("login = %+v", login)
	}
	if len(shown) != 1 || shown[0] != qcCode {
		t.Errorf("show called with %v, want exactly [%s]", shown, qcCode)
	}
}

func TestQuickConnectDisabledDoesNotInitiate(t *testing.T) {
	f := newFakeServer(t, "")
	f.qc.enabled = false

	_, err := QuickConnect(ctx(t), f.URL(), testDevice, nil, func(string) {}, time.Millisecond)

	if !errors.Is(err, ErrQuickConnectDisabled) {
		t.Fatalf("err = %v, want ErrQuickConnectDisabled", err)
	}
	if n := f.count("/QuickConnect/Initiate"); n != 0 {
		t.Errorf("Initiate called %d times", n)
	}
}

func TestQuickConnectContextCancelledWhilePending(t *testing.T) {
	f := newFakeServer(t, "")
	f.qc.pendingPolls = 1 << 30
	c, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := QuickConnect(c, f.URL(), testDevice, nil, func(string) {}, 5*time.Millisecond)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the context error", err)
	}
}

func TestQuickConnectSecretNeverShownNorInErrors(t *testing.T) {
	f := newFakeServer(t, "")
	f.qc.authStatus = http.StatusUnauthorized
	var shown []string

	_, err := QuickConnect(ctx(t), f.URL(), testDevice, nil, func(code string) { shown = append(shown, code) }, time.Millisecond)

	if err == nil {
		t.Fatal("expected an error when authentication is refused")
	}
	if strings.Contains(err.Error(), qcSecret) {
		t.Errorf("error leaks the secret: %v", err)
	}
	for _, s := range shown {
		if strings.Contains(s, qcSecret) {
			t.Errorf("show received the secret: %q", s)
		}
	}
}

func TestQuickConnectFallsBackToGetOnOlderServers(t *testing.T) {
	f := newFakeServer(t, "")
	f.qc.initiateGetOnly = true

	login, err := QuickConnect(ctx(t), f.URL(), testDevice, nil, func(string) {}, time.Millisecond)

	if err != nil || login.Token != "issued-token" {
		t.Fatalf("login = %+v, err = %v", login, err)
	}
}
