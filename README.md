# PlayPresence

Show what you are watching or listening to on Discord, straight from your own media server. One small program, no account, no API key required.

> **Status: early development (pre-1.0).** Discord presence, rendering, configuration, secrets, diagnostics and the Jellyfin integration are built and tested. Other media servers are added one at a time; see the table below.

## Supported servers

| Server | Status |
|---|---|
| [Jellyfin](docs/jellyfin.md) | Supported (pre-1.0) |
| Plex | Planned |
| Emby | Planned |
| Kodi and others | Planned after 1.0 |

You can already try the whole pipeline with the built-in demo, which shows a sample episode on your profile.

## Quick start

You need the Discord desktop app running on the same machine, with **Settings > Activity Privacy > Share my activity** enabled. No Discord application or API key is required (you can use your own, see [Installation](docs/installation.md#use-your-own-discord-application-optional)).

```bash
go install github.com/Dystilest/PlayPresence/cmd/playpresence@latest
playpresence doctor --config path/to/config.toml
playpresence run --demo
```

Pre-built binaries for Windows, Linux and macOS are attached to each [release](../../releases) (currently a pre-release: expect rough edges and please report them).

`playpresence doctor` checks your configuration, that Discord is reachable, and the sources, and tells you what to fix.

## How it works

PlayPresence connects to your media server (read-only), turns the current playback into a Discord activity (title, episode, progress bar, pause state, links) and sends it to the local Discord client. When playback stops, the presence disappears. If Discord restarts, the presence comes back on its own.

```
media server --> source --> session --> render --> publisher --> Discord (local IPC)
```

Details: [Architecture](docs/architecture.md).

## Privacy and security

- Server credentials are kept in your operating system's credential store, never in the configuration file or in logs.
- No telemetry, no analytics, no accounts.
- Artwork is looked up through public services by title and identifiers only, and nothing is uploaded anywhere unless you explicitly enable it.
- Rich Presence is public on your Discord profile; see [Privacy](docs/privacy.md) for exactly what leaves your machine.

Found a vulnerability? Please read [SECURITY.md](SECURITY.md).

## Documentation

- [Installation](docs/installation.md)
- [Configuration reference](docs/configuration.md)
- [Privacy](docs/privacy.md)
- [FAQ and troubleshooting](docs/faq.md)
- [Default presentation](docs/templates.md)
- [Architecture](docs/architecture.md)
- [Adding a new media server](docs/adding-a-source.md)

## Contributing

Contributions are welcome: bug reports, documentation, new media server integrations. Start with [CONTRIBUTING.md](CONTRIBUTING.md). Please follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE)
