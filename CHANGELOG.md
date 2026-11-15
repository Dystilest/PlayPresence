# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Jellyfin integration: live updates over WebSocket with automatic polling fallback, Quick Connect or API-key sign-in (`playpresence auth jellyfin`), per-source user filter, movies, episodes and music, `doctor` checks per source, source failures reported once per cause.
- Discord presence over the local IPC connection: Watching and Listening activities, progress bar, pause state, up to two buttons, status line from the details field.
- Resilient publishing: coalesced updates at least 15 s apart, automatic retry while Discord is unreachable, periodic re-send so the presence returns after a Discord restart, clean removal on exit.
- Server-neutral session model and a source interface for media server integrations, with source supervision, priority between sources and suppression of redundant updates.
- TOML configuration with strict validation and a generated reference.
- Credential storage in the OS credential store, with an encrypted-file fallback.
- Command line: `run` (with `--demo`), `doctor`, `version`.
- Documentation: installation, privacy, FAQ, architecture, guide to adding a media server, contribution guide, security policy.
