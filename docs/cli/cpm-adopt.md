# cpm adopt

Move the login of an existing Claude Code setup (by default ~/.claude) into
a profile, so the profile needs no new login. A profile that is not in the
config yet is added and installed first.

The Keychain login (macOS) or .credentials.json is copied, along with the
account shown by Claude Code, and then deleted from the source. If a step
fails, the steps already done are reverted.

The source is logged out because an OAuth login is valid in one place only:
when one copy refreshes its token, the other copy stops working. --copy
keeps the login in the source anyway. To keep the source logged in and give
the profile its own login, use cpm login instead.

## Usage

```
cpm adopt <profile> [flags]
```

## Flags

- `--copy`: keep the login in the source too (the two then share one token, and one gets logged out)
- `--dry-run`: show the steps without changing anything
- `--from string`: Claude Code config dir to take the login from (default ~/.claude)
- `--json`: output the steps as JSON (progress goes to stderr)
- `--move`: delete the login from the source afterwards (the default; kept for compatibility)

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
