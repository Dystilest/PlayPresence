# Installation

## What you need

- The Discord **desktop** app, running on the same computer as PlayPresence. The browser and phone versions are not enough: the status goes through the Discord app on your computer.
- In Discord, **User Settings > Activity Privacy > Share my activity** turned on.
- A media server PlayPresence supports (today: [Jellyfin](jellyfin.md)).

That's all. You don't need a Discord developer account or any API key.

## Windows

1. Download `playpresence_<version>_windows_amd64.zip` from the [releases page](https://github.com/Dystilest/PlayPresence/releases). (On a Windows on ARM laptop, take `windows_arm64`.)
2. Right-click the zip, choose **Extract All...**, and pick a folder you'll keep, for example `Documents\PlayPresence`.
3. Double-click `playpresence.exe`.
   - If Windows shows **"Windows protected your PC"**, click **More info**, then **Run anyway**. This appears because the program isn't signed with a paid certificate yet ([details](faq.md#windows-says-windows-protected-your-pc)).
4. A window opens and asks for your server's address, then shows a code to enter in Jellyfin. After that, PlayPresence keeps running in that window. Minimize it; close it to stop.

Next time, just double-click `playpresence.exe` again: it remembers your setup.

## macOS

1. Download the archive from the [releases page](https://github.com/Dystilest/PlayPresence/releases): `darwin_arm64` for Macs with an Apple chip (M1 or newer), `darwin_amd64` for older Intel Macs. Double-click it to extract it.
2. Open **Terminal** (Applications > Utilities), type `cd ` (with a space), drag the extracted folder into the Terminal window, and press Return.
3. The program isn't signed by Apple yet, so macOS blocks it the first time. Allow it once with:

   ```bash
   xattr -d com.apple.quarantine playpresence
   ```

4. Set it up, then run it:

   ```bash
   ./playpresence setup
   ./playpresence run
   ```

Keep the Terminal window open while you watch; press Ctrl+C to stop.

## Linux

1. Download the `linux_amd64` (or `linux_arm64`) archive from the [releases page](https://github.com/Dystilest/PlayPresence/releases) and extract it:

   ```bash
   tar -xzf playpresence_*_linux_amd64.tar.gz
   ```

2. Set it up, then run it:

   ```bash
   ./playpresence setup
   ./playpresence run
   ```

Flatpak, Snap and Vesktop builds of Discord are detected automatically. Optionally move `playpresence` to `~/.local/bin` so you can type `playpresence` from anywhere.

## Start automatically

**Windows:** press **Win+R**, type `shell:startup` and press Enter. In the folder that opens, right-click > **New > Shortcut** and point it to your `playpresence.exe`. PlayPresence then starts with Windows in a small window you can minimize.

**Linux (systemd):** create `~/.config/systemd/user/playpresence.service`:

```ini
[Unit]
Description=PlayPresence
After=graphical-session.target

[Service]
ExecStart=%h/.local/bin/playpresence run
Restart=on-failure

[Install]
WantedBy=default.target
```

then run `systemctl --user enable --now playpresence`.

**macOS:** automatic start is not packaged yet ([roadmap](roadmap.md)); run it from Terminal for now.

## The commands

| Command | What it does |
|---|---|
| `playpresence setup` | Connects a media server step by step. Run it again to add another server or to sign in again. |
| `playpresence run` | Shows what you watch on Discord until you stop it. |
| `playpresence start` | `setup` if nothing is configured yet, then `run`. This is what a double-click does on Windows. |
| `playpresence doctor` | Checks the configuration, Discord and your servers, and says what to fix. |
| `playpresence run --demo` | Shows a sample episode on your profile, without any server. |
| `playpresence auth jellyfin --source NAME` | Signs in to a configured server again (for example after a password reset). |
| `playpresence version` | Prints the version. |

Each command accepts `--config PATH` to use a configuration file other than the default one ([locations](configuration.md)).

## Build from source

You need [Go](https://go.dev/dl/), at the version listed in `go.mod`.

```bash
go install github.com/Dystilest/PlayPresence/cmd/playpresence@latest
```

## Docker (experimental)

The image is published with each release. Discord runs on the host, so the container must see Discord's socket; this works on Linux only.

A container has no credential store, so the sign-in is kept in an encrypted file, unlocked with a passphrase of your choice in `PLAYPRESENCE_PASSPHRASE`. The configuration and that file live in the directory you mount on `/config`.

```bash
export PLAYPRESENCE_PASSPHRASE='choose a long passphrase'
mkdir -p ~/playpresence-data
alias pp='docker run --rm -it --user "$(id -u):$(id -g)" -e PLAYPRESENCE_PASSPHRASE \
  -v "$XDG_RUNTIME_DIR/discord-ipc-0:/tmp/discord-ipc-0" \
  -v ~/playpresence-data:/config ghcr.io/dystilest/playpresence'
pp setup
pp run
```

This path has not been tried on many setups yet; please report what you find.

## Use your own Discord application (optional)

Discord shows your status on behalf of an "application", whose name appears after "Watching". PlayPresence comes with applications named after each server (for example "Jellyfin"), so you can skip this. Create your own only if you want a different name or your own fallback images; it's free.

1. Open the [Discord Developer Portal](https://discord.com/developers/applications) and create a **New Application**. Its name is what appears after "Watching".
2. Copy the **Application ID** (a long number) and put it in your [configuration](configuration.md):

   ```toml
   [discord]
   app_id = "123456789012345678"
   ```

3. Optional: in **Rich Presence > Art Assets**, upload two square images named exactly `movie` and `tv`. They are shown when no poster is found. Ready-made ones are in [`assets/discord`](../assets/discord).
