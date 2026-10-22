package internal

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// RenameReport is the result of `cpm rename`: the steps done, or with
// DryRun the steps that would be done.
type RenameReport struct {
	Old    string `json:"old"`
	New    string `json:"new"`
	DryRun bool   `json:"dry_run"`
	Steps  []Step `json:"steps"`
}

// RenameProfile renames profile old to new: the config and secrets.toml
// tables, [[match]] rules, the profile directory, the macOS Keychain login
// (whose service name derives from the directory) and the wrapper. When a
// step fails, the steps already done are reverted in reverse order. The old
// Keychain entry is deleted only after everything else succeeded.
func (a *App) RenameProfile(cfg *Config, old, new string, dryRun bool) (RenameReport, error) {
	report := RenameReport{Old: old, New: new, DryRun: dryRun, Steps: []Step{}}
	steps, finish, err := a.planRename(cfg, old, new)
	if err != nil {
		return report, err
	}

	if dryRun {
		a.progressf("Dry run — rename profile %q to %q:\n", old, new)
	} else {
		a.progressf("Renaming profile %q to %q:\n", old, new)
	}
	report.Steps, err = a.runSteps(steps, finish, dryRun)
	if err != nil {
		return report, err
	}

	if dryRun {
		return report, nil
	}
	if err := a.recordChange(cfg, ProfileChange{Kind: "renamed", From: old, To: new}); err != nil {
		fmt.Fprintf(a.Err, "Warning: cannot record the rename for cloud sync: %v\n", err)
	}
	a.progressf("\nProfile %q is now %q.\n", old, new)
	if a.Getenv("CLAUDE_PROFILE") == old {
		a.progressf("This shell still uses %q; run: eval \"$(cpm use %s)\"\n", old, new)
	}
	a.progressf("Update .claude-profile files that contain %q by hand (cpm link %s).\n", old, new)
	return report, nil
}

// planRename validates the rename and returns its steps, plus the deletion
// of the old Keychain login that runs only after every step succeeded.
func (a *App) planRename(cfg *Config, old, new string) ([]undoStep, *undoStep, error) {
	profile, err := lookupProfile(cfg, old)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateProfileName(new); err != nil {
		return nil, nil, err
	}
	if new == "cloud" {
		return nil, nil, fmt.Errorf("profile name %q is reserved", new)
	}
	if _, exists := cfg.Profiles[new]; exists {
		return nil, nil, fmt.Errorf("profile %q already exists", new)
	}
	oldDir, newDir := filepath.Join(a.BaseDir, old), filepath.Join(a.BaseDir, new)
	if _, err := os.Lstat(newDir); err == nil {
		return nil, nil, fmt.Errorf("%s already exists", newDir)
	}
	oldWrapper, newWrapper := filepath.Join(cfg.BinDir, wrapperPrefix+old), filepath.Join(cfg.BinDir, wrapperPrefix+new)
	if _, err := os.Lstat(newWrapper); err == nil {
		return nil, nil, fmt.Errorf("%s already exists", newWrapper)
	}
	installed := false
	if st, err := os.Lstat(oldDir); err == nil {
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("invalid profile directory %s", oldDir)
		}
		installed = true
	}

	var steps []undoStep
	account := keychainAccountFor(a.Getenv("USER"))
	oldService, newService := KeychainServiceName(oldDir), KeychainServiceName(newDir)
	moveLogin := installed && a.Keychain.Exists(oldService, account)
	if moveLogin {
		if a.Keychain.Exists(newService, account) {
			return nil, nil, fmt.Errorf("the Keychain already has an entry %q for %s; delete it first (security delete-generic-password -s %q)", newService, newDir, newService)
		}
		steps = append(steps, undoStep{
			Step: Step{"keychain", fmt.Sprintf("copy login %q -> %q", oldService, newService)},
			do: func() error {
				secret, err := a.Keychain.Read(oldService, account)
				if err != nil {
					return fmt.Errorf("cannot read the Keychain login: %w", err)
				}
				if err := a.Keychain.Write(newService, account, secret); err != nil {
					return err
				}
				if got, err := a.Keychain.Read(newService, account); err != nil || !bytes.Equal(got, secret) {
					return fmt.Errorf("the copied Keychain login does not match")
				}
				return nil
			},
			undo: func() error { return a.Keychain.Delete(newService, account) },
		})
	}

	if installed {
		marker := filepath.Join(newDir, ".cpm", "profile")
		steps = append(steps, undoStep{
			Step: Step{"move", fmt.Sprintf("%s -> %s", oldDir, newDir)},
			do: func() error {
				if err := os.Rename(oldDir, newDir); err != nil {
					return err
				}
				return writeFileAtomic(marker, []byte(new+"\n"), 0o600)
			},
			undo: func() error {
				if err := writeFileAtomic(marker, []byte(old+"\n"), 0o600); err != nil {
					return err
				}
				return os.Rename(newDir, oldDir)
			},
		})
	}

	configPath := a.expand(a.ConfigPath)
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read config: %w", err)
	}
	renamed, _, err := RenameProfileTable(configData, old, new)
	if err != nil {
		return nil, nil, err
	}
	renamed, rules, err := RenameMatchProfile(renamed, old, new)
	if err != nil {
		return nil, nil, err
	}
	desc := fmt.Sprintf("rename [profiles.%s] in %s", old, configPath)
	if rules > 0 {
		desc += fmt.Sprintf(" and %d [[match]] rule%s", rules, pluralS(rules))
	}
	steps = append(steps, undoStep{
		Step: Step{"config", desc},
		do:   func() error { return writeConfigAtomic(configPath, renamed) },
		undo: func() error { return writeConfigAtomic(configPath, configData) },
	})

	secretsPath := filepath.Join(a.BaseDir, "secrets.toml")
	if secretsData, err := os.ReadFile(secretsPath); err == nil {
		updated, changed, err := RenameProfileTable(secretsData, old, new)
		if err != nil {
			return nil, nil, fmt.Errorf("secrets.toml: %w", err)
		}
		if changed {
			steps = append(steps, undoStep{
				Step: Step{"secrets", fmt.Sprintf("rename [profiles.%s] in %s", old, secretsPath)},
				do:   func() error { return writeFileAtomic(secretsPath, updated, 0o600) },
				undo: func() error { return writeFileAtomic(secretsPath, secretsData, 0o600) },
			})
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}

	oldWrapperData, wrapperErr := os.ReadFile(oldWrapper)
	ownWrapper := wrapperErr == nil && isGeneratedWrapper(oldWrapper)
	if ownWrapper || installed {
		wrapper := GenerateWrapper(new, newDir, profile)
		steps = append(steps, undoStep{
			Step: Step{"wrapper", fmt.Sprintf("%s -> %s", oldWrapper, newWrapper)},
			do: func() error {
				if err := writeFileAtomic(newWrapper, []byte(wrapper), wrapperMode(wrapper)); err != nil {
					return err
				}
				if ownWrapper {
					return os.Remove(oldWrapper)
				}
				return nil
			},
			undo: func() error {
				if ownWrapper {
					if err := writeFileAtomic(oldWrapper, oldWrapperData, wrapperMode(string(oldWrapperData))); err != nil {
						return err
					}
				}
				return os.Remove(newWrapper)
			},
		})
	}

	var finish *undoStep
	if moveLogin {
		finish = &undoStep{
			Step: Step{"keychain", fmt.Sprintf("delete old login %q", oldService)},
			do:   func() error { return a.Keychain.Delete(oldService, account) },
		}
	}
	return steps, finish, nil
}
