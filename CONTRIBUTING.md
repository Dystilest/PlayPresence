# Contributing to PlayPresence

Thanks for your interest! This project aims to be easy to contribute to: a small codebase, fast tests that need neither Discord nor a media server, and clear conventions. You don't need to write code to help.

## Ways to help

### Without writing code

- **Try it and tell us** what worked and what didn't: that's the most useful thing right now.
- **Answer or ask questions** in [Discussions](https://github.com/Dystilest/PlayPresence/discussions).
- **Fix the documentation:** if a step confused you, edit the page on GitHub (the pencil icon) and propose the change. No tools needed.
- **Translate** the documentation.

### With code

- **Report a bug** with the [bug template](https://github.com/Dystilest/PlayPresence/issues/new?template=bug_report.yml). Paste the output of `playpresence doctor` and redact tokens and private addresses.
- **Suggest a feature** with the feature template. Explain the use case first.
- **Add a media server**: see [Adding a media server](docs/adding-a-source.md). Jellyfin, Plex and Emby are the priority; others are welcome.
- Look for issues labelled `good first issue`, and see the [roadmap](docs/roadmap.md) for what's planned.

For anything larger than a small fix, open an issue first so we can agree on the approach before you invest time.

## Development setup

You need:

- Go, at the version in `go.mod`. Windows: `winget install GoLang.Go`; macOS: `brew install go`; Linux: your package manager or [go.dev/dl](https://go.dev/dl/).
- `make` (optional; every target is a plain `go` command you can run by hand).
- For linting: [golangci-lint](https://golangci-lint.run/) (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`), and for the vulnerability check `go install golang.org/x/vuln/cmd/govulncheck@latest`. For the race detector: a C compiler (`gcc`), because `-race` needs cgo. CI always runs all of them on Windows, macOS and Linux, so it's fine to skip what you can't install.

```bash
git clone https://github.com/Dystilest/PlayPresence.git
cd PlayPresence
make test        # go test ./...
make race        # go test -race ./...
make lint        # golangci-lint run
make vuln        # govulncheck ./...
make build       # builds ./playpresence
```

No Discord, no media server and no network are needed to run the tests.

## Project layout

See [Architecture](docs/architecture.md). In short: `internal/source` (media servers) feeds `internal/app` (orchestration), which renders with `internal/render` and publishes through `internal/presence`. `cmd/playpresence` is the command line.

## Making a change

1. Create a branch from `main`: `feat/short-name`, `fix/short-name` or `docs/short-name`.
2. **Write a failing test first**, make it pass, then clean up. A bug fix comes with a test that fails without the fix.
3. Keep the change focused; unrelated cleanups go in their own pull request.
4. Run `gofmt`, `go vet ./...`, `make test` and `make lint`.
5. If you change configuration keys, regenerate the reference: `go run ./internal/config/gen` (a test fails when `docs/configuration.md` is stale).
6. Update the docs and add an entry under *Unreleased* in `CHANGELOG.md`.
7. Open a pull request using the template.

### Commit messages

[Conventional Commits](https://www.conventionalcommits.org/): `feat(render): ...`, `fix(presence): ...`, `docs: ...`, `test: ...`, `chore: ...`. One logical change per commit.

### Code conventions

- Standard Go style; `gofmt` and `go vet` clean.
- Return errors with context (`fmt.Errorf("...: %w", err)`); no panics for expected failures.
- Pass `context.Context` and honour cancellation; every network call has a timeout.
- Tests use temporary directories whose names contain spaces and accents, to catch path bugs early on Windows.

### Security rules

- Never put tokens, keys or passwords in code, tests, logs or issues.
- Secrets go through `internal/secrets`.
- Report vulnerabilities privately, see [SECURITY.md](SECURITY.md).

## Trying it against a real Discord

```bash
go run ./cmd/playpresence doctor --config path/to/config.toml
go run ./cmd/playpresence run --demo
```

You need the Discord desktop app running. `--app-id` is optional: without it the built-in application is used (see [installation](docs/installation.md#use-your-own-discord-application-optional)).

## Review process

A maintainer reviews every pull request, usually within a few days. Expect questions rather than demands; we would rather help you land a good change than turn it down.

## Code of Conduct

By participating you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
