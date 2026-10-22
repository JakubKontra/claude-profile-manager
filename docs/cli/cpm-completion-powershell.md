# cpm completion powershell

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	cpm completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.

## Usage

```
cpm completion powershell [flags]
```

## Flags

- `--no-descriptions`: disable completion descriptions

## Global flags

- `--config string`: path to config file (default ~/.claude-profiles/config.toml)

See also [cpm completion](cpm-completion.md).
