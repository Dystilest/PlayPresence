# Architecture

PlayPresence is a small pipeline. Each stage is a Go package with one job and a narrow interface.

```
source (one per media server)
   |  session.Session on every change
   v
app.Orchestrator  --- picks the session to show (priority), artwork fallback
   |  render.Activity
   v
presence.Publisher --- coalesces, rate limits, retries, resends
   |  SET_ACTIVITY
   v
presence.IPCClient --- Discord's local IPC (named pipe or unix socket)
```

## Packages

| Package | Responsibility |
|---|---|
| `internal/session` | `Session`: the server-neutral description of what is playing, with `Validate` and `Window` (progress bar start and end). |
| `internal/source` | The `Source` interface every media server integration implements. `fake` provides a scripted source for demos and tests. |
| `internal/render` | Turns a `Session` into an `Activity` (fields, buttons, truncation). Pure, no I/O. |
| `internal/presence` | Discord IPC framing, endpoint discovery, `IPCClient`, rate `Limiter`, and the `Publisher`. |
| `internal/app` | `Orchestrator`: source supervision, priority, artwork, suppression of redundant updates. |
| `internal/config` | TOML loading, strict validation, generated reference. |
| `internal/secrets` | Credential storage: OS keyring, with an encrypted file fallback. |
| `cmd/playpresence` | The command line: `setup`, `run`, `start`, `doctor`, `auth`, `version`. |

## Behaviours worth knowing

- **Sources** emit a session whenever something changes and return an error when they lose their backend. The orchestrator forgets that source's session and restarts it with exponential backoff (2 s doubling to 30 s) without affecting the others.
- **Priority:** when several sources play, the first one in `presence.priority` wins; the others follow alphabetically. When it stops, the next takes over.
- **Redundant updates:** if only the progress window drifts by less than 3 seconds (normal polling jitter), nothing is re-sent. A seek, a pause or a new item is.
- **Publisher:** the last desired activity wins; up to three updates may go out at once (so a pause shows immediately), then one per `presence.min_interval` (15 s minimum), which stays inside Discord's limit; the same activity is not re-sent; when Discord is unreachable it retries with backoff and sends the pending activity once it returns; the current activity is re-sent every 60 s so it reappears after a Discord restart; it is cleared on shutdown.
- **IPC client:** tries every known endpoint (including Flatpak, Snap and Vesktop locations), bounds payload sizes, uses deadlines everywhere, reconnects once transparently when a reused connection went stale, and reports a refused handshake distinctly from "not running".
- **Secrets** never reach the configuration file or logs.

## Testing approach

Everything is tested without Discord or a real media server: a fake Discord IPC server for the client and the CLI, scripted or channel-fed sources for the orchestrator, and temporary directories (with spaces and accents in their names) for files.
