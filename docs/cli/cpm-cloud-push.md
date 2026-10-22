# cpm cloud push

Push local settings to cloud repo.

## Usage

```
cpm cloud push [flags]
```

## Flags

- `--allow-secrets`: push even if settings contain values that look like API keys
- `--dry-run`: show what would be committed without pushing
- `-m, --message string`: custom commit message

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm cloud](cpm-cloud.md).
