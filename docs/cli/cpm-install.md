# cpm install

Create profile directories and install wrapper scripts.

## Usage

```
cpm install [flags]
```

## Flags

- `--dry-run`: preview local changes without writing files or running cloud sync
- `--force`: force overwrite diverged files (use with --sync)
- `--sync`: re-copy mutable files from source

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
