package cmd

import (
	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

func cloudCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloud",
		Short: "Sync settings across machines via git",
		Long:  "Synchronize Claude Code settings, plugins, skills, and commands across devices\nusing a private git repository.",
	}

	cmd.AddCommand(cloudInitCmd(), cloudPushCmd(), cloudPullCmd(), cloudDiffCmd(), cloudStatusCmd(), cloudRemoteCmd())

	return cmd
}

func cloudInitCmd() *cobra.Command {
	var allowSecrets bool
	var remote string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize cloud sync repo",
		Long:  "Initialize a local git repo for syncing settings.\nIf --remote points to an existing repo, it will be cloned instead.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CloudInitWithSecrets(remote, allowSecrets)
		},
	}

	cmd.Flags().BoolVar(&allowSecrets, "allow-secrets", false, "allow credentials in the initial cloud commit")
	cmd.Flags().StringVar(&remote, "remote", "", "git remote URL (e.g. git@github.com:user/claude-settings.git)")

	return cmd
}

func cloudPushCmd() *cobra.Command {
	var opts internal.PushOptions

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push local settings to cloud repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CloudPush(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Message, "message", "m", "", "custom commit message")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show what would be committed without pushing")
	cmd.Flags().BoolVar(&opts.AllowSecrets, "allow-secrets", false, "push even if settings contain values that look like API keys")

	return cmd
}

func cloudDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Show local changes since the last sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CloudDiff()
		},
	}
}

func cloudPullCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull settings from cloud repo and apply locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CloudPull(dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without applying")

	return cmd
}

func cloudStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cloud sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CloudStatus()
		},
	}
}

func cloudRemoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remote <url>",
		Short: "Set or update the git remote URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.CloudRemote(args[0])
		},
	}
}
