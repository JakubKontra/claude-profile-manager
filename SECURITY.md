# Security

## Reporting a vulnerability

Please report security issues privately through [GitHub security advisories](https://github.com/JakubKontra/claude-profile-manager/security/advisories/new), not in public issues.

## What cpm does with your credentials

cpm manages Claude Code logins but never sends them anywhere. It never contacts Anthropic. The only network calls are `git` for `cpm cloud`, to the remote you configure, and the GitHub Releases API for `cpm version` and `cpm upgrade`.

**Logins.** Claude Code stores each profile's login in the macOS Keychain or, on Linux, in the profile's `.credentials.json`. cpm reads or writes a login only when a command needs it:
- `cpm rename` and `cpm adopt` copy the login to its new place.
- `cpm remove --purge` and `cpm uninstall --purge` delete it.
- `cpm doctor --verify` reads it to check expiry.

When cpm writes to the Keychain, the secret goes to `security` on stdin, never on a command line where `ps` would show it.

**Secrets in the config.** Put API keys and tokens in `secrets.toml` next to `config.toml`, never in `config.toml` itself. cpm requires `secrets.toml` to be a regular file with mode `0600`. Wrapper scripts and direnv snippets never contain the values; they are read when the profile starts.

**Cloud sync.**
- `secrets.toml` and `.credentials.json` are never synced.
- Before `cloud init` and `cloud push`, cpm scans the synced files for values that look like secrets and refuses to commit them unless you pass `--allow-secrets` or set `cloud.allow_secrets`.
- The scan is a heuristic. Keep the sync repository private.

**Self-update.** `cpm upgrade` checks the downloaded binary against the release's `checksums.txt` and refuses to install a binary that does not match.
