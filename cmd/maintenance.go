package cmd

import (
	"github.com/spf13/cobra"
)

func versionCmd() *cobra.Command {
	var prerelease bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			app.PrintVersion(prerelease)
		},
	}
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "also check prereleases (default when running one)")
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
