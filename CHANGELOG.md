# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `golangci-lint` configuration and CI lint step.
- This changelog.

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

[Unreleased]: https://github.com/jakubkontra/claude-profile-manager/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/jakubkontra/claude-profile-manager/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/jakubkontra/claude-profile-manager/compare/v0.1.1...v0.2.0
[0.1.0]: https://github.com/jakubkontra/claude-profile-manager/releases/tag/v0.1.0
