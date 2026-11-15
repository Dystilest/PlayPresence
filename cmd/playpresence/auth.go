package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dystilest/PlayPresence/internal/config"
	"github.com/Dystilest/PlayPresence/internal/secrets"
	"github.com/Dystilest/PlayPresence/internal/source/jellyfin"
	"github.com/Dystilest/PlayPresence/internal/version"
)

// secretService is the credential-store namespace of every PlayPresence secret.
const secretService = "playpresence"

const (
	defaultQuickConnectPoll = 2 * time.Second
	quickConnectTimeout     = 5 * time.Minute
)

func tokenKey(source string) string  { return "source." + source + ".token" }
func userIDKey(source string) string { return "source." + source + ".user_id" }

// openSecrets returns the credential store: the injected one in tests, else
// the OS keyring, falling back to an encrypted file next to the config that is
// unlocked with PLAYPRESENCE_PASSPHRASE.
func openSecrets(e env) (secrets.Store, error) {
	if e.secrets != nil {
		return e.secrets, nil
	}
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	pass := func() (string, error) {
		if p := os.Getenv("PLAYPRESENCE_PASSPHRASE"); p != "" {
			return p, nil
		}
		return "", errors.New("no OS credential store is available: set PLAYPRESENCE_PASSPHRASE to unlock the encrypted secrets file")
	}
	return secrets.Default(filepath.Join(filepath.Dir(cfgPath), "secrets.enc"), pass), nil
}

// deviceID returns this installation's stable id, creating it on first use.
func deviceID(st secrets.Store) (string, error) {
	id, err := st.Get(secretService, "device_id")
	if err == nil && id != "" {
		return id, nil
	}
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return "", err
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id = hex.EncodeToString(buf)
	return id, st.Set(secretService, "device_id", id)
}

func jellyfinDevice(st secrets.Store) (jellyfin.Device, error) {
	id, err := deviceID(st)
	if err != nil {
		return jellyfin.Device{}, err
	}
	name, _ := os.Hostname()
	if name == "" {
		name = "PlayPresence"
	}
	return jellyfin.Device{Name: name, ID: id, Version: version.Version}, nil
}

func cmdAuth(ctx context.Context, args []string, e env) int {
	if len(args) == 0 || args[0] != "jellyfin" {
		_, _ = fmt.Fprintln(e.stderr, "usage: playpresence auth jellyfin --source NAME [--config PATH] [--token-stdin]")
		return 2
	}
	fs := flag.NewFlagSet("auth jellyfin", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	cfgPath := fs.String("config", "", "path to the configuration file")
	name := fs.String("source", "", "name of the [sources.NAME] entry to sign in to")
	tokenStdin := fs.Bool("token-stdin", false, "read an access token or API key from standard input instead of using Quick Connect")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *name == "" {
		_, _ = fmt.Fprintln(e.stderr, "playpresence: --source is required")
		return 2
	}

	cfg, _, err := loadConfig(*cfgPath, false)
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}
	sc, ok := cfg.Sources[*name]
	if !ok {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: source %q is not in the configuration: add a [sources.%s] section\n", *name, *name)
		return 1
	}
	if sc.Kind != "jellyfin" {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: source %q is not a jellyfin source\n", *name)
		return 1
	}
	st, err := openSecrets(e)
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}
	dev, err := jellyfinDevice(st)
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}

	var login jellyfin.Login
	if *tokenStdin {
		login, err = loginWithToken(ctx, e, sc.URL, dev)
	} else {
		login, err = loginWithQuickConnect(ctx, e, sc.URL, dev)
	}
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}

	if err := st.Set(secretService, tokenKey(*name), login.Token); err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: store token: %v\n", err)
		return 1
	}
	if login.UserID != "" {
		err = st.Set(secretService, userIDKey(*name), login.UserID)
	} else {
		err = st.Delete(secretService, userIDKey(*name))
	}
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: store user id: %v\n", err)
		return 1
	}
	if login.UserName != "" {
		_, _ = fmt.Fprintf(e.stdout, "Signed in as %s. The token is stored in the credential store, not in the configuration.\n", login.UserName)
	} else {
		_, _ = fmt.Fprintln(e.stdout, "Token verified and stored in the credential store, not in the configuration.")
	}
	return 0
}

func loginWithQuickConnect(ctx context.Context, e env, url string, dev jellyfin.Device) (jellyfin.Login, error) {
	poll := e.qcPoll
	if poll <= 0 {
		poll = defaultQuickConnectPoll
	}
	ctx, cancel := context.WithTimeout(ctx, quickConnectTimeout)
	defer cancel()
	show := func(code string) {
		_, _ = fmt.Fprintf(e.stdout, "Quick Connect code: %s\nIn a Jellyfin app where you are signed in, open your profile menu, choose Quick Connect and enter this code. Waiting...\n", code)
	}
	login, err := jellyfin.QuickConnect(ctx, url, dev, nil, show, poll)
	switch {
	case errors.Is(err, jellyfin.ErrQuickConnectDisabled):
		return login, errors.New("this server has Quick Connect disabled: enable it in the Jellyfin dashboard (General), or create an API key there and pipe it to `playpresence auth jellyfin --source NAME --token-stdin`")
	case errors.Is(err, context.DeadlineExceeded):
		return login, errors.New("quick connect timed out before the code was approved")
	}
	return login, err
}

func loginWithToken(ctx context.Context, e env, url string, dev jellyfin.Device) (jellyfin.Login, error) {
	line, err := bufio.NewReader(e.stdin).ReadString('\n')
	if err != nil && line == "" {
		return jellyfin.Login{}, errors.New("--token-stdin: no token on standard input")
	}
	token := strings.TrimSpace(line)
	if token == "" {
		return jellyfin.Login{}, errors.New("--token-stdin: the token on standard input is empty")
	}
	c, err := jellyfin.NewClient(url, dev, token, nil)
	if err != nil {
		return jellyfin.Login{}, err
	}
	vctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := c.Sessions(vctx); err != nil {
		if errors.Is(err, jellyfin.ErrUnauthorized) {
			return jellyfin.Login{}, errors.New("the server rejected this token; nothing was stored")
		}
		return jellyfin.Login{}, fmt.Errorf("could not verify the token: %w", err)
	}
	return jellyfin.Login{Token: token}, nil
}
