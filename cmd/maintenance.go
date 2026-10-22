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
