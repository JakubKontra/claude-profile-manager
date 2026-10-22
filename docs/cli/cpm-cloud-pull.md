# cpm cloud pull

Pull the cloud repo and apply its files locally. Profiles added on other
machines are added to config.toml.

Profiles removed or renamed on other machines are listed and, after you
confirm (or with --yes), removed or renamed here too. A removal keeps the
profile's data, like 'cpm remove' without --purge.

## Usage

```
cpm cloud pull [flags]
```

## Flags

- `--dry-run`: show what would change without applying
- `-y, --yes`: apply profile removals and renames from other machines without asking

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm cloud](cpm-cloud.md).
