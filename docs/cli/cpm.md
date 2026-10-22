# cpm

Claude Profile Manager — manage multiple Claude Code accounts.

## Usage

```
cpm [flags]
```

## Flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

## Commands

- [cpm add](cpm-add.md): Add a profile to config.toml and install it
- [cpm adopt](cpm-adopt.md): Use an existing Claude Code login for a profile
- [cpm clone](cpm-clone.md): Clone an existing profile (without credentials)
- [cpm cloud](cpm-cloud.md): Sync settings across machines via git
- [cpm completion](cpm-completion.md): Generate the autocompletion script for the specified shell
- [cpm credentials](cpm-credentials.md): Show credential status for all profiles
- [cpm direnv](cpm-direnv.md): Print .envrc snippet for a profile
- [cpm doctor](cpm-doctor.md): Diagnose issues with profiles and configuration
- [cpm edit](cpm-edit.md): Open config.toml in $VISUAL / $EDITOR
- [cpm exec](cpm-exec.md): Run any command with a profile's environment
- [cpm hook](cpm-hook.md): Print shell hook for auto-switching via .claude-profile files and [[match]] rules
- [cpm init](cpm-init.md): Interactive setup wizard for config.toml
- [cpm install](cpm-install.md): Create profile directories and install wrapper scripts
- [cpm link](cpm-link.md): Create .claude-profile in current directory (like .nvmrc)
- [cpm list](cpm-list.md): List all configured profiles
- [cpm login](cpm-login.md): Run claude auth login for profiles, one at a time
- [cpm prompt](cpm-prompt.md): Print current profile name for shell prompt (PS1/starship)
- [cpm remove](cpm-remove.md): Remove a profile (config entry and wrapper; --purge deletes its data too)
- [cpm rename](cpm-rename.md): Rename a profile, keeping its login
- [cpm run](cpm-run.md): Run claude with a specific profile (one-shot)
- [cpm status](cpm-status.md): Show sync status of all profiles
- [cpm uninstall](cpm-uninstall.md): Remove wrappers and shell hook lines (--purge: profiles and logins too)
- [cpm unlink](cpm-unlink.md): Remove .claude-profile from current directory
- [cpm upgrade](cpm-upgrade.md): Upgrade cpm to the latest version from GitHub Releases
- [cpm use](cpm-use.md): Switch the current shell to a profile (use with eval)
- [cpm version](cpm-version.md): Print version information
- [cpm which](cpm-which.md): Show the currently active profile
