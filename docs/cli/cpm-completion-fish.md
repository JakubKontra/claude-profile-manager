# cpm completion fish

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	cpm completion fish | source

To load completions for every new session, execute once:

	cpm completion fish > ~/.config/fish/completions/cpm.fish

You will need to start a new shell for this setup to take effect.

## Usage

```
cpm completion fish [flags]
```

## Flags

- `--no-descriptions`: disable completion descriptions

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm completion](cpm-completion.md).
