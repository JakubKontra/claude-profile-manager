# cpm cloud

Synchronize Claude Code settings, plugins, skills, and commands across devices
using a private git repository.

## Usage

```
cpm cloud [flags]
```

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

## Commands

- [cpm cloud diff](cpm-cloud-diff.md): Show local changes since the last sync
- [cpm cloud init](cpm-cloud-init.md): Initialize cloud sync repo
- [cpm cloud pull](cpm-cloud-pull.md): Pull settings from cloud repo and apply locally
- [cpm cloud push](cpm-cloud-push.md): Push local settings to cloud repo
- [cpm cloud remote](cpm-cloud-remote.md): Set or update the git remote URL
- [cpm cloud status](cpm-cloud-status.md): Show cloud sync status

See also [cpm](cpm.md).
