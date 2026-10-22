# cpm use

Switch the current shell to a profile.

  eval "$(cpm use work)"               # bash / zsh
  cpm use --shell fish work | source   # fish
  eval "$(cpm use auto)"               # profile from .claude-profile or [[match]]
  eval "$(cpm use)"                    # interactive picker (menu on stderr)

## Usage

```
cpm use [profile|auto] [flags]
```

## Flags

- `-q, --quiet`: do not print the 'Switched to profile' message
- `--shell string`: shell syntax: bash, zsh or fish (default: from $SHELL)

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
