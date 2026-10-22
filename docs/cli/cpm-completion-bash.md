# cpm completion bash

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(cpm completion bash)

To load completions for every new session, execute once:

#### Linux:

	cpm completion bash > /etc/bash_completion.d/cpm

#### macOS:

	cpm completion bash > $(brew --prefix)/etc/bash_completion.d/cpm

You will need to start a new shell for this setup to take effect.

## Usage

```
cpm completion bash
```

## Flags

- `--no-descriptions`: disable completion descriptions

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm completion](cpm-completion.md).
