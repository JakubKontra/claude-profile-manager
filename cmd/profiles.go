package cmd

import (
	"bufio"
	"fmt"
	"os/exec"

	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

func installCmd() *cobra.Command {
	var opts internal.InstallOptions

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Create profile directories and install wrapper scripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			return app.InstallProfiles(cfg, opts)
		},
	}

	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "preview local changes without writing files or running cloud sync")
	cmd.Flags().BoolVar(&opts.Sync, "sync", false, "re-copy mutable files from source")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force overwrite diverged files (use with --sync)")

	return cmd
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive setup wizard for config.toml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RunInit()
		},
	}
}

func cloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <source-profile> <new-profile>",
		Short: "Clone an existing profile (without credentials)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			return app.CloneProfile(args[0], args[1], cfg)
		},
	}
}

func addCmd() *cobra.Command {
	var shareProjects, inheritMCP bool
	var opts internal.AddOptions
	var envFlags []string

	cmd := &cobra.Command{
		Use:   "add <profile>",
		Short: "Add a profile to config.toml and install it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := internal.ParseEnvFlag(envFlags)
			if err != nil {
				return err
			}
			opts.Env = env
			if cmd.Flags().Changed("share-projects") {
				opts.ShareProjects = &shareProjects
			}
			if cmd.Flags().Changed("inherit-mcp") {
				opts.InheritMCP = &inheritMCP
			}
			if stdinIsTerminal() {
				if err := app.ChooseIsolation(bufio.NewReader(app.In), &opts); err != nil {
					return err
				}
			}
			return app.AddProfile(args[0], opts)
		},
	}

	cmd.Flags().BoolVar(&shareProjects, "share-projects", true, "allow project history sharing (false isolates projects)")
	cmd.Flags().BoolVar(&inheritMCP, "inherit-mcp", true, "inherit global MCP servers")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "human-readable description")
	cmd.Flags().StringVarP(&opts.Model, "model", "m", "", "default model (e.g. sonnet, opus)")
	cmd.Flags().StringArrayVar(&opts.AddDirs, "add-dir", nil, "extra directory passed via --add-dir (repeatable)")
	cmd.Flags().StringArrayVarP(&envFlags, "env", "e", nil, "environment variable KEY=VALUE (repeatable)")
	cmd.Flags().BoolVar(&opts.NoInstall, "no-install", false, "only update config.toml, do not run install")

	return cmd
}

func removeCmd() *cobra.Command {
	var opts internal.RemoveOptions

	cmd := &cobra.Command{
		Use:   "remove <profile>",
		Short: "Remove a profile (config entry and wrapper; --purge deletes its data too)",
		Long: `Remove a profile's wrapper script and its [profiles.<name>] section from
config.toml. Comment lines inside that section are removed with it.

With --purge the profile directory, its .credentials.json and (on macOS)
its Keychain entry are deleted as well. This asks for confirmation unless
--yes is given.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadCloudConfig()
			if err != nil {
				return err
			}
			opts.Stdin = app.In
			opts.IsTerminal = stdinIsTerminal()
			return app.RemoveProfile(cfg, args[0], opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Purge, "purge", false, "also delete the profile directory and credentials")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "do not ask for confirmation")

	return cmd
}

func editCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open config.toml in $VISUAL / $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.EditConfig(app.Getenv, func(c *exec.Cmd) error { return c.Run() })
		},
	}
}

func loginCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "login [profile...]",
		Short: "Run claude auth login for profiles, one at a time",
		Long: `Run 'claude auth login' inside each named profile, one after another, and
print a summary at the end.

With --all every profile without usable credentials is logged in. Profiles
that are already logged in, or that authenticate through ANTHROPIC_API_KEY,
ANTHROPIC_AUTH_TOKEN, CLAUDE_CODE_OAUTH_TOKEN, Bedrock, Vertex or Foundry,
are skipped.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return fmt.Errorf("name the profiles to log in, or pass --all")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			return app.Login(cfg, args, all)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "log in every profile without usable credentials")
	return cmd
}

func renameCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a profile, keeping its login",
		Long: `Rename a profile: its config and secrets.toml tables, [[match]] rules,
directory, wrapper script and, on macOS, its Keychain login, whose name
derives from the directory. The account stays logged in.

If a step fails, the steps already done are reverted. Close Claude Code
sessions of the profile first.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			return app.RenameProfile(cfg, args[0], args[1], dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the steps without changing anything")
	return cmd
}
