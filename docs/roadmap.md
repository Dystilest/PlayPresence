# Roadmap

What's done, what's next, and where help is most welcome. Plans change with feedback: if something matters to you, say so in [Discussions](https://github.com/Dystilest/PlayPresence/discussions) or open an issue.

## Done

- Discord status over the local Discord app: title, episode, poster, progress bar, pause, TMDb and IMDb buttons; comes back by itself after a Discord restart.
- Jellyfin: live updates, Quick Connect or API key sign-in, only your own playback by default, posters.
- Guided `setup`, double-click start on Windows, `doctor` to diagnose problems.
- Sign-ins kept in the system's credential store; no telemetry.
- Releases for Windows, macOS and Linux (amd64 and arm64), with checksums and build attestations; experimental Docker image.

## Next (towards 1.0)

| Item | Notes | Help wanted |
|---|---|---|
| **Plex** | Sign-in with a plex.tv PIN (no password), live session updates, Plex's own posters. | Testers with a Plex server; code welcome |
| **Emby** | Close to Jellyfin's API; API-key or Emby Connect sign-in. | Testers with an Emby server |
| **More poster sources** | TVmaze (keyless) and TMDb, Fanart.tv (with your own key) for items the server can't match. Already accepted in `art.resolvers`, not active yet. | Yes |
| **Start with the computer on macOS** | A login item, so nothing has to be typed. | Mac users |
| **Signed Windows and macOS binaries** | Removes the "Windows protected your PC" and Gatekeeper warnings. Needs certificates. | Ideas and sponsors |
| **Real-world testing** | Episodes and season posters are tested against simulated servers only so far. | Anyone with Jellyfin: tell us what you see |

## Later

- Customisable text (choose what each line shows).
- A tray icon instead of a console window.
- Kodi and other players.
- Translations of the messages and the documentation.

## How to help

- Try it and [report what doesn't work](https://github.com/Dystilest/PlayPresence/issues/new/choose); that's the most useful thing right now.
- Pick an issue labelled [good first issue](https://github.com/Dystilest/PlayPresence/labels/good%20first%20issue) or [help wanted](https://github.com/Dystilest/PlayPresence/labels/help%20wanted).
- Adding a media server? Read [Adding a media server](adding-a-source.md) and open an issue first so we can agree on the approach.
