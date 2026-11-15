# Releasing

For maintainers.

## Cut a release

1. Make sure `main` is green in CI.
2. Move the entries under *Unreleased* in `CHANGELOG.md` to a new version heading with today's date, and commit (`chore: release vX.Y.Z`).
3. Tag and push the tag:

   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```

The *Release* workflow then:

- builds static binaries (no cgo) for Linux, Windows and macOS on amd64 and arm64 with GoReleaser, publishes the archives and `checksums.txt` on the GitHub release, with an SBOM for each archive and an auto-generated changelog;
- attests the build provenance of the checksums file;
- builds and pushes the Docker image to `ghcr.io/<owner>/playpresence` (tags `X.Y.Z`, `X.Y`, `latest`) and attests it.

## Verify a download

```bash
gh attestation verify playpresence_X.Y.Z_linux_amd64.tar.gz --repo Dystilest/PlayPresence
```

and compare the archive with `checksums.txt`.

## Local dry run

```bash
goreleaser check
goreleaser release --snapshot --clean   # builds into ./dist, publishes nothing
```

## Known limitation: unsigned Windows binaries

Windows binaries are not Authenticode-signed (this needs a code-signing certificate). Windows SmartScreen or an antivirus may therefore warn about a new, unsigned executable. Provenance attestations and checksums let you verify what you downloaded. Signing is planned once a certificate is available.
