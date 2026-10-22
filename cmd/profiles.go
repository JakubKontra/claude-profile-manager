package cmd

import (
	"bufio"
	"os"
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
			cfg, _, err := loadCfg()
			if err != nil {
				return err
			}
			return internal.InstallProfiles(cfg, configPath, opts)
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
			return internal.RunInit(configPath)
		},
	}
}

func cloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <source-profile> <new-profile>",
		Short: "Clone an existing profile (without credentials)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadCfg()
			if err != nil {
				return err
			}
			return internal.CloneProfile(args[0], args[1], configPath, cfg)
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
				if err := internal.ChooseIsolation(bufio.NewReader(os.Stdin), &opts); err != nil {
					return err
				}
			}
			return internal.AddProfile(configPath, args[0], opts)
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
			cfg, err := internal.LoadCloudConfig(configPath)
			if err != nil {
				return err
			}
			opts.Stdin = os.Stdin
			opts.IsTerminal = stdinIsTerminal()
			return internal.RemoveProfile(cfg, configPath, args[0], opts)
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
			return internal.EditConfig(configPath, os.Getenv, func(c *exec.Cmd) error { return c.Run() })
		},
	}
}
