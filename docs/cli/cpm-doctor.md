# cpm doctor

Diagnose issues with profiles and configuration.

## Usage

```
cpm doctor [flags]
```

## Flags

- `--fix`: repair what cpm install can (missing or outdated wrappers, directories, symlinks)
- `--json`: output as JSON
- `--verify`: read Keychain tokens to check expiry (macOS, may prompt)

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
