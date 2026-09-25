package internal

import (
	"errors"
	"fmt"
	"path/filepath"
)

// InstallOptions controls InstallProfiles.
type InstallOptions struct {
	// Sync re-copies mutable files from the source directory.
	Sync bool
	// Force overwrites diverged profile files when syncing.
	Force bool
	// skipCloud disables auto_pull_on_install / auto_push (used when the
	// install runs as part of a cloud operation).
	skipCloud bool
}

// ErrDiverged is returned by InstallProfiles when --sync would overwrite
// profile files that carry local changes.
var ErrDiverged = errors.New("diverged profile files detected; merge changes back to source first, or use --sync --force")

// InstallProfiles creates every profile directory, patches its settings and
// installs the claude-<name> wrapper scripts. It is the single install
// pipeline every other command builds on.
func InstallProfiles(cfg *Config, configPath string, opts InstallOptions) error {
	profilesBase := ProfilesBaseDir(configPath)

	cloudEnabled := !opts.skipCloud && cfg.Cloud != nil && fileExistsAt(filepath.Join(CloudRepoDir(configPath), ".git"))
	if cloudEnabled && cfg.Cloud.AutoPullOnInstall {
		outln("Cloud: pulling (auto_pull_on_install)")
		if err := cloudPullFiles(configPath); err != nil {
			return fmt.Errorf("auto pull: %w", err)
		}
		// Pulled files may include settings; re-read the config too.
		if fresh, err := LoadConfig(configPath); err == nil {
			cfg = fresh
		}
		outln()
	}

	if opts.Sync && !opts.Force {
		diverged := CheckDivergence(cfg, profilesBase)
		if len(diverged) > 0 {
			out("\nDiverged profile files detected:\n\n")
			for _, d := range diverged {
				outf("  %s\n", d.Details)
			}
			outln()
			return ErrDiverged
		}
	}

	activeNames := make(map[string]bool)
	claudeJSONPath := DefaultClaudeJSONPath()

	for _, name := range SortedProfileNames(cfg) {
		profile := cfg.Profiles[name]
		profileDir := filepath.Join(profilesBase, name)
		scriptName := wrapperPrefix + name
		activeNames[scriptName] = true

		outf("\nProfile: %s\n", name)

		if err := SetupProfile(name, profileDir, cfg.SourceDir, EffectiveShareDirs(cfg, profile), opts.Sync); err != nil {
			return fmt.Errorf("profile %s: %w", name, err)
		}
		if err := PatchSettings(profileDir, EffectiveSettingsOverrides(profile)); err != nil {
			return fmt.Errorf("profile %s settings: %w", name, err)
		}
		if _, err := SeedClaudeJSON(profileDir, claudeJSONPath); err != nil {
			return fmt.Errorf("profile %s seed: %w", name, err)
		}
		if err := SyncMCPServers(profileDir, profile, claudeJSONPath); err != nil {
			return fmt.Errorf("profile %s mcp sync: %w", name, err)
		}

		wrapper := GenerateWrapper(name, profileDir, profile)
		scriptPath := filepath.Join(cfg.BinDir, scriptName)
		if err := InstallWrapper(scriptPath, wrapper); err != nil {
			return fmt.Errorf("profile %s wrapper: %w", name, err)
		}
	}

	outln("\nCleanup:")
	CleanupStaleScripts(cfg.BinDir, activeNames)

	if cloudEnabled && cfg.Cloud.AutoPush {
		outln("\nCloud: pushing (auto_push)")
		if err := CloudPush(configPath, PushOptions{}); err != nil {
			return fmt.Errorf("auto push: %w", err)
		}
	}

	outln("\nDone.")
	return nil
}
