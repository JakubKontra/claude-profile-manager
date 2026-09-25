package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

var configPath string

func main() {
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

	cmd.Flags().BoolVar(&opts.Sync, "sync", false, "re-copy mutable files from source")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force overwrite diverged files (use with --sync)")

	return cmd
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			internal.RenderProfileList(internal.ListProfiles(cfg, base))
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show sync status of all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			internal.ShowStatus(cfg, base)
			return nil
		},
	}
}

func direnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "direnv <profile>",
		Short: "Print .envrc snippet for a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			snippet, err := internal.Direnv(cfg, base, args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(os.Stdout, snippet)
			return nil
		},
	}
}

func useCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <profile>",
		Short: "Switch the current shell to a profile (use with eval)",
		Long:  "Switch the current shell to a profile.\nUsage: eval \"$(cpm use <profile>)\"\nPass \"auto\" to use the profile from the nearest .claude-profile file.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			dir, err := cwd()
			if err != nil {
				return err
			}
			snippet, err := internal.Use(cfg, base, args[0], dir)
			if err != nil {
				return err
			}
			fmt.Fprint(os.Stdout, snippet)
			return nil
		},
	}
}

func whichCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "which",
		Short: "Show the currently active profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := cwd()
			if err != nil {
				return err
			}
			internal.RenderWhich(internal.Which(dir))
			return nil
		},
	}
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

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose issues with profiles and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			report := internal.DoctorReportFor(cfg, base)
			internal.RenderDoctorReport(report)
			if !report.OK {
				return internal.ErrDoctorFailed
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
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			spec, err := internal.BuildRunExec(cfg, base, args[0], args[1:])
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
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			spec, err := internal.BuildExecExec(cfg, base, args[0], args[1:])
			if err != nil {
				return err
			}
			return syscall.Exec(spec.Path, spec.Argv, spec.Env)
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
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
			return internal.AddProfile(configPath, args[0], opts)
		},
	}

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
			cfg, _, err := loadCfg()
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

func stdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

func promptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Print current profile name for shell prompt (PS1/starship)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(os.Stdout, internal.PromptString())
		},
	}
}

func credentialsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "credentials",
		Short: "Show credential status for all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			internal.RenderCredentialReport(internal.CredentialReport(cfg, base))
			return nil
		},
	}
}

func hookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hook",
		Short: "Print shell hook for auto-switching via .claude-profile files",
		Long:  "Print shell hook for auto-switching.\nAdd to your .zshrc: eval \"$(cpm hook)\"",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(os.Stdout, internal.GenerateShellHook())
		},
	}
}

func linkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link <profile>",
		Short: "Create .claude-profile in current directory (like .nvmrc)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadCfg()
			if err != nil {
				return err
			}
			dir, err := cwd()
			if err != nil {
				return err
			}
			return internal.Link(cfg, dir, args[0])
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
			return internal.Unlink(dir)
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			internal.PrintVersion()
		},
	}
}

func upgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade cpm to the latest version from GitHub Releases",
		RunE: func(cmd *cobra.Command, args []string) error {
			return internal.Upgrade()
		},
	}
}

func cloudCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloud",
		Short: "Sync settings across machines via git",
		Long:  "Synchronize Claude Code settings, plugins, skills, and commands across devices\nusing a private git repository.",
	}

	cmd.AddCommand(cloudInitCmd(), cloudPushCmd(), cloudPullCmd(), cloudStatusCmd(), cloudRemoteCmd())

	return cmd
}

func cloudInitCmd() *cobra.Command {
	var remote string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize cloud sync repo",
		Long:  "Initialize a local git repo for syncing settings.\nIf --remote points to an existing repo, it will be cloned instead.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return internal.CloudInit(configPath, remote)
		},
	}

	cmd.Flags().StringVar(&remote, "remote", "", "git remote URL (e.g. git@github.com:user/claude-settings.git)")

	return cmd
}

func cloudPushCmd() *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push local settings to cloud repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			return internal.CloudPush(configPath, message)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "custom commit message")

	return cmd
}

func cloudPullCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull settings from cloud repo and apply locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			return internal.CloudPull(configPath, dryRun)
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
			return internal.CloudStatus(configPath)
		},
	}
}

func cloudRemoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remote <url>",
		Short: "Set or update the git remote URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return internal.CloudRemote(configPath, args[0])
		},
	}
}
