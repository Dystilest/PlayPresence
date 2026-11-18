# Privacy

PlayPresence runs on your machine. This page lists everything it can send, and to whom.

## What PlayPresence never does

- No telemetry, analytics, crash reporting or update checks.
- No account, no cloud service operated by this project.
- It does not modify your media server: it only reads what is playing.

## What leaves your machine

| Destination | What is sent | When |
|---|---|---|
| Your media server | Authenticated read requests. Jellyfin: a WebSocket on `/socket` (or `GET /Sessions` when polling) for what is playing; `GET /Items` once per series for the TMDb and IMDb ids; `GET /Items/{id}/RemoteImages` once per item for a poster address; `GET /System/Info/Public` and `GET /Users/Me` from `setup` and `doctor`; the Quick Connect endpoints while signing in. | Continuously while running |
| The Discord app on your computer | The status: title, year, season and episode, progress, button links, poster address | When playback changes, and every minute to survive a Discord restart |

That's the complete list. PlayPresence itself contacts no other service.

**Posters.** Your Jellyfin server knows where its posters come from (usually TMDb). PlayPresence asks your server for that public address (for example an `image.tmdb.org` link) and passes it to Discord, which then downloads the image through its own image proxy. The address of your server is never sent to Discord, and you can turn posters off by setting `art.resolvers = []` in the [configuration](configuration.md).

Planned artwork sources (see the [roadmap](roadmap.md)) will be listed here, with what they receive, before they ship. They will stay opt-in.

## What Discord does with it

Rich Presence is public on your Discord profile. Anyone who can see your profile can see the title you are watching, the buttons and the image. Discord's own privacy setting controls who can see your activity. If you do not want a title shown, stop PlayPresence (Ctrl+C clears the presence) or change your Discord activity privacy setting.

## Secrets

Server tokens are stored in the operating system's credential store (Windows Credential Manager, macOS Keychain, Secret Service on Linux). When none is available (for example in a container), an encrypted file protected by a passphrase you provide is used. Secrets are never written to the configuration file or to logs, and a configuration file that contains a token or key field is refused.

## Your media server address

The address of your media server is never sent to Discord, and it is left out of error messages so that a pasted log doesn't reveal it.
