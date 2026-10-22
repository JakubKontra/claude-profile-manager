# cpm add

Add a profile to config.toml and install it.

## Usage

```
cpm add <profile> [flags]
```

## Flags

- `--add-dir stringArray`: extra directory passed via --add-dir (repeatable)
- `-d, --description string`: human-readable description
- `-e, --env stringArray`: environment variable KEY=VALUE (repeatable)
- `--inherit-mcp`: inherit global MCP servers (default true)
- `-m, --model string`: default model (e.g. sonnet, opus)
- `--no-install`: only update config.toml, do not run install
- `--share-projects`: allow project history sharing (false isolates projects) (default true)

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
