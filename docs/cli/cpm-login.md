# cpm login

Run 'claude auth login' inside each named profile, one after another, and
print a summary at the end.

With --all every profile without usable credentials is logged in. Profiles
that are already logged in, or that authenticate through ANTHROPIC_API_KEY,
ANTHROPIC_AUTH_TOKEN, CLAUDE_CODE_OAUTH_TOKEN, Bedrock, Vertex or Foundry,
are skipped.

## Usage

```
cpm login [profile...] [flags]
```

## Flags

- `--all`: log in every profile without usable credentials
- `--json`: output the summary as JSON (login output goes to stderr)

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
