# cpm rename

Rename a profile: its config and secrets.toml tables, [[match]] rules,
directory, wrapper script and, on macOS, its Keychain login, whose name
derives from the directory. The account stays logged in.

If a step fails, the steps already done are reverted. Close Claude Code
sessions of the profile first.

## Usage

```
cpm rename <old> <new> [flags]
```

## Flags

- `--dry-run`: show the steps without changing anything
- `--json`: output the steps as JSON (progress goes to stderr)

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
