# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `playpresence setup`: a guided setup that asks for the server address (pasted straight from the browser is fine), signs in with Quick Connect or an API key, and checks Discord. It never asks for a password, and running it again signs in again or adds another server without duplicating it.
- `playpresence start`, and a double-click on `playpresence.exe` on Windows: set up when nothing is configured, then run; the window stays open on an error so it can be read.
- `run` says that it is running and prints what is shown on Discord whenever it changes.
- `doctor` warns about artwork sources listed in `art.resolvers` that are not available yet.
- Documentation for non-technical users: step-by-step installation per system, starting with the computer, a roadmap, more FAQ entries.

### Changed

- The project is scoped to movies and TV series. Music support was removed: a Jellyfin audio item is ignored, the Listening activity type, MusicBrainz links and the music-related artwork resolvers (iTunes, MusicBrainz, Open Library) are gone, and `music` is no longer a fallback image.
- `art.resolvers` defaults to `["server"]`, the only artwork source available today. The planned ones (`tvmaze`, `tmdb`, `fanart`, ...) are still accepted so existing files load, but `doctor` now says they do nothing yet. The documentation no longer describes artwork services that don't exist.
- Docker image: the configuration and the encrypted sign-in file live in the directory mounted on `/config`.

### Fixed

- Jellyfin: when a second device starts playing, the session already shown stays on screen instead of being replaced by the most recently active one.

## [0.1.0-beta.1] - 2026-11-15

First public pre-release: the core and the Jellyfin integration. Plex and Emby are not available yet.

### Added

- Built-in Discord applications: nothing to create, the one named after your server shows after "Watching" (`discord.app_id` stays available to use your own).
- Posters taken from the sources your Jellyfin server already uses (season poster for episodes, series poster as a fallback), sent to Discord as public image URLs; the server address is never sent.
- By default only the account you signed in with is shown; `user = "*"` shows every session the token can see.

- Jellyfin integration: live updates over WebSocket with automatic polling fallback, Quick Connect or API-key sign-in (`playpresence auth jellyfin`), per-source user filter, movies, episodes and music, `doctor` checks per source, source failures reported once per cause.
- Discord presence over the local IPC connection: Watching and Listening activities, progress bar, pause state, up to two buttons, status line from the details field.
- Resilient publishing: coalesced updates (a burst of three, then one per 15 s), automatic retry while Discord is unreachable, periodic re-send so the presence returns after a Discord restart, clean removal on exit.
- Server-neutral session model and a source interface for media server integrations, with source supervision, priority between sources and suppression of redundant updates.
- TOML configuration with strict validation and a generated reference.
- Credential storage in the OS credential store, with an encrypted-file fallback.
- Command line: `run` (with `--demo`), `doctor`, `version`.
- Documentation: installation, privacy, FAQ, architecture, guide to adding a media server, contribution guide, security policy.

### Known limitations

- Only Jellyfin is supported. Plex and Emby are planned.
- Verified against a real Jellyfin server for movies, pause and resume, posters and the built-in Discord application. Episodes (season posters) and music have been tested against simulated servers only.
- The Discord application is chosen once at startup: with several server types configured together, the generic "PlayPresence" application is used.
- Posters come from the image sources the Jellyfin server itself uses; without a match, a generic image is shown.
- The Windows binaries are not signed, so SmartScreen may ask for confirmation. The Docker image is experimental.
