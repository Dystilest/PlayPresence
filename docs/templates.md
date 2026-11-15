# Default presentation

This page describes how a session is shown on Discord today. Customisable templates are planned; until then these rules apply to every source.

## Fields

| Media | Status line (Discord "Watching" or "Listening to") | Details | State | Image tooltip |
|---|---|---|---|---|
| Movie | Watching | `Title (Year)` | empty | Title |
| Episode | Watching | Series title | `S01E02 · Episode title` | Series title |
| Track | Listening to | Track title | Artist | Album |

- The text next to your name uses the **details** line (`status_display_type` 2).
- A progress bar is shown while playing, drawn from a start and end time.
- While **paused**, there is no progress bar and the state line reads `Paused · 12:34` (or `1:02:03` past an hour).
- When playback **stops**, the presence is removed.
- Details and state are cut at 128 characters with an ellipsis, never in the middle of a character.
- An empty title is shown as `Unknown`.

## Buttons

At most two, shown when the identifier is known:

| Media | Buttons |
|---|---|
| Movie | TMDb, IMDb |
| Episode | TMDb (series), IMDb |
| Track | MusicBrainz |

A button whose label exceeds 32 characters or whose link exceeds 512 characters is dropped, following Discord's limits.

## Images

The large image is the artwork found by your artwork resolvers. If none is found, the application assets `movie`, `tv` or `music` are used, so upload those to your Discord application.
