package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// ErrQuickConnectDisabled means the server has Quick Connect turned off.
var ErrQuickConnectDisabled = errors.New("jellyfin: quick connect is disabled on this server")

// Login is the result of a successful authentication.
type Login struct {
	Token    string
	UserID   string
	UserName string
}

// QuickConnect logs in through Jellyfin's Quick Connect: it asks the server
// for a code, hands it to show so the user can approve it from a signed-in
// Jellyfin client, then waits (polling every poll) and exchanges the approval
// for an access token. The secret never leaves this function.
func QuickConnect(ctx context.Context, baseURL string, dev Device, hc *http.Client, show func(code string), poll time.Duration) (Login, error) {
	c, err := NewClient(baseURL, dev, "", hc)
	if err != nil {
		return Login{}, err
	}
	var enabled bool
	if err := c.do(ctx, http.MethodGet, "QuickConnect/Enabled", nil, nil, &enabled, false); err != nil {
		return Login{}, err
	}
	if !enabled {
		return Login{}, ErrQuickConnectDisabled
	}

	var init struct {
		Secret string `json:"Secret"`
		Code   string `json:"Code"`
	}
	err = c.do(ctx, http.MethodPost, "QuickConnect/Initiate", nil, nil, &init, false)
	var se statusError
	if errors.As(err, &se) && se == http.StatusMethodNotAllowed {
		// Servers before 10.9 expose Initiate as GET.
		err = c.do(ctx, http.MethodGet, "QuickConnect/Initiate", nil, nil, &init, false)
	}
	if err != nil {
		return Login{}, err
	}
	if init.Secret == "" || init.Code == "" {
		return Login{}, errors.New("jellyfin: quick connect: unexpected answer from the server")
	}
	show(init.Code)

	for {
		var st struct {
			Authenticated bool `json:"Authenticated"`
		}
		q := url.Values{"secret": {init.Secret}}
		if err := c.do(ctx, http.MethodGet, "QuickConnect/Connect", q, nil, &st, false); err != nil {
			return Login{}, err
		}
		if st.Authenticated {
			break
		}
		select {
		case <-ctx.Done():
			return Login{}, ctx.Err()
		case <-time.After(poll):
		}
	}

	var res struct {
		AccessToken string `json:"AccessToken"`
		User        struct {
			ID   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"User"`
	}
	body := map[string]string{"Secret": init.Secret}
	if err := c.do(ctx, http.MethodPost, "Users/AuthenticateWithQuickConnect", nil, body, &res, false); err != nil {
		return Login{}, err
	}
	if res.AccessToken == "" {
		return Login{}, errors.New("jellyfin: quick connect: the server returned no access token")
	}
	return Login{Token: res.AccessToken, UserID: res.User.ID, UserName: res.User.Name}, nil
}
