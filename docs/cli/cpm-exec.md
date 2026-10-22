# cpm exec

Run an arbitrary command with the profile's CLAUDE_CONFIG_DIR, CLAUDE_PROFILE
and env variables set. Useful for scripts, git hooks or CI.

## Usage

```
cpm exec <profile> -- <command> [args...] [flags]
```

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
