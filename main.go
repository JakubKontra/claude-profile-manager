package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

var configPath string

const banner = `
   _____ ____  __  __
  / ____|  _ \|  \/  |
 | |    | |_) | \  / |
 | |    |  __/| |\/| |
 | |____| |   | |  | |
  \_____|_|   |_|  |_|
  Claude Profile Manager
`

func main() {
	root := &cobra.Command{
		Use:   "cpm",
		Short: "Claude Profile Manager — manage multiple Claude Code accounts",
		Long:  banner + "\n  Manage multiple Claude Code accounts with isolated profiles.\n  https://github.com/jakubkontra/claude-profile-manager",
	}

	root.PersistentFlags().StringVar(&configPath, "config", internal.DefaultConfigPath(), "path to config file")

	root.AddCommand(installCmd())
	root.AddCommand(listCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(direnvCmd())
	root.AddCommand(useCmd())
	root.AddCommand(whichCmd())
	root.AddCommand(initCmd())
	root.AddCommand(doctorCmd())
	root.AddCommand(runCmd())
	root.AddCommand(cloneCmd())
	root.AddCommand(promptCmd())
	root.AddCommand(credentialsCmd())
	root.AddCommand(hookCmd())
	root.AddCommand(linkCmd())
	root.AddCommand(unlinkCmd())
	root.AddCommand(versionCmd())
	root.AddCommand(upgradeCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func installCmd() *cobra.Command {
	var sync, force bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Create profile directories and install wrapper scripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			profilesBase := internal.ProfilesBaseDir(configPath)

			if sync && !force {
				diverged := internal.CheckDivergence(cfg, profilesBase)
				if len(diverged) > 0 {
					fmt.Print("\nDiverged profile files detected:\n\n")
					for _, d := range diverged {
						fmt.Printf("  %s\n", d.Details)
					}
					fmt.Println("\nMerge changes back to source first, or use: --sync --force")
					return nil
				}
			}

			activeNames := make(map[string]bool)
			names := sortedKeys(cfg.Profiles)

			for _, name := range names {
				profile := cfg.Profiles[name]
				profileDir := filepath.Join(profilesBase, name)
				scriptName := "claude-" + name

				activeNames[scriptName] = true

				fmt.Printf("\nProfile: %s\n", name)

				if err := internal.SetupProfile(name, profileDir, cfg.SourceDir, sync); err != nil {
					return fmt.Errorf("profile %s: %w", name, err)
				}

				if err := internal.PatchAttribution(profileDir, profile.Attribution); err != nil {
					return fmt.Errorf("profile %s attribution: %w", name, err)
				}

				if err := internal.SyncMCPServers(profileDir); err != nil {
					return fmt.Errorf("profile %s mcp sync: %w", name, err)
				}

				wrapper := internal.GenerateWrapper(name, profileDir, profile)
				scriptPath := filepath.Join(cfg.BinDir, scriptName)
				if err := internal.InstallWrapper(scriptPath, wrapper); err != nil {
					return fmt.Errorf("profile %s wrapper: %w", name, err)
				}
			}

			fmt.Println("\nCleanup:")
			internal.CleanupStaleScripts(cfg.BinDir, activeNames)

			fmt.Println("\nDone.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&sync, "sync", false, "re-copy mutable files from source")
	cmd.Flags().BoolVar(&force, "force", false, "force overwrite diverged files (use with --sync)")

	return cmd
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			profilesBase := internal.ProfilesBaseDir(configPath)
			names := sortedKeys(cfg.Profiles)
			current := internal.CurrentProfile()

			for _, name := range names {
				profile := cfg.Profiles[name]
				profileDir := filepath.Join(profilesBase, name)

				status := "not installed"
				if _, err := os.Stat(profileDir); err == nil {
					status = "installed"
				}

				credPath := filepath.Join(profileDir, ".credentials.json")
				if _, err := os.Stat(credPath); err == nil {
					status = "authenticated"
				}

				desc := profile.Description
				if desc == "" {
					desc = "(no description)"
				}

				marker := "  "
				if name == current {
					marker = "* "
				}

				fmt.Printf("%sclaude-%-20s %s  [%s]\n", marker, name, desc, status)
			}

			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show sync status of all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			profilesBase := internal.ProfilesBaseDir(configPath)
			diverged := internal.CheckDivergence(cfg, profilesBase)

			if len(diverged) == 0 {
				fmt.Println("All profiles are in sync with source.")
				return nil
			}

			fmt.Println("Diverged files:")
			for _, d := range diverged {
				fmt.Printf("  %s\n", d.Details)
			}
			fmt.Println("\nRun 'cpm install --sync' to re-sync.")

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
			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			name := args[0]
			if _, ok := cfg.Profiles[name]; !ok {
				available := strings.Join(sortedKeys(cfg.Profiles), ", ")
				return fmt.Errorf("unknown profile %q (available: %s)", name, available)
			}

			profilesBase := internal.ProfilesBaseDir(configPath)
			profileDir := filepath.Join(profilesBase, name)

			fmt.Print(internal.GenerateDirenvSnippet(name, profileDir))
			return nil
		},
	}
}

func useCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <profile>",
		Short: "Switch the current shell to a profile (use with eval)",
		Long:  "Switch the current shell to a profile.\nUsage: eval \"$(cpm use <profile>)\"",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			profile, ok := cfg.Profiles[name]
			if !ok {
				// Try .claude-profile auto-detection if "auto" is passed
				if name == "auto" {
					detected, err := internal.DetectProfileFile(".")
					if err != nil {
						return fmt.Errorf("no .claude-profile found in current or parent directories")
					}
					name = detected
					profile, ok = cfg.Profiles[name]
					if !ok {
						return fmt.Errorf("profile %q from .claude-profile not found in config", name)
					}
				} else {
					available := strings.Join(sortedKeys(cfg.Profiles), ", ")
					return fmt.Errorf("unknown profile %q (available: %s)", name, available)
				}
			}

			profilesBase := internal.ProfilesBaseDir(configPath)
			profileDir := filepath.Join(profilesBase, name)

			fmt.Print(internal.GenerateUseOutput(name, profileDir, profile))
			return nil
		},
	}
}

func whichCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "which",
		Short: "Show the currently active profile",
		Run: func(cmd *cobra.Command, args []string) {
			profile := internal.CurrentProfile()
			configDir := internal.CurrentConfigDir()

			if profile != "" {
				fmt.Printf("Profile:    %s\n", profile)
				fmt.Printf("Config dir: %s\n", configDir)
				fmt.Printf("Source:     environment (CLAUDE_PROFILE)\n")
				fmt.Printf("Command:    claude-%s\n", profile)
				return
			}

			// Try to detect from .claude-profile file
			detected, err := internal.DetectProfileFile(".")
			if err == nil {
				fmt.Printf("Profile:    %s\n", detected)
				fmt.Printf("Source:     .claude-profile\n")
				fmt.Printf("Command:    claude-%s\n", detected)
				fmt.Println("\nNote: profile detected from .claude-profile but not active in this shell.")
				fmt.Println("Run: eval \"$(cpm use " + detected + ")\"")
				fmt.Println("Or add to .zshrc: eval \"$(cpm hook)\"")
				return
			}

			fmt.Println("No active profile.")
			fmt.Println("  - No CLAUDE_PROFILE env var set")
			fmt.Println("  - No .claude-profile file found in current or parent directories")
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
			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			profilesBase := internal.ProfilesBaseDir(configPath)
			checks := internal.RunDoctor(cfg, profilesBase)

			fmt.Print("cpm doctor\n\n")
			internal.PrintChecks(checks)

			hasErrors := false
			for _, c := range checks {
				if c.Status == "error" {
					hasErrors = true
					break
				}
			}

			if hasErrors {
				fmt.Println("\nSome checks failed. Fix the issues above.")
			} else {
				fmt.Println("\nAll checks passed.")
			}

			return nil
		},
	}
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run <profile> [claude args...]",
		Short:              "Run claude with a specific profile (one-shot)",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			claudeArgs := args[1:]

			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			profile, ok := cfg.Profiles[name]
			if !ok {
				available := strings.Join(sortedKeys(cfg.Profiles), ", ")
				return fmt.Errorf("unknown profile %q (available: %s)", name, available)
			}

			profilesBase := internal.ProfilesBaseDir(configPath)
			profileDir := filepath.Join(profilesBase, name)

			// Build environment
			env := os.Environ()
			// Remove existing CLAUDE_*/ANTHROPIC_* vars
			filtered := env[:0]
			for _, e := range env {
				if !strings.HasPrefix(e, "CLAUDE_") && !strings.HasPrefix(e, "ANTHROPIC_") {
					filtered = append(filtered, e)
				}
			}
			filtered = append(filtered,
				fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", profileDir),
				fmt.Sprintf("CLAUDE_PROFILE=%s", name),
			)
			for k, v := range profile.Env {
				filtered = append(filtered, fmt.Sprintf("%s=%s", k, v))
			}

			// Build claude command with add-dirs and model
			fullArgs := []string{"claude"}
			for _, d := range profile.AddDirs {
				fullArgs = append(fullArgs, "--add-dir", internal.ExpandPath(d))
			}
			if profile.Model != "" {
				// Check if user passed --model
				hasModel := false
				for _, a := range claudeArgs {
					if a == "--model" || strings.HasPrefix(a, "--model=") {
						hasModel = true
						break
					}
				}
				if !hasModel {
					fullArgs = append(fullArgs, "--model", profile.Model)
				}
			}
			fullArgs = append(fullArgs, claudeArgs...)

			claudePath, err := exec.LookPath("claude")
			if err != nil {
				return fmt.Errorf("claude not found on PATH")
			}

			return syscall.Exec(claudePath, fullArgs, filtered)
		},
	}
}

func cloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <source-profile> <new-profile>",
		Short: "Clone an existing profile (without credentials)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			sourceName := args[0]
			targetName := args[1]

			if _, ok := cfg.Profiles[sourceName]; !ok {
				available := strings.Join(sortedKeys(cfg.Profiles), ", ")
				return fmt.Errorf("unknown source profile %q (available: %s)", sourceName, available)
			}

			profilesBase := internal.ProfilesBaseDir(configPath)
			return internal.CloneProfile(sourceName, targetName, profilesBase, cfg.SourceDir, cfg)
		},
	}
}

func promptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Print current profile name for shell prompt (PS1/starship)",
		Run: func(cmd *cobra.Command, args []string) {
			p := internal.PromptString()
			if p != "" {
				fmt.Print(p)
			}
		},
	}
}

func credentialsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "credentials",
		Short: "Show credential status for all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			profilesBase := internal.ProfilesBaseDir(configPath)
			names := sortedKeys(cfg.Profiles)

			for _, name := range names {
				profileDir := filepath.Join(profilesBase, name)
				account, expired, err := internal.GetCredentialInfo(profileDir)

				if err != nil {
					fmt.Printf("  claude-%-20s %s\n", name, err)
				} else {
					status := "valid"
					if expired {
						status = "EXPIRED"
					}
					fmt.Printf("  claude-%-20s %s  [%s]\n", name, account, status)
				}
			}

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
			fmt.Print(internal.GenerateShellHook())
		},
	}
}

func linkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link <profile>",
		Short: "Create .claude-profile in current directory (like .nvmrc)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				return err
			}

			if _, ok := cfg.Profiles[name]; !ok {
				available := strings.Join(sortedKeys(cfg.Profiles), ", ")
				return fmt.Errorf("unknown profile %q (available: %s)", name, available)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if err := internal.LinkProfile(cwd, name); err != nil {
				return err
			}

			fmt.Printf("Linked profile %q to %s\n", name, cwd)
			fmt.Println("\nTo auto-switch, add to your .zshrc:")
			fmt.Println("  eval \"$(cpm hook)\"")

			return nil
		},
	}
}

func unlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink",
		Short: "Remove .claude-profile from current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if err := internal.UnlinkProfile(cwd); err != nil {
				return err
			}

			fmt.Println("Removed .claude-profile")
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(banner)
			fmt.Printf("  Version: %s\n  Commit:  %s\n", internal.Version, internal.Commit)

			latest, err := internal.CheckLatestVersion()
			if err == nil && latest != "" && latest != "v"+internal.Version && latest != internal.Version {
				fmt.Printf("\nNew version available: %s (current: %s)\n", latest, internal.Version)
				fmt.Println("Run 'cpm upgrade' to update.")
			}
		},
	}
}

func upgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade cpm to the latest version from GitHub Releases",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := internal.LoadConfig(configPath)
			if err != nil {
				// Fallback to default bin dir
				home, _ := os.UserHomeDir()
				return internal.Upgrade(filepath.Join(home, ".local", "bin"))
			}
			return internal.Upgrade(cfg.BinDir)
		},
	}
}

func sortedKeys(m map[string]*internal.Profile) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
