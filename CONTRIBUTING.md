# Contributing

Thanks for helping with cpm. This page covers how the code is organized and what every change has to keep working.

## Before you open a pull request

```bash
go vet ./...
go test ./... -race
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
```

CI runs the same checks on Linux and macOS. One pull request covers one change; larger work follows an item in [ROADMAP.md](ROADMAP.md).

## Layout

- `cmd/` holds the cobra commands. They parse flags and call `internal`.
- `internal/` holds everything else. Commands are methods on `App`, which carries the config path, the output streams, the home directory, the environment, the Keychain and the git runner. Tests build an `App` around a temp dir and fakes (`internal/testutil_test.go`), so they run in parallel and never touch your real setup.
- `integration_test.go` builds the real binary and drives it with fake `claude`, `security` and `git` remotes.

## Compatibility checks

From 1.0, [docs/stability.md](docs/stability.md) lists what must not break. Tests pin each part. When one of them fails, either your change broke the contract or the contract changes on purpose. In the second case, regenerate the file and say so in the pull request:

| What | File | Regenerate |
|------|------|-----------|
| Commands and flags | `cmd/testdata/cli-surface.txt` | `go test ./cmd -update` |
| `--json` output | `internal/testdata/json/*.json` | `go test ./internal -update` |
| Command reference | `docs/cli/*.md` | `go run . docs --markdown docs/cli` |

Fields and commands may be added. Removing or renaming them waits for 2.0.

## Releases

Each release gets a compatibility fixture made by that release, so later versions are tested against real files:

```bash
internal/testdata/compat/generate.sh v0.8.0
```

Then add the tag to `compatVersions` in `internal/compat_test.go`.

To prepare a release, add a `CHANGELOG.md` section and bump the version in `flake.nix` (both `version` and the `ldflags` line). After merging, tag the merge commit. A tag with a suffix such as `v1.0.0-rc.1` becomes a GitHub prerelease, which Homebrew and `cpm upgrade` skip.

## Fuzzing

`internal/fuzz_test.go` fuzzes the parsers of user input. `go test` runs the seeds, and the weekly Fuzz workflow explores further. To fuzz one target locally:

```bash
go test ./internal -run '^$' -fuzz '^FuzzGlobMatch$' -fuzztime 60s
```

A failing input is written to `internal/testdata/fuzz/`. Commit it together with the fix.
