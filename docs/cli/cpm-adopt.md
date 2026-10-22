# cpm adopt

Copy the login of an existing Claude Code setup (by default ~/.claude) into
a profile, so the profile needs no new login. A profile that is not in the
config yet is added and installed first.

The Keychain login (macOS) or .credentials.json is copied, along with the
account shown by Claude Code. With --move the login is deleted from the
source afterwards. If a step fails, the steps already done are reverted.

## Usage

```
cpm adopt <profile> [flags]
```

## Flags

- `--dry-run`: show the steps without changing anything
- `--from string`: Claude Code config dir to take the login from (default ~/.claude)
- `--json`: output the steps as JSON (progress goes to stderr)
- `--move`: delete the login from the source afterwards

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
