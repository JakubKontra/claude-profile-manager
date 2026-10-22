# cpm uninstall

Remove what cpm installed outside its directory: the claude-<name> wrapper
scripts and the hook lines in shell rc files (a backup of each edited file
is kept next to it). Lines that only mention cpm are listed, not changed.

With --purge, every profile, its login (Keychain entry or .credentials.json),
secrets.toml, the cloud repo and config.toml are deleted too.

The plan is shown first and needs confirmation, or --yes.

## Usage

```
cpm uninstall [flags]
```

## Flags

- `--dry-run`: show what would be removed without changing anything
- `--purge`: also delete every profile, its login and the config
- `-y, --yes`: do not ask for confirmation

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
