# Installation

## Requirements

- The Discord **desktop** app, running on the same machine as PlayPresence. A browser or phone session is not enough: Rich Presence goes through the local Discord client.
- **Settings > Activity Privacy > Share my activity** enabled in Discord.
- Nothing else: PlayPresence ships with its own Discord applications. Creating your own (below) is optional.

## Use your own Discord application (optional)

Discord shows a presence on behalf of an application, and its name appears after "Watching". PlayPresence uses applications named after your media server by default (for example "Jellyfin"), so you can skip this section. Create your own if you want a different name or your own fallback images; it costs nothing.

1. Open the [Discord Developer Portal](https://discord.com/developers/applications) and create a **New Application**. The name you choose is what appears in your status ("Watching *name*"), so pick something you like.
2. Copy the **Application ID** (a long number). This is your `discord.app_id`; put it in the configuration to use it instead of the built-in application.
3. Optional but recommended: in **Rich Presence > Art Assets**, upload three square images (1024x1024 recommended) named exactly `movie`, `tv` and `music`. They are shown when no artwork can be found for an item. Ready-made ones are in [`assets/discord`](../assets/discord).

## Install PlayPresence

### Pre-built binary

Download the archive for your system from the [releases page](../../releases), extract it, and put `playpresence` (or `playpresence.exe`) anywhere you like. There is no installer and nothing else to install. Windows binaries are currently unsigned, so SmartScreen may ask for confirmation; see [releasing](releasing.md).

### From source

You need the Go version listed in `go.mod`.

```bash
go install github.com/Dystilest/PlayPresence/cmd/playpresence@latest
```

### Docker (experimental)

The image is published with each release. Discord runs on the host, so the container must see Discord's IPC socket; this works on Linux only, by mounting the socket:

```bash
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$XDG_RUNTIME_DIR/discord-ipc-0:/tmp/discord-ipc-0" \
  -v "$PWD/config.toml:/config/config.toml:ro" \
  ghcr.io/dystilest/playpresence run --config /config/config.toml
```

This path has not been exercised on a wide range of setups yet; please report what you find.

## First run

Create a configuration file (see the [reference](configuration.md) for its location and every key):

```toml
[sources.home]
kind = "jellyfin"
url = "http://192.168.1.10:8096"
```

Then:

```bash
playpresence doctor     # checks configuration, Discord and sources
playpresence run --demo # shows a sample episode on your profile (no server needed)
```

Stop with Ctrl+C: the presence is cleared on exit.

Media server setup guides are added as each integration ships.

## Connect a media server

Add a `[sources.NAME]` section to the configuration, sign in, then check:

```bash
playpresence auth jellyfin --source NAME
playpresence doctor
playpresence run
```

Each server has its own guide: [Jellyfin](jellyfin.md).
