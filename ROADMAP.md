# Roadmap

This is the plan for the next releases of cpm. Refactoring comes first so the code is easier to test. New features are built on top of it.

Each item lists **Why**, **Scope**, **Steps** and **Done when**. Each numbered item is its own PR.

---

## Phase 1: Refactor

Behavior stays the same. For every PR, `go test -race ./...`, `golangci-lint run` and the end-to-end suite in `integration_test.go` must pass.

### 1.1 Move cobra commands out of `main.go`

- **Why:** `main.go` is 610 lines and defines all ~28 commands inline (`installCmd()` … `cloudCmd()`), so a single command is hard to find.
- **Scope:** `main.go`, new package `cmd/`.
- **Steps:**
  - `cmd/root.go`: the root command, the `--config` flag, `loadCfg()` and `Execute()`. This includes the sentinel-error handling for `ErrDoctorFailed` and `ErrDiverged`.
  - `cmd/profiles.go`: install, add, remove, clone, edit, init.
  - `cmd/switching.go`: use, run, exec, link, unlink, hook, direnv, prompt, which.
  - `cmd/inspect.go`: list, status, doctor, credentials.
  - `cmd/cloud.go`: `cloud` and its subcommands.
  - `cmd/maintenance.go`: version, upgrade.
  - `main.go` only calls `cmd.Execute()`.
- **Done when:** `main.go` is under 20 lines, and the `cpm --help` output and every subcommand's `--help` match a snapshot taken before the change.

### 1.2 Introduce an `App` context

- **Why:** Package-level mutable state (`stdout` in `internal/output.go`, swapped via `SetOutput`) and a `configPath string` threaded through ~85 call sites block `t.Parallel()` and make signatures noisy.
- **Scope:** `internal/output.go`, every function that takes `configPath`, and the tests.
- **Steps:**
  - Add a context struct:
    ```go
    type App struct {
        ConfigPath string
        BaseDir    string
        Out, Err   io.Writer
        Run        Runner        // see 1.3
        Keychain   KeychainStore // see 1.3
        Now        func() time.Time
    }
    ```
    `NewApp(configPath)` fills in the production defaults.
  - Migrate one area per commit:
    1. Turn the output helpers (`outf`, `outln`, `out`, `PrintJSON`) into methods on `App`.
    2. Turn the cloud functions (`CloudInit`, `CloudPush`, `CloudPull`, …) into methods.
    3. Do the same for install, add, remove and clone.
    4. Do the same for doctor, credentials and list.
  - Remove `SetOutput` once no test uses it.
- **Done when:** command output and config paths go through `App`, `SetOutput` is gone, and tests that do not touch process-wide state run with `t.Parallel()`.
- **Status:** done. `App` holds `ConfigPath`, `BaseDir`, `In`, `Out` and `Err`; `Run`, `Keychain` and `Now` arrive with 1.3. 143 of 189 unit tests run in parallel. The rest set environment variables (mostly `HOME`) or stub package variables, see 1.3 and 1.4.

### 1.3 `Runner` and `KeychainStore` interfaces

- **Why:** `git` (`gitExecDir` in `internal/cloud.go`) and `security` (`internal/credentials.go`) are called through `exec.Command` directly, so unit tests need fake executables on `PATH`. The Keychain calls are already injectable through package variables (`keychainLookup`, `keychainRead`, `keychainDelete`). This step turns that into a proper interface.
- **Scope:** `internal/cloud.go`, `internal/credentials.go`, `internal/testutil_test.go`.
- **Steps:**
  - Add `type Runner interface { Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) }` with a production `ExecRunner`.
  - Add `type KeychainStore interface { Exists(service, account string) bool; Read(...) ([]byte, error); Write(...) error; Delete(...) error }`.
    - The darwin implementation goes in a build-tagged file, following `terminal_darwin.go` and `terminal_linux.go`.
    - The Linux implementation is a no-op.
  - Add in-memory fakes in `testutil_test.go`.
- **Done when:** unit tests never call `git` or `security`, and fake executables are used only in `integration_test.go`.
- **Status:** done, with three deviations from the plan:
  - `Runner.Run` takes no `context.Context`, because nothing cancels a git call yet.
  - `KeychainStore` has `Exists`, `Read` and `Delete`. `Write` comes with 2.3, the first feature that needs it.
  - The cloud round-trip tests still run real git against a bare repo, because they test git behavior itself. The fake runner covers failure paths (failed push, failed pull, unreachable remote).

  No unit test touches the real Keychain any more. 161 of 192 unit tests run in parallel. The rest set environment variables (see 1.4) or stub the HTTP and executable globals in `version.go`.

### 1.4 Inject home directory and environment

- **Why:** `os.UserHomeDir` (in `cloud.go`, `config.go`, `doctor.go`, `sync.go`) and `os.Getenv` are read directly, so tests change them with `t.Setenv`, which cannot be combined with `t.Parallel()`.
- **Scope:** the functions above, `cmd_which.go`, `shell.go`, `launch.go` and the tests that call `t.Setenv`.
- **Steps:**
  - Add `Home string` and `Getenv func(string) string` to `App`. `NewApp` fills them from the process.
  - Read the home directory and environment through `App` instead of `os`.
  - Replace `t.Setenv` in unit tests with fields on the test `App`.
- **Done when:** unit tests no longer call `t.Setenv`, and every unit test that does not stub a package variable runs in parallel.
- **Status:** done.
  - `App` has `Home` and `Env []string`, read through `a.Getenv`. A slice is simpler to set in tests than a `Getenv` function. `App.lookPath` searches the `PATH` in `Env`.
  - `ExecRunner.Env` adds variables to git's environment, so the cloud tests isolate git without `t.Setenv`.
  - Unit tests no longer read the real home directory. Before, tests that did not set `HOME` could pick up the user's `~/.claude.json` and other synced files.
  - 188 of 193 unit tests run in parallel. The other 5 stub `httpClient`, `githubAPIBase` and `executablePath` in `version.go`.

### 1.5 Split `internal/` into packages (optional, after 1.2)

- **Why:** `internal/` is one flat package of ~10k lines, and `cloud.go` alone is 916 lines.
- **Target packages:**
  - `config`: config, tomledit, secrets.
  - `profile`: install_plan, sync, wrapper, launch, clone, isolation.
  - `cloud`: split into push, pull/distribute, secret scan and config merge.
  - `creds`: credentials and Keychain.
  - `shell`: hook, use, direnv, prompt, link.
- **Note:** Do this after 1.2, because `App` sets the dependency boundaries. Move one package per PR.

---

## Phase 2: Features

### 2.1 Auto-match profiles by git remote or path

- **Why:** Today every repository needs `cpm link` and a `.claude-profile` file, because `DetectProfileFile` (`internal/shell.go`) looks for nothing else.
- **Config:**
  ```toml
  [[match]]
  remote = "github.com/company/*"
  profile = "work"

  [[match]]
  path = "~/Work/client/**"
  profile = "client"
  ```
- **Resolution order:**
  1. The nearest `.claude-profile`.
  2. The first `[[match]]` entry that matches, in file order.
  3. `hook --default`.
  4. Otherwise the profile is unset.
- **Steps:**
  - Read the remote URL from `.git/config` directly instead of running `git`, so the hook stays fast on every `cd`. Normalize SSH, HTTPS and `git@host:` forms before matching.
  - Put the logic in one resolver used by `cpm use auto`, the hook and `cpm which`.
  - `cpm which` reports where the profile came from (`matched remote github.com/company/*`).
  - `cpm doctor` checks that every `match.profile` exists.
  - Update the README config reference.
- **Done when:** unit tests cover the resolution order, glob matching and remote normalization, and an end-to-end test switches profile on `cd` without any `.claude-profile`.
- **Status:** done.
  - Without `[[match]]` rules the hook is the same pure-shell script as before. With rules, it calls the hidden `cpm resolve` on every directory change, which takes about 5 ms.
  - A rule matches if any of the repository's remotes matches, not only `origin`.
  - The hook now passes `--config` to cpm when it is not the default path.
  - `cpm cloud pull` adds `[[match]]` rules missing locally, after the local ones, so local rules keep priority. It also adds a global `indicator` (2.2) when the local config does not set one.

### 2.2 Profile indicator inside Claude Code

- **Why:** The most common mistake is working under the wrong account, and nothing inside the Claude Code UI shows which profile is active.
- **Config:** `indicator = true`, set globally or per profile.
- **Steps:**
  - `cpm install` sets `statusLine` in the profile's `settings.json` to show the profile name.
  - It does this only if neither `settings.json` nor `profiles.<name>.settings` already defines a `statusLine`.
  - The injected value is part of the baseline in `.cpm/baseline/`, so `install --sync` does not report it as a local edit.
- **Done when:** `install --dry-run` shows the change, and an existing `statusLine` is never overwritten.
- **Status:** done.
  - It is off by default. `indicator = true` turns it on globally, and `profiles.<name>.indicator` overrides it.
  - The status line runs `echo 'cpm: <profile>'`. A profile without any `settings.json` gets one with only the `statusLine`.
  - Instead of storing it in the baseline, the divergence check compares `settings.json` as JSON and ignores the line cpm injected, so turning the indicator on or off is never reported as a local edit. A side effect: formatting-only edits to a profile's `settings.json` no longer count as local changes.
  - Turning it off removes the line only if it is still exactly the one cpm added.

### 2.3 `cpm rename <old> <new>`

- **Why:** On macOS the Keychain service name is derived from `sha256(CLAUDE_CONFIG_DIR)` (`KeychainServiceName` in `internal/credentials.go`). Renaming the profile directory by hand therefore loses the login.
- **Steps:**
  1. Validate the new name with the same rules as `add`.
  2. Rename the TOML table in `config.toml` (`tomledit.go`) and in `secrets.toml`.
  3. Move the profile directory.
  4. On macOS, read the token from the old service, write it under the new one, then delete the old entry. This adds `Write` to `KeychainStore` (1.3). Keep the secret off the `security` command line, where `ps` would show it.
  5. Write the new `claude-<new>` wrapper and remove the old one.
  6. Warn about `.claude-profile` files and `[[match]]` entries that still use the old name.
  7. Support `--dry-run`.
- **Rollback:** Each step records an undo action. On failure, the steps already done are reverted in reverse order.
- **Done when:** after a rename, `claude-<new> auth status` shows the same account with no new login.
- **Status:** done.
  - Unlike the plan, `[[match]]` rules that use the old name are renamed in `config.toml` instead of only warned about. cpm edits that file anyway, and the result is checked by parsing it again. Only `.claude-profile` files get a reminder, because cpm cannot find them.
  - The old Keychain entry is deleted last, after every other step succeeded, so a rollback never has to restore a secret.
  - `KeychainStore.Write` sends `add-generic-password -X <hex>` to `security -i` on stdin, so the token never appears on a command line. The copy is read back and compared before the rename continues.
  - Checked by hand against the real macOS Keychain with a throwaway entry. Unit tests use the in-memory Keychain, including a failed write and a rollback from the last step.
  - Other machines still have the old name after `cloud pull`, which only adds the new profile.

### 2.4 `cpm login [profile|--all]`

- **Why:** Setting up a new machine or a new set of profiles means running `claude-<name>` for each profile one by one.
- **Steps:**
  - Use `GetCredentialStatus` (`internal/credentials.go`) to find profiles with no valid credentials.
  - Run `claude-<name> auth login` for each, one at a time, through `LaunchSpec`.
  - Print a summary at the end.
  - When a profile has no login, `cpm doctor` suggests `cpm login <name>`.
- **Done when:** an end-to-end test with the fake `claude` binary checks the order and that profiles already logged in are skipped.
- **Status:** done.
  - `cpm login <name>...` logs in the named profiles even when they are logged in, which is how you switch a profile to another account. `--all` takes every profile without usable credentials (missing or expired).
  - `--all` also skips profiles that authenticate through an API key, a token or a cloud provider (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `CLAUDE_CODE_USE_BEDROCK`/`VERTEX`/`FOUNDRY`). `cpm doctor` now reports those as "no login needed" instead of "not authenticated".
  - A failed login does not stop the others. The command exits non-zero after the summary.
  - `App.Attach` runs the interactive login, so unit tests record the calls without starting a process.

---

## Phase 3: Road to 1.0

1.0 is a stability promise. After 1.0, these do not change incompatibly: the config format, commands and flags, `--json` output, exit codes, the wrapper and hook contract (including the hidden `cpm resolve`), the layout of `~/.claude-profiles/` and the layout of the cloud repository. The Keychain service names (they mirror Claude Code), human-readable text and the Go API in `internal/` are not part of the promise.

Releases: 0.6 (contract), 0.7 (profile lifecycle), 0.8 (quality), then `v1.0.0-rc.1` with no new features, and `v1.0.0` after at least 14 days of use. Each item is its own PR against `main`.

### 0.6: Stability contract

#### 3.1 Prerelease-safe releases
- **Why:** `.goreleaser.yaml` sets neither `release.prerelease` nor `brews.skip_upload`, so a `v1.0.0-rc.1` tag would become a normal release and the Homebrew formula. `cpm upgrade` compares tags by equality only (`internal/version.go`), so it would move an RC user back to the latest stable.
- **Steps:**
  - `.goreleaser.yaml`: `release.prerelease: auto`, `brews[0].skip_upload: auto`, `archives.formats`.
  - `release.yml`: run the same lint and `-race` matrix as `test.yml` before releasing.
  - `version.go`: a semver comparison that orders prereleases. `upgrade` and `version` act only when the latest release is newer. `--prerelease` also considers prereleases.
- **Done when:** tests cover a stable user who never sees an RC, an RC user who is never downgraded and an RC user who is offered the final release.
- **Status:** done.
  - A build that is itself a prerelease checks prereleases without the flag, so RC testers get `rc.2` and then the final release.
  - `goreleaser check` passes apart from the existing `brews` deprecation. Moving to `homebrew_casks` changes the tap layout and is left for later.

#### 3.2 Config version
- **Why:** Without a version, cpm cannot tell the user that a config comes from a newer cpm, and the format cannot evolve after 1.0.
- **Steps:**
  - `version = 1` in `config.toml`. A missing version means 1. A higher version fails with "needs a newer cpm".
  - Add a migration hook for future versions.
  - New configs get `version = 1`. Existing files are not rewritten.
  - `cloud pull` refuses a config with a higher version. `cpm doctor` reports the version.
- **Done when:** a missing version, `version = 1` and `version = 2` load, fail and migrate as described.
- **Status:** done.
  - `writeConfigAtomic` adds `version = 1` when it creates a file (`add`, `clone`, `cloud remote` on a new config), and `init` writes it.
  - Migrations change only the loaded config. They are keyed by the version they start from.

#### 3.3 Exit codes
- **Why:** Every error exits 1, so a script cannot tell a typo from a failed check.
- **Steps:**
  - Split `cmd/root.go` into `NewRoot()` and `Execute()` so the command tree can be tested.
  - Exit codes: 0 ok, 1 error, 2 wrong usage (flags or arguments), 3 failed check (`doctor`, local edits blocking `install --sync`, cloud conflicts), 4 failed login.
- **Done when:** a test produces every documented code. This is a breaking change and needs an upgrade note.
- **Status:** done.
  - Every command's `Args` validator and cobra's flag errors are marked as usage errors.
  - Cobra reports an unknown command as an untyped error, so it is recognized by its message.
  - `cpm login` checks its arguments before loading the config.
  - Unit tests cover the mapping and the usage errors through `NewRoot()`. The end-to-end test checks each code with the real binary.

#### 3.4 `--json` everywhere
- **Why:** `status`, `cloud status`, `cloud diff`, `login`, `rename --dry-run` and `version` have no machine-readable output.
- **Steps:**
  - Follow the `DoctorReport` pattern: a report struct with JSON tags and a separate text renderer.
  - `cloud diff --json` lists files and their status, not patches.
  - Golden files in `internal/testdata/json/`. JSON goes to stdout only on success, errors stay on stderr, fields are only ever added, and `detail` texts are free-form.
- **Done when:** every `--json` output has a golden file.
- **Status:** done.
  - `App.JSON` sends progress messages and the output of the programs cpm runs to stderr, so stdout holds only the JSON document.
  - `rename --json` also works without `--dry-run` and lists the steps done.
  - Goldens replace temp dirs with `$ROOT` and Keychain hashes with `HASH`. `go test ./internal -update` rewrites them.
  - `cloud status` used to list modified files in random order; they are now sorted.

#### 3.5 CLI review and `docs/stability.md`
- **Why:** Inconsistencies would be frozen for all of 1.x.
- **Steps:**
  - Rules: `--dry-run` on every command that deletes or overwrites data. A prompt with `--yes` only for irreversible deletion; without a terminal and without `--yes` the command refuses. Add `remove --dry-run`.
  - `cmd/testdata/cli-surface.txt` lists every command and flag, checked by a test.
  - `docs/stability.md` lists what the promise covers and what it does not.
- **Done when:** the surface file is committed and the README links `docs/stability.md`.
- **Status:** done.
  - `remove --dry-run` prints the same list the `--purge` prompt shows. That list now names only what exists: the wrapper only if cpm generated it, and the config entry only if the profile is configured.
  - `cmd/testdata/cli-surface.txt` is generated from `NewRoot()`; `go test ./cmd -update` rewrites it.

### 0.7: Profile lifecycle

#### 3.6 `cpm adopt <name>`
- **Why:** Most people already use `claude` with `~/.claude`. Turning that into a profile today means logging in again.
- **Steps:**
  - Copy the login (Keychain `Claude Code-credentials` or `.credentials.json`) and `oauthAccount` into the profile. Create the profile if it does not exist.
  - Reuse the undo steps from `cmd_rename.go`.
  - `--move` deletes the source last. `--dry-run` shows the plan.
  - `cpm doctor` suggests `adopt` when only `~/.claude` has a login.
- **Done when:** unit tests cover copy, move and rollback, and the command was checked once against the real Keychain.
- **Status:** done.
  - `~/.claude` uses the bare `Claude Code-credentials` service and `~/.claude.json`. Any other `--from` directory uses the hashed service and its own `.claude.json`.
  - The step and undo runner moved from `cmd_rename.go` to `step.go`, and rename and adopt share it.
  - Checked against the real Keychain with a throwaway entry through `--from`. The real default login was not read, to avoid an access prompt.

#### 3.7 Propagate remove and rename through cloud sync
- **Why:** After a rename, other machines keep the old profile, because `cloud pull` only adds.
- **Steps:**
  - `remove` and `rename` append to `.cpm/tombstones.toml`. Sync it as `cpm/tombstones.toml`, merged by id.
  - After a pull, apply only entries that match the pulled config, through `RenameProfile` and `RemoveProfile`. Never purge. Ask first, or apply with `--yes`.
- **Done when:** an end-to-end test with two machines ends with the same profiles and the moved login, and a second pull changes nothing.
- **Status:** done.
  - One list of changes in order (`[[change]]` with `kind = "removed"` or `"renamed"`), so a chain of renames replays in order.
  - Each machine keeps the ids it has handled in `.cpm/tombstones-applied.json`, so a re-created profile is never removed by an old entry. Changes that cannot apply on a machine (for example a rename of a profile it never had) are marked as handled.
  - Until a rename is confirmed, the config merge does not add the new name as a separate profile.
  - Without a terminal and without `--yes`, `cloud pull` lists the changes and applies none. `auto_pull_on_install` never applies them.
  - Old entries are not pruned yet; the journal holds one short entry per removal or rename.

#### 3.8 `cpm uninstall`
- **Why:** There is no clean way to remove cpm; wrappers and hook lines stay behind.
- **Steps:**
  - Remove generated wrappers and the exact hook lines in rc files, with a backup.
  - `--purge` also removes credentials and `~/.claude-profiles/`.
  - Support `--dry-run` and `--yes`.
- **Done when:** after install, hook and uninstall, the rc file is byte-identical to the original minus the hook line.
- **Status:** done.
  - Only the documented hook lines are removed: `eval "$(cpm hook …)"` and `cpm hook … | source`. Other lines that mention `cpm hook` are listed but not changed.
  - Every edited rc file gets a backup next to it (`.cpm-uninstall.bak`), so it survives `--purge`.
  - `--purge` deletes only what cpm owns in the config directory (profiles, `secrets.toml`, `cloud/`, `.cpm/`, the config file), and the directory itself only when it is empty afterwards.

#### 3.9 `cpm doctor --fix`
- **Why:** Most findings have the same answer: `cpm install`.
- **Steps:**
  - Each check carries a fix hint (`fix` in JSON).
  - `--fix` runs the install once and prints the report before and after. The exit code follows the report after the fix.
- **Done when:** a deleted wrapper and a broken symlink are clean after `--fix`.
- **Status:** done.
  - `fix` holds the command that fixes a check: `cpm install`, `cpm login <name>`, `cpm adopt <profile>` or `cpm remove <name> --purge`. `--fix` runs only `cpm install`, and skips cloud auto-pull and auto-push.
  - A broken symlink is marked fixable only when its source directory exists, because only then can install relink it.

### 0.8: Quality

#### 3.10 Compatibility fixtures
- **Steps:** add configs, secrets, wrappers and cloud state from 0.3 to 0.5 in `internal/testdata/compat/`.
- **Done when:** the fixtures load, and `install --dry-run` on them plans only wrapper rewrites.

#### 3.11 Fuzz tests
- **Steps:** fuzz `tomledit`, `globMatch`, `NormalizeRemote`, `gitRemoteURLs` and `applyIndicator`. Check that the result parses again, that round trips return the input and that nothing panics. PRs run the seeds; a weekly workflow fuzzes.
- **Done when:** every target runs in CI.

#### 3.12 Supported platforms
- **Steps:** state macOS and Linux in the README, and Windows only through WSL (native Windows does not build because of `syscall.Exec`). `cpm doctor` reports the platform.
- **Done when:** the README and doctor show it.

#### 3.13 Generated docs
- **Steps:**
  - A hidden `cpm docs --markdown|--man`. CI checks that `docs/cli/` is up to date.
  - Ship man pages through Homebrew.
  - Add `CONTRIBUTING.md` and `SECURITY.md`.
- **Done when:** a help change without regenerated docs fails CI.

### 1.0

- **`v1.0.0-rc.1`:** changelog with an "Upgrading from 0.x" section and the version in `flake.nix`, nothing else. Then check:
  - the release is marked prerelease,
  - `releases/latest` and the Homebrew formula stay on 0.8,
  - `cpm upgrade` offers the RC only with `--prerelease`.
- **Soak:** at least 14 days, including:
  - daily use on two machines with cloud sync,
  - a real rename, remove and `adopt`,
  - a fresh install on macOS and Linux,
  - an upgrade from a 0.5 setup.

  A change to the CLI surface, the JSON goldens or the docs means `rc.2` and another 7 days.
- **`v1.0.0`:** only the changelog and `flake.nix` change since the last RC.

---

## After 1.0

- **Profile inheritance** (`extends = "work"`) to avoid repeating `add_dirs`, `env` and `settings` across similar profiles.
- **Fallback on rate limit** (`cpm run work --fallback personal`). First check whether Claude Code reliably reports a rate limit through its exit code.
- **`cpm statusline`**, which wraps an existing `statusLine` and adds the profile, instead of leaving the indicator off.
- **Usage statistics per profile.**
- **1.5:** split `internal/` into packages. Deferred so a large move does not land right before the freeze.
