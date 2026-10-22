package cmd

import (
	"errors"
	"os"

	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

var configPath string

// Execute runs the cpm root command and exits with status 1 on error.
func Execute() {
	root := &cobra.Command{
		Use:           "cpm",
		Short:         "Claude Profile Manager — manage multiple Claude Code accounts",
		Long:          internal.Banner + "\n  Manage multiple Claude Code accounts with isolated profiles.\n  https://github.com/jakubkontra/claude-profile-manager",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&configPath, "config", internal.DefaultConfigPath(), "path to config file")

	root.AddCommand(
		installCmd(),
		listCmd(),
		statusCmd(),
		direnvCmd(),
		useCmd(),
		whichCmd(),
		initCmd(),
		doctorCmd(),
		runCmd(),
		execCmd(),
		cloneCmd(),
		addCmd(),
		removeCmd(),
		editCmd(),
		promptCmd(),
		credentialsCmd(),
		hookCmd(),
		linkCmd(),
		unlinkCmd(),
		versionCmd(),
		upgradeCmd(),
		cloudCmd(),
	)

	if err := root.Execute(); err != nil {
		// Sentinel errors have already printed their own report.
		if !errors.Is(err, internal.ErrDoctorFailed) && !errors.Is(err, internal.ErrDiverged) {
			root.PrintErrln("Error:", err)
		} else {
			root.PrintErrln(err)
		}
		os.Exit(1)
	}
}

func loadCfg() (*internal.Config, string, error) {
	cfg, err := internal.LoadConfig(configPath)
	if err != nil {
		return nil, "", err
	}
	return cfg, internal.ProfilesBaseDir(configPath), nil
}

func cwd() (string, error) {
	return os.Getwd()
}

func stdinIsTerminal() bool { return internal.IsTerminal(os.Stdin.Fd()) }
