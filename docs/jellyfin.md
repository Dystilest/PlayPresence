# Jellyfin

PlayPresence reads the active playback sessions of a Jellyfin server and shows them on Discord. It only reads: it never changes anything on your server.

## Requirements

- A Jellyfin server you can reach from the machine that runs PlayPresence (same network, VPN or public address).
- Quick Connect enabled on the server (Jellyfin 10.7 or newer), or an API key. Quick Connect is the easiest way: no password is typed into PlayPresence.
- The Discord setup described in [Installation](installation.md).

## Set up

### The easy way

Run the setup and answer its questions (on Windows, double-clicking `playpresence.exe` does this the first time):

```bash
playpresence setup
```

1. Type your server's address, the one you open in your browser (for example `http://192.168.1.10:8096`). You can paste it straight from the browser's address bar.
2. PlayPresence shows a short code. In a Jellyfin app where you're signed in, open your profile menu, choose **Quick Connect** and enter the code.
3. It checks that Discord is open, and you're done: run `playpresence run`.

It never asks for your password. If Quick Connect is turned off on your server, it explains how to turn it on, or lets you paste an API key instead (Jellyfin dashboard > **API Keys**).

Run `playpresence setup` again at any time to sign in again or to add another server.

### By hand

1. Add the server to your [configuration](configuration.md). The name after `sources.` identifies it everywhere (`doctor`, `auth`, messages). Spaces and accents are fine when quoted.

   ```toml
   [sources.home]
   kind = "jellyfin"
   url = "http://192.168.1.10:8096"
   ```

   A path prefix such as `https://example.org/jellyfin` and a trailing slash are fine. Do not put a password, token or API key in the URL: such a configuration is refused.

2. Sign in once with Quick Connect:

   ```bash
   playpresence auth jellyfin --source home
   ```

   or with an API key, piped in so that it doesn't end up in your shell history (the key is checked before it is stored):

   ```bash
   playpresence auth jellyfin --source home --token-stdin
   ```

3. Check everything, then run:

   ```bash
   playpresence doctor
   playpresence run
   ```

   `doctor` should print a line like `OK   source:home  Jellyfin 10.10.0 reachable, token valid for <your user>`.

## Which sessions are shown

- Only **movies** and **episodes** (TV series) are shown. Everything else (music, books, live TV channels, music videos) is ignored.
- If several devices are playing at once, the one already on your profile stays there; another device doesn't take over until it stops. When nothing is shown yet, the most recently active device wins.
- By default only the playback of the account you signed in with is shown, even when the token (an administrator's, say) can see other people's sessions. Set `user` to show another Jellyfin user name (case-insensitive) instead, or `user = "*"` to show every session the token can see (the most recently active one wins). An API key has no account behind it, so with an API key every visible session is considered unless `user` is set:

  ```toml
  [sources.home]
  kind = "jellyfin"
  url = "http://192.168.1.10:8096"
  user = "samy"
  ```

## What is displayed

| Item | Shown |
|---|---|
| Movie | Title and year, poster, progress bar, links to TMDb and IMDb when the server knows the ids |
| Episode | Series name, season and episode, episode title, season poster (series poster as a fallback), progress bar; the links point to the **series** |

Paused playback shows the paused state without a running clock. If the server does not report a duration, the progress bar is left out.

## How it talks to the server

PlayPresence opens a WebSocket on `/socket` and asks the server to push the session list; it re-checks on every push. If the WebSocket cannot be established (for example a reverse proxy that does not forward upgrades), it polls `GET /Sessions` every 5 seconds instead, automatically. Series ids for the links are fetched once per series with `GET /Items`, and poster addresses once per item with `GET /Items/{id}/RemoteImages`. Requests are authenticated with the `Authorization` header, never with a token in the URL. See [Privacy](privacy.md) for the full list.

## Troubleshooting

**`token rejected`.** The token was revoked or the server was reset. Run `playpresence setup` (or `playpresence auth jellyfin --source NAME`) again.

**`this server has Quick Connect disabled`.** Enable it in the Jellyfin dashboard (General), or use `--token-stdin` with an API key.

**`unreachable`.** `doctor` could not connect. Check the URL (scheme and port), that the server is running, and that a firewall is not in the way. The address itself is deliberately not echoed in error messages.

**Nothing shows while something plays.** Check that `user` is not filtering you out, that the item is a movie or an episode, and run `playpresence doctor`. Then see the [FAQ](faq.md).

**Behind a reverse proxy.** Forward WebSocket upgrades on `/socket` for the fastest updates; without them polling is used and changes show up within a few seconds.

**Headless machine or container (no credential store).** The token is kept in an encrypted file (`secrets.enc`) in the PlayPresence configuration folder, unlocked with the passphrase in the `PLAYPRESENCE_PASSPHRASE` environment variable. Set it for `setup`, `auth` and `run` alike.

## Self-signed certificates

There is deliberately no option to disable TLS verification: a presence tool that trusts any certificate would also hand your token to anyone in the middle. Use a certificate your system trusts (a free one from a public authority, or your own authority added to the system trust store), or an `http://` address on a trusted local network.
