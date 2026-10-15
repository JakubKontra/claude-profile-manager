package internal

import (
	"errors"
	"fmt"
	"path/filepath"
)

// InstallOptions controls InstallProfiles.
type InstallOptions struct {
	DryRun bool
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
	if opts.DryRun {
		plan, err := PlanInstall(cfg, configPath, opts)
		if err != nil {
			return err
		}
		RenderInstallPlan(plan, cfg)
		return nil
	}

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

	plan, err := PlanInstall(cfg, configPath, opts)
	if err != nil {
		return err
	}
	if err := ApplyInstall(plan); err != nil {
		return err
	}

	if cloudEnabled && cfg.Cloud.AutoPush {
		outln("\nCloud: pushing (auto_push)")
		if err := CloudPush(configPath, PushOptions{}); err != nil {
			return fmt.Errorf("auto push: %w", err)
		}
	}

	outln("\nDone.")
	return nil
}
