# FAQ and troubleshooting

Run `playpresence doctor` first: it checks the configuration, Discord and the sources, and prints what to fix.

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

No artwork was found for that item, or the artwork services in `art.resolvers` are disabled. Upload `movie` and `tv` images to your Discord application so the fallback looks good (see [installation](installation.md#use-your-own-discord-application-optional)).

## Why is the progress bar not updating every second?

It does not need to: Discord draws the bar from a start and an end time. PlayPresence only updates when something changes (pause, seek, next item), and sends at most three updates back to back, then one every 15 seconds, which keeps it within Discord's rate limit.

## Can I run it on a server?

Rich Presence works only through the desktop Discord client on the machine where you are logged in. A headless server cannot show your presence unless it can reach that client's IPC socket (see the Docker note in [installation](installation.md)).

## Where are my passwords stored?

In your operating system's credential store, never in the configuration file. See [privacy](privacy.md).

## `run` says "not signed in"

The source has no stored token yet (or it was removed). Run `playpresence auth jellyfin --source NAME`, using the name from your `[sources.NAME]` section.

## `run` prints "token rejected" and nothing shows

The server no longer accepts the stored token (revoked, or the server was reinstalled). Sign in again with `playpresence auth jellyfin --source NAME`; the other sources keep running meanwhile.

## It shows what someone else is watching

By default PlayPresence shows only the account you signed in with. If you see someone else's playback you probably set `user = "*"` or use an API key; set `user = "your name"`, see [Jellyfin](jellyfin.md#which-sessions-are-shown).
