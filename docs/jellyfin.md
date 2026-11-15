# Jellyfin

PlayPresence reads the active playback sessions of a Jellyfin server and shows them on Discord. It only reads: it never changes anything on your server.

## Requirements

- A Jellyfin server you can reach from the machine that runs PlayPresence (same network, VPN or public address).
- Quick Connect enabled on the server (Jellyfin 10.7 or newer), or an API key. Quick Connect is the easiest way: no password is typed into PlayPresence.
- The Discord setup described in [Installation](installation.md).

## Set up

1. Add the server to your [configuration](configuration.md). The name you choose after `sources.` identifies it everywhere (`doctor`, `auth`, logs). Spaces and accents are fine when quoted.

   ```toml
   [discord]
   app_id = "YOUR_APPLICATION_ID"

   [sources.home]
   kind = "jellyfin"
   url = "http://192.168.1.10:8096"
   ```

   The URL is the address you type in a browser. A path prefix such as `https://example.org/jellyfin` and a trailing slash are fine. Do not put a password, token or API key in the URL: such a configuration is refused.

2. Sign in once:

   ```bash
   playpresence auth jellyfin --source home
   ```

   PlayPresence prints a short code. In a Jellyfin app where you are signed in, open your profile menu, choose **Quick Connect** and enter the code. PlayPresence then stores the access token in your operating system's credential store (never in the configuration file).

   Quick Connect is switched on in the Jellyfin dashboard (**General**). If you prefer an API key (dashboard, **API Keys**), pipe it in instead of typing it on the command line, where it would end up in your shell history:

   ```bash
   playpresence auth jellyfin --source home --token-stdin
   ```

   The key is checked against the server before it is stored; a rejected key is not saved.

3. Check everything:

   ```bash
   playpresence doctor
   ```

   You should see a line like `OK   source:home  Jellyfin 10.10.0 reachable, token valid for <your user>`.

4. Run:

   ```bash
   playpresence run
   ```

## Which sessions are shown

- Only **movies**, **episodes** and **music tracks** are shown. Other item types (books, live TV channels, music videos) are ignored.
- If several devices are playing at once, the one with the most recent activity is shown. When it stops while another keeps playing, the other one takes over.
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
| Movie | Title and year, progress bar, links to TMDb and IMDb when the server knows the ids |
| Episode | Series name, season and episode, episode title, progress bar; the links point to the **series** |
| Music track | Title, artist and album, progress bar |

Paused playback shows the paused state without a running clock. If the server does not report a duration, the progress bar is left out.

## How it talks to the server

PlayPresence opens a WebSocket on `/socket` and asks the server to push the session list; it re-checks on every push. If the WebSocket cannot be established (for example a reverse proxy that does not forward upgrades), it polls `GET /Sessions` every 5 seconds instead, automatically. Series ids for the links are fetched once per series with `GET /Items`. Requests are authenticated with the `Authorization` header, never with a token in the URL. See [Privacy](privacy.md) for the full list.

## Troubleshooting

**`token rejected`.** The token was revoked or the server was reset. Run `playpresence auth jellyfin --source NAME` again.

**`this server has Quick Connect disabled`.** Enable it in the Jellyfin dashboard (General), or use `--token-stdin` with an API key.

**`unreachable`.** `doctor` could not connect. Check the URL (scheme and port), that the server is running, and that a firewall is not in the way. The address itself is deliberately not echoed in error messages.

**Nothing shows while something plays.** Check that `user` is not filtering you out, that the item is a movie, episode or track, and run `playpresence doctor`. Then see the [FAQ](faq.md).

**Behind a reverse proxy.** Forward WebSocket upgrades on `/socket` for the fastest updates; without them polling is used and changes show up within a few seconds.

**Headless machine or container (no credential store).** The token is kept in an encrypted file next to the configuration, unlocked with the passphrase in the `PLAYPRESENCE_PASSPHRASE` environment variable. Set it for both `auth` and `run`.

## Self-signed certificates

There is deliberately no option to disable TLS verification: a presence tool that trusts any certificate would also hand your token to anyone in the middle. Use a certificate your system trusts (a free one from a public authority, or your own authority added to the system trust store), or an `http://` address on a trusted local network.
