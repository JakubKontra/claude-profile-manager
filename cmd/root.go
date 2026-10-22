package cmd

import (
	"errors"
	"os"
	"strings"

	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

var (
	configPath string
	// app is built from the parsed flags before any subcommand runs.
	app *internal.App
)

// Execute runs the cpm root command and exits with internal.ExitCode.
func Execute() {
	root := NewRoot()
	err := run(root)
	if err != nil {
		// Sentinel errors have already printed their own report.
		if !errors.Is(err, internal.ErrDoctorFailed) && !errors.Is(err, internal.ErrDiverged) {
			root.PrintErrln("Error:", err)
		} else {
			root.PrintErrln(err)
		}
	}
	os.Exit(internal.ExitCode(err))
}

// NewRoot builds the cpm command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "cpm",
		Short:         "Claude Profile Manager — manage multiple Claude Code accounts",
		Long:          internal.Banner + "\n  Manage multiple Claude Code accounts with isolated profiles.\n  https://github.com/jakubkontra/claude-profile-manager",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		app = internal.NewApp(configPath)
	}
	root.PersistentFlags().StringVar(&configPath, "config", internal.DefaultConfigPath(), "path to config file")

	root.AddCommand(
		installCmd(),
		listCmd(),
		statusCmd(),
		direnvCmd(),
		useCmd(),
		whichCmd(),
		resolveCmd(),
		initCmd(),
		doctorCmd(),
		runCmd(),
		execCmd(),
		cloneCmd(),
		addCmd(),
		adoptCmd(),
		removeCmd(),
		renameCmd(),
		editCmd(),
		promptCmd(),
		credentialsCmd(),
		loginCmd(),
		hookCmd(),
		linkCmd(),
		unlinkCmd(),
		versionCmd(),
		upgradeCmd(),
		cloudCmd(),
	)

	// Bad flags and arguments are usage errors (exit code 2).
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return internal.UsageError(err) })
	markUsageErrors(root)
	return root
}

// run executes root. Cobra reports an unknown command as a plain error, so
// it is recognized by its message and marked as a usage error.
func run(root *cobra.Command) error {
	err := root.Execute()
	if err != nil && strings.HasPrefix(err.Error(), "unknown command ") {
		return internal.UsageError(err)
	}
	return err
}

// markUsageErrors wraps every command's argument validator so its errors
// are usage errors.
func markUsageErrors(cmd *cobra.Command) {
	if validate := cmd.Args; validate != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			return internal.UsageError(validate(c, args))
		}
	}
	for _, sub := range cmd.Commands() {
		markUsageErrors(sub)
	}
}

func cwd() (string, error) {
	return os.Getwd()
}

func stdinIsTerminal() bool { return internal.IsTerminal(os.Stdin.Fd()) }
