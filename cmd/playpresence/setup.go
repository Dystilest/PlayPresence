package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/Dystilest/PlayPresence/internal/config"
	"github.com/Dystilest/PlayPresence/internal/presence"
	"github.com/Dystilest/PlayPresence/internal/source/jellyfin"
)

const (
	setupAttempts     = 3
	setupProbeTimeout = 10 * time.Second
	configHeader      = "# PlayPresence configuration.\n" +
		"# Every setting: https://github.com/Dystilest/PlayPresence/blob/main/docs/configuration.md\n" +
		"# Your sign-in is not stored here: it is kept in your system's credential store.\n"
)

// cmdSetup walks a first-time user through connecting a media server.
func cmdSetup(ctx context.Context, args []string, e env) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	cfgPath := fs.String("config", "", "path to the configuration file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if _, ok := setup(ctx, *cfgPath, e); !ok {
		return 1
	}
	_, _ = fmt.Fprintln(e.stdout, "\nAll set! Start PlayPresence with `playpresence run`, then play a movie or an episode.")
	return 0
}

// cmdStart sets PlayPresence up when nothing is configured yet, then runs it.
// It is what a double-click on the program does.
func cmdStart(ctx context.Context, args []string, e env) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	cfgPath := fs.String("config", "", "path to the configuration file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	cfg, path, err := loadConfig(*cfgPath, true)
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
		return 1
	}
	if len(enabledSources(cfg)) == 0 {
		if path, err = setupPath(path); err != nil {
			_, _ = fmt.Fprintf(e.stderr, "playpresence: %v\n", err)
			return 1
		}
		var ok bool
		if path, ok = setup(ctx, path, e); !ok {
			return 1
		}
		_, _ = fmt.Fprintln(e.stdout)
	}
	return cmdRun(ctx, []string{"--config", path}, e)
}

func setupPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return config.DefaultPath()
}

// setup asks for the server address, writes the configuration, signs in and
// checks Discord. It never asks for a password. It returns the configuration
// path and whether everything worked.
func setup(ctx context.Context, cfgPath string, e env) (string, bool) {
	in := bufio.NewReader(e.stdin)
	terminal := e.stdin
	e.stdin = in // every later read shares the same buffer
	say := func(format string, a ...any) { _, _ = fmt.Fprintf(e.stdout, format+"\n", a...) }
	fail := func(format string, a ...any) (string, bool) {
		_, _ = fmt.Fprintf(e.stderr, "playpresence: "+format+"\n", a...)
		return cfgPath, false
	}

	path, err := setupPath(cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	cfg, err := config.Load(path)
	exists := err == nil
	switch {
	case errors.Is(err, os.ErrNotExist):
		cfg = config.Default()
	case err != nil:
		return fail("your configuration file has a problem: %v\nFix it, or delete it to start over, then run setup again.", err)
	}

	say("Welcome to PlayPresence! Let's show what you watch on Jellyfin on your Discord profile.")
	say("This takes about a minute. PlayPresence never asks for your password.\n")

	st, err := openSecrets(e)
	if err != nil {
		return fail("%v", err)
	}
	dev, err := jellyfinDevice(st)
	if err != nil {
		return fail("%v", err)
	}

	// Step 1: the server address.
	say("Step 1 of 3: your Jellyfin server")
	var serverURL string
	for attempt := 1; ; attempt++ {
		line, err := ask(in, e.stdout, "Address of your Jellyfin server, as you open it in your browser\n(for example http://192.168.1.10:8096): ")
		if err != nil {
			return fail("no answer received, setup stopped")
		}
		u, info, advice := probeJellyfin(ctx, line, dev)
		if advice == "" {
			serverURL = u
			say("Found %q (Jellyfin %s).\n", info.ServerName, info.Version)
			break
		}
		say("%s", advice)
		if attempt == setupAttempts {
			return fail("setup stopped: run it again once the server opens in your browser on this computer")
		}
	}

	name, known := sourceFor(cfg, serverURL)
	if !known {
		if err := appendSource(path, exists, name, serverURL); err != nil {
			return fail("could not save the configuration: %v", err)
		}
		say("Saved to %s.\n", path)
	}

	// Step 2: sign in.
	say("Step 2 of 3: sign in to Jellyfin")
	login, err := loginWithQuickConnect(ctx, e, serverURL, dev)
	if errors.Is(err, jellyfin.ErrQuickConnectDisabled) {
		say("Quick Connect is turned off on this server. Either turn it on (Jellyfin dashboard > General > Quick Connect)")
		say("and run setup again, or create an API key (Jellyfin dashboard > API Keys) and paste it here.")
		var key string
		key, err = askSecret(in, terminal, e.stdout, "API key (or just press Enter to stop): ")
		switch {
		case err != nil || key == "":
			return fail("setup stopped before signing in; your server is saved, run setup again to sign in")
		default:
			login, err = verifyToken(ctx, serverURL, dev, key)
		}
	}
	if err != nil {
		return fail("%v", err)
	}
	if err := storeLogin(st, name, login); err != nil {
		return fail("%v", err)
	}
	if login.UserName != "" {
		say("Signed in as %s.\n", login.UserName)
	} else {
		say("API key accepted.\n")
	}

	// Step 3: Discord.
	say("Step 3 of 3: Discord")
	if final, err := config.Load(path); err == nil {
		cfg = final
	}
	dctx, cancel := context.WithTimeout(ctx, doctorDiscordTimeout)
	client := presence.NewIPCClient(cfg.ResolveAppID(), e.dial, e.candidates)
	err = client.Connect(dctx)
	_ = client.Close()
	cancel()
	switch {
	case err == nil:
		say("Discord is running. Make sure Settings > Activity Privacy > Share my activity is turned on.")
	case errors.Is(err, presence.ErrNotRunning):
		say("Discord is not open right now. Start the Discord desktop app (the browser version does not work)")
		say("and turn on Settings > Activity Privacy > Share my activity. PlayPresence connects to it on its own.")
	default:
		say("Discord answered but refused the connection: %v", err)
	}
	return path, true
}

// ask prints prompt and returns the trimmed answer.
func ask(in *bufio.Reader, out io.Writer, prompt string) (string, error) {
	_, _ = fmt.Fprint(out, prompt)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// askSecret is ask without echo when the input is a terminal.
func askSecret(in *bufio.Reader, terminal io.Reader, out io.Writer, prompt string) (string, error) {
	if f, ok := terminal.(*os.File); ok && term.IsTerminal(int(f.Fd())) { //nolint:gosec // a file descriptor fits in an int
		_, _ = fmt.Fprint(out, prompt)
		b, err := term.ReadPassword(int(f.Fd())) //nolint:gosec // a file descriptor fits in an int
		_, _ = fmt.Fprintln(out)
		return strings.TrimSpace(string(b)), err
	}
	return ask(in, out, prompt)
}

// normalizeServerURL accepts what people copy from their browser: a missing
// scheme, the web client's own path (/web/index.html#!/home.html), a query or
// a trailing slash.
func normalizeServerURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.RawQuery, u.Fragment, u.RawFragment = "", "", ""
	if i := strings.Index(u.Path, "/web"); i >= 0 && (len(u.Path) == i+4 || u.Path[i+4] == '/') {
		u.Path = u.Path[:i]
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String()
}

// probeJellyfin checks that raw is the address of a reachable Jellyfin server.
// On failure it returns advice for the user, which never echoes the address.
func probeJellyfin(ctx context.Context, raw string, dev jellyfin.Device) (string, jellyfin.PublicInfo, string) {
	u := normalizeServerURL(raw)
	if u == "" {
		return "", jellyfin.PublicInfo{}, "Please type the address of your server."
	}
	check := config.Default()
	check.Sources = map[string]config.SourceConfig{"server": {Kind: "jellyfin", URL: u}}
	if check.Validate() != nil {
		return "", jellyfin.PublicInfo{}, "That does not look like a server address. Type it as it appears in your browser, without any password or key in it."
	}
	c, err := jellyfin.NewClient(u, dev, "", nil)
	if err != nil {
		return "", jellyfin.PublicInfo{}, "That does not look like a server address. Type it as it appears in your browser."
	}
	pctx, cancel := context.WithTimeout(ctx, setupProbeTimeout)
	defer cancel()
	info, err := c.PublicInfo(pctx)
	if err != nil {
		return "", jellyfin.PublicInfo{}, fmt.Sprintf("Could not reach a Jellyfin server there (%v).\nCheck that the server is on and that this address opens Jellyfin in a browser on this computer.", err)
	}
	return u, info, ""
}

// sourceFor returns the configured Jellyfin source with this address, or a
// new unused name for it.
func sourceFor(cfg config.Config, serverURL string) (string, bool) {
	for name, s := range cfg.Sources {
		if s.Kind == "jellyfin" && normalizeServerURL(s.URL) == serverURL {
			return name, true
		}
	}
	name := "jellyfin"
	for i := 2; ; i++ {
		if _, taken := cfg.Sources[name]; !taken {
			return name, false
		}
		name = fmt.Sprintf("jellyfin%d", i)
	}
}

// appendSource adds a source section at the end of the configuration file,
// creating it when needed, so existing comments and settings are kept.
func appendSource(path string, exists bool, name, serverURL string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	if !exists {
		b.WriteString(configHeader)
	}
	_, _ = fmt.Fprintf(&b, "\n[sources.%s]\nkind = \"jellyfin\"\nurl = %q\n", name, serverURL)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the path is the user's own config location
	if err != nil {
		return err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if _, err := config.Load(path); err != nil {
		return fmt.Errorf("the updated file does not load: %w", err)
	}
	return nil
}
