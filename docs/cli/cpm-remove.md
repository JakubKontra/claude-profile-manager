# cpm remove

Remove a profile's wrapper script and its [profiles.<name>] section from
config.toml. Comment lines inside that section are removed with it.

With --purge the profile directory, its .credentials.json and (on macOS)
its Keychain entry are deleted as well. This asks for confirmation unless
--yes is given.

## Usage

```
cpm remove <profile> [flags]
```

## Flags

- `--dry-run`: show what would be removed without changing anything
- `--purge`: also delete the profile directory and credentials
- `-y, --yes`: do not ask for confirmation

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
