package internal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RemoveOptions holds the flags of `cpm remove`.
type RemoveOptions struct {
	// Purge also deletes the profile directory and its credentials.
	Purge bool
	// Yes skips the confirmation prompt.
	Yes bool
	// DryRun prints what would be removed and changes nothing.
	DryRun bool
	// Stdin answers the prompt; IsTerminal says whether prompting is possible.
	Stdin      io.Reader
	IsTerminal bool
}

// RemoveProfile removes a profile's wrapper and config entry, and with Purge
// its directory, .credentials.json and macOS Keychain entry.
func (a *App) RemoveProfile(cfg *Config, name string, opts RemoveOptions) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	_, configured := cfg.Profiles[name]
	if name == "cloud" {
		return fmt.Errorf("refusing to remove reserved cloud directory")
	}
	if !configured && !opts.Purge {
		return unknownProfileError(cfg, name)
	}

	profilesBase := a.BaseDir
	profileDir := filepath.Join(profilesBase, name)
	if filepath.Dir(profileDir) != profilesBase || filepath.Base(profileDir) != name {
		return fmt.Errorf("refusing to remove %s: outside the profiles directory", profileDir)
	}
	if st, err := os.Lstat(profileDir); err == nil && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("refusing to purge non-directory or symlink %s", profileDir)
	}
	if !configured && !isProfileDirectory(profileDir) {
		return fmt.Errorf("%s is not a recognized orphan profile directory", profileDir)
	}
	wrapperPath := filepath.Join(cfg.BinDir, wrapperPrefix+name)

	// What goes away; with Purge the same list is shown before asking.
	var items []string
	if isGeneratedWrapper(wrapperPath) {
		items = append(items, wrapperPath+" (wrapper script)")
	}
	if opts.Purge {
		items = append(items,
			profileDir+" (profile directory, sessions, history)",
			fmt.Sprintf("Keychain entry %q / .credentials.json", KeychainServiceName(profileDir)),
			fmt.Sprintf("[profiles.%s] in %s", name, filepath.Join(profilesBase, "secrets.toml")))
	}
	if configured {
		items = append(items, fmt.Sprintf("[profiles.%s] in %s", name, a.expand(a.ConfigPath)))
	}
	if opts.DryRun {
		a.outf("Dry run — remove profile %q would delete:\n", name)
		for _, item := range items {
			a.outf("  %s\n", item)
		}
		return nil
	}

	if opts.Purge && !opts.Yes {
		if !opts.IsTerminal {
			return fmt.Errorf("refusing to purge non-interactively without --yes")
		}
		a.outf("This will permanently delete:\n")
		for _, item := range items {
			a.outf("  %s\n", item)
		}
		a.outf("\nDelete profile %q and all its data? [y/N] ", name)
		if !confirm(opts.Stdin) {
			a.outln("Aborted.")
			return nil
		}
	}

	if isGeneratedWrapper(wrapperPath) {
		if err := os.Remove(wrapperPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove wrapper: %w", err)
		}
		a.outf("  removed %s\n", wrapperPath)
	}

	if opts.Purge {
		secretPath := filepath.Join(profilesBase, "secrets.toml")
		if data, err := os.ReadFile(secretPath); err == nil {
			updated, removed, err := RemoveProfileTable(data, name)
			if err != nil {
				return err
			}
			if removed {
				if err := writeFileAtomic(secretPath, updated, 0o600); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := DeleteProfileCredentials(a.Keychain, profileDir); err != nil {
			return fmt.Errorf("cannot remove credentials: %w", err)
		}
		a.outln("  removed credentials")
		if err := os.RemoveAll(profileDir); err != nil {
			return fmt.Errorf("cannot remove profile directory: %w", err)
		}
		a.outf("  removed %s\n", profileDir)
	}

	path := a.expand(a.ConfigPath)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read config: %w", err)
	}
	updated, removed, err := RemoveProfileTable(data, name)
	if err != nil {
		return err
	}
	if removed {
		if err := writeConfigAtomic(path, updated); err != nil {
			return err
		}
		a.outf("  removed [profiles.%s] from %s\n", name, path)
		if err := a.recordChange(cfg, ProfileChange{Kind: "removed", Name: name}); err != nil {
			fmt.Fprintf(a.Err, "Warning: cannot record the removal for cloud sync: %v\n", err)
		}
	}

	if !opts.Purge {
		a.outf("\nProfile %q removed from config. Its data is still in %s\n", name, profileDir)
		a.outf("Use 'cpm remove %s --purge' to delete the directory and credentials too.\n", name)
	} else {
		a.outf("\nProfile %q removed.\n", name)
	}
	if configured && len(cfg.Profiles) == 1 {
		a.outln("Note: this was the last profile; add one with 'cpm add <name>'.")
	}
	return nil
}

func confirm(r io.Reader) bool {
	if r == nil {
		return false
	}
	line, _ := bufio.NewReader(r).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func isProfileDirectory(path string) bool {
	st, err := os.Lstat(path)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return false
	}
	for _, marker := range []string{".cpm/profile", ".cpm/baseline", ".claude.json", ".credentials.json", "settings.json"} {
		if fileExistsAt(filepath.Join(path, marker)) {
			return true
		}
	}
	return false
}

func orphanProfiles(cfg *Config, base string) []string {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if name == "cloud" || strings.HasPrefix(name, ".") || cfg.Profiles[name] != nil || ValidateProfileName(name) != nil {
			continue
		}
		if isProfileDirectory(filepath.Join(base, name)) {
			names = append(names, name)
		}
	}
	return names
}
