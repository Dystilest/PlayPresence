# FAQ and troubleshooting

Run `playpresence doctor` first: it checks the configuration, Discord and your servers, and prints what to fix. Still stuck? [Ask in Discussions](https://github.com/Dystilest/PlayPresence/discussions); there are no silly questions.

## Nothing shows on my Discord profile

- Is the Discord **desktop** app running on the same machine? A web or mobile session is not enough.
- Is **Settings > Activity Privacy > Share my activity** enabled?
- If you set `discord.app_id`, is it the Application ID from the [Developer Portal](https://discord.com/developers/applications)? `doctor` reports a rejected ID. Remove the key to use the built-in application.
- You can only see your own presence on your own profile if "Share my activity" is on; ask a friend, or look at your profile card from another account.

## `doctor` says "Discord is not running"

No Discord IPC endpoint could be reached. Start the desktop app. On Linux with Flatpak or Snap, PlayPresence also looks in the sandbox locations; if yours is elsewhere, open an issue with the output of `doctor`.

## `doctor` says the handshake failed

Discord is running but refused the connection. If you set `discord.app_id`, it is almost always wrong: copy it again from the Developer Portal, or remove the key to use the built-in application.

## The presence disappears after I restart Discord

It comes back by itself, usually within a minute and at worst about 75 seconds (the activity is re-sent periodically).

## The image is the generic one

Your media server has no poster with a public address for that item (for example a home video, or an item it couldn't match on TMDb), or posters are turned off (`art.resolvers = []`). Refreshing the item's metadata in Jellyfin often fixes it. More artwork sources are [planned](roadmap.md).

## Why is the progress bar not updating every second?

It does not need to: Discord draws the bar from a start and an end time. PlayPresence only updates when something changes (pause, seek, next item), and sends at most three updates back to back, then one every 15 seconds, which keeps it within Discord's rate limit.

## Can I run it on a server?

Rich Presence works only through the desktop Discord client on the machine where you are logged in. A headless server cannot show your presence unless it can reach that client's IPC socket (see the Docker note in [installation](installation.md)).

## Where are my passwords stored?

In your operating system's credential store, never in the configuration file. See [privacy](privacy.md).

## `run` says "not signed in"

The server has no stored sign-in yet (or it was removed). Run `playpresence setup` and enter the same address: it signs you in again without adding a second server.

## `run` prints "token rejected" and nothing shows

The server no longer accepts the stored sign-in (revoked, or the server was reinstalled). Run `playpresence setup` again; other servers keep running meanwhile.

## It shows what someone else is watching

By default PlayPresence shows only the account you signed in with. If you see someone else's playback you probably set `user = "*"` or use an API key; set `user = "your name"`, see [Jellyfin](jellyfin.md#which-sessions-are-shown).

## Windows says "Windows protected your PC"

Windows shows this for programs that aren't signed with a paid code-signing certificate, which PlayPresence doesn't have yet. Click **More info**, then **Run anyway**. You can check that your download is genuine: each release lists checksums and a build attestation (see [releasing](releasing.md#verify-a-download)).

## macOS says the program "cannot be opened"

It isn't signed by Apple yet. In Terminal, in the folder where you extracted it, run `xattr -d com.apple.quarantine playpresence` once.

## The window closes as soon as I open it

A double-clicked `playpresence.exe` stays open, asks its questions, and waits for Enter before closing on an error (older versions just closed). If it still closes, open **Command Prompt** in that folder and type `playpresence start` to read the message.

## How do I start it with my computer?

See [Start automatically](installation.md#start-automatically).

## Does it work with music?

No: PlayPresence is about movies and TV series. Music is ignored on purpose.

## Can I hide a title?

Stop PlayPresence before you watch it (close the window or press Ctrl+C: the status is cleared at once), or turn off **Share my activity** in Discord for a while.
