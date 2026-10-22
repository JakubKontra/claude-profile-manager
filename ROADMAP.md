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

### 2.2 Profile indicator inside Claude Code

- **Why:** The most common mistake is working under the wrong account, and nothing inside the Claude Code UI shows which profile is active.
- **Config:** `indicator = true`, set globally or per profile.
- **Steps:**
  - `cpm install` sets `statusLine` in the profile's `settings.json` to show the profile name.
  - It does this only if neither `settings.json` nor `profiles.<name>.settings` already defines a `statusLine`.
  - The injected value is part of the baseline in `.cpm/baseline/`, so `install --sync` does not report it as a local edit.
- **Done when:** `install --dry-run` shows the change, and an existing `statusLine` is never overwritten.

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

### 2.4 `cpm login [profile|--all]`

- **Why:** Setting up a new machine or a new set of profiles means running `claude-<name>` for each profile one by one.
- **Steps:**
  - Use `GetCredentialStatus` (`internal/credentials.go`) to find profiles with no valid credentials.
  - Run `claude-<name> auth login` for each, one at a time, through `LaunchSpec`.
  - Print a summary at the end.
  - When a profile has no login, `cpm doctor` suggests `cpm login <name>`.
- **Done when:** an end-to-end test with the fake `claude` binary checks the order and that profiles already logged in are skipped.

---

## Not planned yet

- **Profile inheritance** (`extends = "work"`) to avoid repeating `add_dirs`, `env` and `settings` across similar profiles.
- **Fallback on rate limit** (`cpm run work --fallback personal`). First check whether Claude Code reliably reports a rate limit through its exit code.
