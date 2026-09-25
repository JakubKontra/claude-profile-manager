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
}

// ErrDiverged is returned by InstallProfiles when --sync would overwrite
// profile files that carry local changes.
var ErrDiverged = errors.New("diverged profile files detected; merge changes back to source first, or use --sync --force")

// InstallProfiles creates every profile directory, patches its settings and
// installs the claude-<name> wrapper scripts. It is the single install
// pipeline every other command builds on.
func InstallProfiles(cfg *Config, configPath string, opts InstallOptions) error {
	profilesBase := ProfilesBaseDir(configPath)

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

		if err := SetupProfile(name, profileDir, cfg.SourceDir, opts.Sync); err != nil {
			return fmt.Errorf("profile %s: %w", name, err)
		}
		if err := PatchAttribution(profileDir, profile.Attribution); err != nil {
			return fmt.Errorf("profile %s attribution: %w", name, err)
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

	outln("\nDone.")
	return nil
}
