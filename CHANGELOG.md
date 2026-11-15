# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
