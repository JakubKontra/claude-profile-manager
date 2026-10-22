package cmd

import (
	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

func versionCmd() *cobra.Command {
	var prerelease, asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			report := internal.VersionInfo(prerelease)
			if asJSON {
				return app.PrintJSON(report)
			}
			app.PrintVersion(report)
			return nil
		},
	}
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "also check prereleases (default when running one)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func upgradeCmd() *cobra.Command {
	var prerelease bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade cpm to the latest version from GitHub Releases",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Upgrade(prerelease)
		},
	}
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "also install prereleases (default when running one)")
	return cmd
}

func uninstallCmd() *cobra.Command {
	var opts internal.UninstallOptions
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove wrappers and shell hook lines (--purge: profiles and logins too)",
		Long: `Remove what cpm installed outside its directory: the claude-<name> wrapper
scripts and the hook lines in shell rc files (a backup of each edited file
is kept next to it). Lines that only mention cpm are listed, not changed.

With --purge, every profile, its login (Keychain entry or .credentials.json),
secrets.toml, the cloud repo and config.toml are deleted too.

The plan is shown first and needs confirmation, or --yes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadCloudConfig()
			if err != nil {
				return err
			}
			opts.Stdin = app.In
			opts.IsTerminal = stdinIsTerminal()
			return app.Uninstall(cfg, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Purge, "purge", false, "also delete every profile, its login and the config")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show what would be removed without changing anything")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "do not ask for confirmation")
	return cmd
}
