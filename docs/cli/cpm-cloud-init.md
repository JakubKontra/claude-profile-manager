# cpm cloud init

Initialize a local git repo for syncing settings.
If --remote points to an existing repo, it will be cloned instead.

## Usage

```
cpm cloud init [flags]
```

## Flags

- `--allow-secrets`: allow credentials in the initial cloud commit
- `--remote string`: git remote URL (e.g. git@github.com:user/claude-settings.git)

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm cloud](cpm-cloud.md).
