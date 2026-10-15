# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-10-15

Thanks [@JakubKontra](https://github.com/JakubKontra) for the improvements in this release.

### Added
- `cpm install --dry-run` previews local changes with redacted diffs, using the same plan as installation; supports `--sync` and `--force` ([#10](https://github.com/JakubKontra/claude-profile-manager/pull/10)).
- Local `secrets.toml` profile environment overlays, protected by `0600` permissions and loaded at runtime instead of embedded in wrappers or direnv files ([#8](https://github.com/JakubKontra/claude-profile-manager/pull/8)).
- Interactive history/MCP isolation choices in `init` and `add`, plus `--share-projects`, `--inherit-mcp` and the `inherit_mcp` configuration field ([#11](https://github.com/JakubKontra/claude-profile-manager/pull/11)).
- End-to-end CLI coverage with isolated homes, fake Claude/Keychain executables, two-machine cloud scenarios and race detection in Linux/macOS CI ([#12](https://github.com/JakubKontra/claude-profile-manager/pull/12)).

### Fixed
- Cloud pull preserves local additions and edits, detects conflicts against the last successful transfer, and backs up overwritten/deleted files before applying changes ([#7](https://github.com/JakubKontra/claude-profile-manager/pull/7)).
- Failed cloud operations return errors instead of reporting success; profile resync conflicts propagate to automation ([#6](https://github.com/JakubKontra/claude-profile-manager/pull/6)).
- Initial cloud commits and pushes check synced TOML/JSON for secrets, including profile env and MCP headers; explicit `--allow-secrets` remains available ([#8](https://github.com/JakubKontra/claude-profile-manager/pull/8)).
- Wrapper installation rejects foreign files/symlinks, sanitizes multiline descriptions and writes atomically; legacy wrappers containing credentials use owner-only permissions ([#5](https://github.com/JakubKontra/claude-profile-manager/pull/5)).
- `remove --purge` works after removing a profile from config, including the last profile; doctor reports orphan data and purge removes local secrets ([#9](https://github.com/JakubKontra/claude-profile-manager/pull/9)).
- `run`/`exec` consume the optional `--` command separator; `/dev/null` is no longer mistaken for an interactive terminal, and runtime direnv output uses POSIX syntax even with a fish login shell ([#12](https://github.com/JakubKontra/claude-profile-manager/pull/12)).

### Upgrade notes
- Existing configs and noninteractive profile creation retain their sharing defaults. Newly created interactive profiles default to isolated project history and no global MCP inheritance.
- Move credentials from portable config into `~/.claude-profiles/secrets.toml`, run `chmod 600` and then `cpm install`. Profiles with local secrets require `cpm` on PATH.
- A first cloud pull without a recorded baseline refuses differing local files. Reconcile them before retrying. Backups and restore paths are stored locally under `.cpm/backups/` next to config.toml.
- Previously committed credentials remain in Git history; rotate them and clean history separately. Secret detection remains heuristic.

## [0.3.1] - 2026-10-15

### Added
- Nix flake (`nix run`, `nix profile install`, NixOS flake input). Thanks [@Stefanomarton](https://github.com/Stefanomarton) ([#4](https://github.com/jakubkontra/claude-profile-manager/pull/4)).

## [0.3.0] - 2026-09-25

### Added
- `cpm add`, `cpm remove [--purge] [--yes]` and `cpm edit` to manage profiles without editing `config.toml` by hand; `cpm clone` now registers the clone in the config.
- `cpm exec <profile> -- <cmd>` runs any command with a profile's environment.
- New profiles are seeded with `.claude.json` (onboarding flags, theme, install method) so they skip the onboarding wizard and get MCP servers on first launch.
- Per-profile MCP servers: `mcp_exclude` and `[profiles.<name>.mcp_servers.<id>]`.
- Per-profile `settings.json` overrides via `[profiles.<name>.settings]` (deep merge).
- Configurable directory sharing: global `share`, per-profile `share` / `isolate` (e.g. a private `projects/`).
- `--json` output for `list`, `which`, `credentials` and `doctor`.
- `cpm doctor` checks the shell hook, `git` (when cloud sync is configured), unknown config keys and outdated wrappers; `--verify` reads the macOS Keychain token to report expiry and subscription.
- fish shell support for `cpm use --shell fish` and `cpm hook --shell fish`; `cpm hook --default <profile>`; `cpm use` without an argument shows a picker.
- Cloud sync: `skills/` is synced, `cloud.include`, `cloud.auto_pull_on_install`, `cloud.allow_secrets`, `cpm cloud push --dry-run`, `cpm cloud diff`, and a pre-push check that refuses to sync `settings*.json` env values that look like API keys.
- `cpm cloud pull` re-syncs the profiles' copies of `settings.json` / `CLAUDE.md`.
- `cloud.auto_push` (documented since 0.2.0) is now implemented.
- Profiles record a baseline of each copied file, so `install --sync` distinguishes local edits from upstream changes.
- `golangci-lint` configuration and CI lint step; this changelog.

### Changed
- Wrapper scripts are generated from a shared launch spec with proper shell quoting; the first `cpm install` after upgrading rewrites every `claude-<name>` script.
- `cpm use` records what it exported in `CPM_MANAGED_VARS`; the hook unsets only those instead of every `CLAUDE_*` / `ANTHROPIC_*` variable, prints the switch message once, and uses `PROMPT_COMMAND` in bash instead of aliasing `cd`.
- `settings.local.json` is no longer synced to the cloud by default (add it back with `cloud.include`).
- `cpm doctor` exits with code 1 when a check fails; `cpm install --sync` exits with code 1 when it refuses to overwrite diverged files.
- `cpm upgrade` replaces the running binary (not `bin_dir/cpm`), verifies the SHA-256 against `checksums.txt` and defers to `brew upgrade` for Homebrew installs.
- `LoadConfig` rejects invalid profile names (letters, digits, `-`, `_` only) and invalid environment variable names.
- `cpm run` honours `--config` and bypasses profile flags for subcommands (`auth`, `mcp`, ...) like the wrapper does.
- `cpm credentials` shows expiry time, subscription type and organization.

### Fixed
- `cpm cloud pull` never wrote profiles pulled from the cloud to `config.toml`.
- `cpm cloud remote` silently failed to save the URL when the config contained the commented `# [cloud]` example.
- `cpm list` reported macOS Keychain logins as "installed" instead of "authenticated".
- Credential expiry was never detected: the parser did not know Claude Code's `claudeAiOauth.expiresAt` format.
- `cloud.exclude` was ignored on pull/init; files deleted on another machine were never removed locally; `--dry-run` omitted directories and the config.
- `cpm cloud init --remote` with an unreachable remote silently created a fresh repo; an init with no syncable files skipped adding the remote.
- Version check and upgrade had no HTTP timeout.
- Relative symlink targets were reported as broken and re-created on every install.
- Env values containing quotes or `$` broke generated wrappers and `cpm use` output.
- Output order of `status`, `doctor` and `cpm use` depended on map iteration.

## [0.2.1] - 2026-08-17

### Fixed
- Detect macOS Keychain logins in `cpm doctor` and `cpm credentials`.
- `cpm install` no longer deletes the `cpm` binary from `bin_dir`.

## [0.2.0] - 2026-04-13

### Added
- Cloud sync (`cpm cloud init|push|pull|status|remote`) via a private git repository.

## [0.1.0] - 2026-04-13

### Added
- Initial release: profiles, wrapper scripts, `.claude-profile` auto-switching, doctor, clone, upgrade.

[Unreleased]: https://github.com/jakubkontra/claude-profile-manager/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/jakubkontra/claude-profile-manager/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/jakubkontra/claude-profile-manager/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/jakubkontra/claude-profile-manager/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/jakubkontra/claude-profile-manager/compare/v0.1.1...v0.2.0
[0.1.0]: https://github.com/jakubkontra/claude-profile-manager/releases/tag/v0.1.0
