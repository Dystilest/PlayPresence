# Default presentation

This page describes how a session is shown on Discord today. Customisable templates are planned; until then these rules apply to every source.

## Fields

| Media | Status line (Discord "Watching") | Details | State | Image tooltip |
|---|---|---|---|---|
| Movie | Watching | `Title (Year)` | empty | Title |
| Episode | Watching | Series title | `S01E02 · Episode title` | Series title |

- The text next to your name uses the **details** line (`status_display_type` 2).
- A progress bar is shown while playing, drawn from a start and end time.
- While **paused**, there is no progress bar and the state line reads `Paused · 12:34` (or `1:02:03` past an hour).
- When playback **stops**, the presence is removed.
- Details, state and tooltips are cut at 128 UTF-16 code units (an emoji counts as 2) with an ellipsis, never in the middle of a character.
- An empty title is shown as `Unknown`.

## Buttons

At most two, shown when the identifier is known:

| Media | Buttons |
|---|---|
| Movie | TMDb, IMDb |
| Episode | TMDb (series), IMDb |

A button whose label exceeds 32 characters or whose link exceeds 512 characters is dropped, following Discord's limits.

## Images

The large image is the poster your media server already uses for the item (for an episode, the season poster, then the series poster). It is sent to Discord as a public image address, never as an address on your server. If no poster is found, the application images `movie` or `tv` are shown; the built-in applications already have them, and if you use your own application, upload them to it ([how](installation.md#use-your-own-discord-application-optional)).
