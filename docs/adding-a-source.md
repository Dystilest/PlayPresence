# Adding a media server

A new integration is one package that implements `source.Source`. Nothing else in the program needs to change except registering it.

## The contract

```go
type Source interface {
    Name() string
    Watch(ctx context.Context, out chan<- session.Session) error
}
```

`Watch`:

- sends a `session.Session` on `out` **every time something changes** (state, item, a seek, a periodic position refresh);
- sends a session with `State: session.StateStopped` (or simply stops sending) when nothing plays any more;
- returns an error when the backend is lost (connection refused, token rejected). The orchestrator clears that source's session and restarts it with backoff, so do not retry forever inside `Watch`;
- must return promptly when `ctx` is cancelled, including while blocked on `out`.

Fill as much of the session as the server knows: kind, title, year, season and episode, artist and album, external ids (`session.IDs`: TMDb, IMDb, TVDB, MusicBrainz), position and duration. The ids drive the buttons and the artwork lookups.

## Skeleton

```go
package mysource

type Source struct {
    name string
    // client, URL, token read from the secrets store...
}

func (s *Source) Name() string { return s.name }

func (s *Source) Watch(ctx context.Context, out chan<- session.Session) error {
    for {
        sessions, err := s.fetch(ctx) // one authenticated read request
        if err != nil {
            return err
        }
        for _, sess := range sessions {
            select {
            case out <- sess:
            case <-ctx.Done():
                return ctx.Err()
            }
        }
        select {
        case <-time.After(5 * time.Second):
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

Prefer a push channel (a WebSocket) when the server offers one, and fall back to polling.

## Rules

- Read only. Never change anything on the server.
- Credentials come from `internal/secrets`, never from the configuration file, and never appear in logs or error messages (redact URLs that embed a token).
- Use timeouts on every request.
- Filter by the configured `user` when set.

## Tests

Test against a fake server (`net/http/httptest`) that returns recorded responses, not against a real one. Cover: a playing item, a paused item, a stop, an expired token (an error is returned), a server that disappears mid-session, and a cancelled context. See `internal/source/fake` and the orchestrator tests for patterns.

## Checklist for a pull request

- [ ] `make test` and `make lint` pass.
- [ ] The kind is added to `config.SourceKinds` and to the reference (`go run ./internal/config/gen`).
- [ ] A short setup guide in `docs/`, and the supported-servers table in the README is updated.
- [ ] `CHANGELOG.md` has an entry under *Unreleased*.
