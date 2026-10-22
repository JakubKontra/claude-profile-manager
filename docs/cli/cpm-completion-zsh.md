# cpm completion zsh

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(cpm completion zsh)

To load completions for every new session, execute once:

#### Linux:

	cpm completion zsh > "${fpath[1]}/_cpm"

#### macOS:

	cpm completion zsh > $(brew --prefix)/share/zsh/site-functions/_cpm

You will need to start a new shell for this setup to take effect.

## Usage

```
cpm completion zsh [flags]
```

## Flags

- `--no-descriptions`: disable completion descriptions

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm completion](cpm-completion.md).
