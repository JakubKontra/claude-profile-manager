package cmd

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

func direnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "direnv <profile>",
		Short: "Print .envrc snippet for a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			snippet, err := app.Direnv(cfg, args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(app.Out, snippet)
			return nil
		},
	}
}

func useCmd() *cobra.Command {
	var opts internal.UseOptions
	cmd := &cobra.Command{
		Use:   "use [profile|auto]",
		Short: "Switch the current shell to a profile (use with eval)",
		Long: `Switch the current shell to a profile.

  eval "$(cpm use work)"               # bash / zsh
  cpm use --shell fish work | source   # fish
  eval "$(cpm use auto)"               # profile from .claude-profile or [[match]]
  eval "$(cpm use)"                    # interactive picker (menu on stderr)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			dir, err := cwd()
			if err != nil {
				return err
			}
			if opts.Shell == "" {
				opts.Shell = internal.DetectShell(app.Getenv)
			}
			var name string
			if len(args) == 1 {
				name = args[0]
			} else {
				if !stdinIsTerminal() {
					return errors.New("profile name required (or run interactively)")
				}
				name, err = internal.SelectProfileInteractive(cfg, app.CurrentProfile(), app.In, app.Err)
				if err != nil {
					return err
				}
			}
			snippet, err := app.Use(cfg, name, dir, opts)
			if err != nil {
				return err
			}
			fmt.Fprint(app.Out, snippet)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Shell, "shell", "", "shell syntax: bash, zsh or fish (default: from $SHELL)")
	cmd.Flags().BoolVarP(&opts.Quiet, "quiet", "q", false, "do not print the 'Switched to profile' message")
	return cmd
}

func whichCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "which",
		Short: "Show the currently active profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := cwd()
			if err != nil {
				return err
			}
			cfg, err := app.LoadCloudConfig()
			if err != nil {
				cfg = &internal.Config{} // which still reports env and .claude-profile
			}
			result := app.Which(cfg, dir)
			if asJSON {
				return app.PrintJSON(result)
			}
			app.RenderWhich(result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

// resolveCmd is what the shell hook calls on every directory change when
// [[match]] rules exist. It prints the profile name, or nothing.
func resolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "resolve [dir]",
		Short:  "Print the profile for a directory (used by the shell hook)",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := cwd()
			if len(args) == 1 {
				dir, err = args[0], nil
			}
			if err != nil {
				return err
			}
			cfg, err := app.LoadCloudConfig()
			if err != nil {
				return err
			}
			if res, ok := app.ResolveProfile(cfg, dir); ok {
				fmt.Fprintln(app.Out, res.Profile)
			}
			return nil
		},
	}
}

func runCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <profile> [claude args...]",
		Short: "Run claude with a specific profile (one-shot)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			spec, err := app.BuildRunExec(cfg, args[0], args[1:])
			if err != nil {
				return err
			}
			return syscall.Exec(spec.Path, spec.Argv, spec.Env)
		},
	}
	// Everything after the profile name belongs to claude, including flags.
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func execCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <profile> -- <command> [args...]",
		Short: "Run any command with a profile's environment",
		Long:  "Run an arbitrary command with the profile's CLAUDE_CONFIG_DIR, CLAUDE_PROFILE\nand env variables set. Useful for scripts, git hooks or CI.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			spec, err := app.BuildExecExec(cfg, args[0], args[1:])
			if err != nil {
				return err
			}
			return syscall.Exec(spec.Path, spec.Argv, spec.Env)
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func promptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Print current profile name for shell prompt (PS1/starship)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(app.Out, app.PromptString())
		},
	}
}

func hookCmd() *cobra.Command {
	var opts internal.HookOptions
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Print shell hook for auto-switching via .claude-profile files and [[match]] rules",
		Long: `Print shell hook for auto-switching.

  eval "$(cpm hook)"                    # .zshrc / .bashrc
  cpm hook --shell fish | source        # config.fish
  eval "$(cpm hook --default personal)" # fall back to a profile outside projects

The hook switches to the profile in the nearest .claude-profile file. When
config.toml has [[match]] rules, it applies them too, so start a new shell
after adding the first rule.`,
		Run: func(cmd *cobra.Command, args []string) {
			if opts.Shell == "" {
				opts.Shell = internal.DetectShell(app.Getenv)
			}
			if cfg, err := app.LoadCloudConfig(); err == nil && len(cfg.Match) > 0 {
				opts.Resolve = true
			}
			if configPath != internal.DefaultConfigPath() {
				opts.ConfigPath = configPath
			}
			fmt.Fprint(app.Out, internal.GenerateShellHook(opts))
		},
	}
	cmd.Flags().StringVar(&opts.Shell, "shell", "", "shell: bash, zsh or fish (default: from $SHELL)")
	cmd.Flags().StringVar(&opts.DefaultProfile, "default", "", "profile to use when no .claude-profile or [[match]] rule applies")
	return cmd
}

func linkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link <profile>",
		Short: "Create .claude-profile in current directory (like .nvmrc)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			dir, err := cwd()
			if err != nil {
				return err
			}
			return app.Link(cfg, dir, args[0])
		},
	}
}

func unlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink",
		Short: "Remove .claude-profile from current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := cwd()
			if err != nil {
				return err
			}
			return app.Unlink(dir)
		},
	}
}
