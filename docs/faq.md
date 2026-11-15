# FAQ and troubleshooting

Run `playpresence doctor` first: it checks the configuration, Discord and the sources, and prints what to fix.

## Nothing shows on my Discord profile

- Is the Discord **desktop** app running on the same machine? A web or mobile session is not enough.
- Is **Settings > Activity Privacy > Share my activity** enabled?
- Is `discord.app_id` the Application ID from the [Developer Portal](https://discord.com/developers/applications)? `doctor` reports a rejected ID.
- You can only see your own presence on your own profile if "Share my activity" is on; ask a friend, or look at your profile card from another account.

## `doctor` says "Discord is not running"

No Discord IPC endpoint could be reached. Start the desktop app. On Linux with Flatpak or Snap, PlayPresence also looks in the sandbox locations; if yours is elsewhere, open an issue with the output of `doctor`.

## `doctor` says the handshake failed

Discord is running but refused the connection, almost always because `discord.app_id` is wrong. Copy it again from the Developer Portal.

## The presence disappears after I restart Discord

It comes back by itself within about a minute (the activity is re-sent periodically).

## The image is the generic one

No artwork was found for that item, or the artwork services in `art.resolvers` are disabled. Upload `movie`, `tv` and `music` images to your Discord application so the fallback looks good (see [installation](installation.md#create-your-discord-application)).

## Why is the progress bar not updating every second?

It does not need to: Discord draws the bar from a start and an end time. PlayPresence only updates when something changes (pause, seek, next item), and never more often than every 15 seconds.

## Can I run it on a server?

Rich Presence works only through the desktop Discord client on the machine where you are logged in. A headless server cannot show your presence unless it can reach that client's IPC socket (see the Docker note in [installation](installation.md)).

## Where are my passwords stored?

In your operating system's credential store, never in the configuration file. See [privacy](privacy.md).
