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
	// Stdin answers the prompt; IsTerminal says whether prompting is possible.
	Stdin      io.Reader
	IsTerminal bool
}

// RemoveProfile removes a profile's wrapper and config entry, and with Purge
// its directory, .credentials.json and macOS Keychain entry.
func RemoveProfile(cfg *Config, configPath, name string, opts RemoveOptions) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return unknownProfileError(cfg, name)
	}

	profilesBase := ProfilesBaseDir(configPath)
	profileDir := filepath.Join(profilesBase, name)
	if filepath.Dir(profileDir) != profilesBase || filepath.Base(profileDir) != name {
		return fmt.Errorf("refusing to remove %s: outside the profiles directory", profileDir)
	}
	wrapperPath := filepath.Join(cfg.BinDir, wrapperPrefix+name)

	if opts.Purge && !opts.Yes {
		if !opts.IsTerminal {
			return fmt.Errorf("refusing to purge non-interactively without --yes")
		}
		outf("This will permanently delete:\n")
		outf("  %s (profile directory, sessions, history)\n", profileDir)
		outf("  %s (wrapper script)\n", wrapperPath)
		outf("  Keychain entry %q / .credentials.json\n", KeychainServiceName(profileDir))
		outf("  [profiles.%s] in %s\n\n", name, ExpandPath(configPath))
		outf("Delete profile %q and all its data? [y/N] ", name)
		if !confirm(opts.Stdin) {
			outln("Aborted.")
			return nil
		}
	}

	if isGeneratedWrapper(wrapperPath) {
		if err := os.Remove(wrapperPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove wrapper: %w", err)
		}
		outf("  removed %s\n", wrapperPath)
	}

	if opts.Purge {
		if err := DeleteProfileCredentials(profileDir); err != nil {
			return fmt.Errorf("cannot remove credentials: %w", err)
		}
		outln("  removed credentials")
		if err := os.RemoveAll(profileDir); err != nil {
			return fmt.Errorf("cannot remove profile directory: %w", err)
		}
		outf("  removed %s\n", profileDir)
	}

	path := ExpandPath(configPath)
	data, err := os.ReadFile(path)
	if err != nil {
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
		outf("  removed [profiles.%s] from %s\n", name, path)
	}

	if !opts.Purge {
		outf("\nProfile %q removed from config. Its data is still in %s\n", name, profileDir)
		outln("Use 'cpm remove --purge' to delete the directory and credentials too.")
	} else {
		outf("\nProfile %q removed.\n", name)
	}
	if len(cfg.Profiles) == 1 {
		outln("Note: this was the last profile; add one with 'cpm add <name>'.")
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
