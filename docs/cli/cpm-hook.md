# cpm hook

Print shell hook for auto-switching.

  eval "$(cpm hook)"                    # .zshrc / .bashrc
  cpm hook --shell fish | source        # config.fish
  eval "$(cpm hook --default personal)" # fall back to a profile outside projects

The hook switches to the profile in the nearest .claude-profile file. When
config.toml has [[match]] rules, it applies them too, so start a new shell
after adding the first rule.

## Usage

```
cpm hook [flags]
```

## Flags

- `--default string`: profile to use when no .claude-profile or [[match]] rule applies
- `--shell string`: shell: bash, zsh or fish (default: from $SHELL)

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm](cpm.md).
