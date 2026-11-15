# Security policy

## Reporting a vulnerability

Please **do not open a public issue**. Report privately through GitHub: open the repository's **Security** tab and choose **Report a vulnerability** (private vulnerability reporting).

Include what you found, how to reproduce it, the version (`playpresence version`) and your operating system. We will acknowledge the report, keep you informed, and credit you in the release notes if you wish. This is a volunteer project: we aim to respond within a week and fix confirmed issues as quickly as is reasonable.

## Supported versions

Only the latest release receives security fixes while the project is pre-1.0.

## Scope and design assumptions

PlayPresence is a local, single-user desktop program. It holds credentials for your media server and talks to the local Discord client.

In scope: credential handling (storage, logging, leaking in errors), parsing of untrusted server or Discord responses, the release artifacts and pipeline, and anything that could expose your media server or accounts.

Safeguards by design:

- Credentials live in the OS credential store; the encrypted-file fallback uses Argon2id and AES-256-GCM with a passphrase you provide. Nothing is hardcoded.
- Configuration files containing token or key fields are rejected.
- The program opens no network listener.
- Frames from Discord are size-capped and every I/O has a deadline.
- Dependencies are scanned with `govulncheck` in CI.

Out of scope: attacks that need prior control of your account on the machine, or of your Discord client.
