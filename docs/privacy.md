# Privacy

PlayPresence runs on your machine. This page lists everything it can send, and to whom.

## What PlayPresence never does

- No telemetry, analytics, crash reporting or update checks.
- No account, no cloud service operated by this project.
- It does not modify your media server: it only reads what is playing.

## What leaves your machine

| Destination | What is sent | When |
|---|---|---|
| Your media server (Jellyfin, Plex, Emby) | Authenticated read requests for the active sessions | Continuously while running |
| Your local Discord client | The activity: title, episode, artist, progress, button links, image URL | When playback changes |
| Public artwork services (keyless) | Title, year and public identifiers (such as a TMDb or MusicBrainz id) of what you play | Only when an item has no artwork yet; results are cached on disk |

The artwork services are those listed in `art.resolvers` in your [configuration](configuration.md). Remove any you do not want to use; with none enabled, the generic image is shown.

Optional features, **off by default**, change this table:

- An API key for a lookup service (stored in your credential store, never in the configuration file) lets that service see your lookups under your key.
- A custom image upload target sends the artwork file to the host you choose.

## What Discord does with it

Rich Presence is public on your Discord profile. Anyone who can see your profile can see the title you are watching, the buttons and the image. Discord's own privacy setting controls who can see your activity. If you do not want a title shown, pause PlayPresence.

## Secrets

Server tokens and optional keys are stored in the operating system's credential store (Windows Credential Manager, macOS Keychain, Secret Service on Linux). When none is available (for example in a container), an encrypted file protected by a passphrase you provide is used. Secrets are never written to the configuration file or to logs, and a configuration file that contains a token or key field is refused.

## Your media server address

The address of your media server is not sent to Discord or to artwork services. Posters are fetched from public services, not from your server, unless you explicitly declare a public image URL.
