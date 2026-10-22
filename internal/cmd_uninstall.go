package internal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

// UninstallOptions holds the flags of `cpm uninstall`.
type UninstallOptions struct {
	// Purge also deletes the config, every profile and its credentials.
	Purge  bool
	DryRun bool
	// Yes skips the confirmation prompt.
	Yes bool
	// Stdin answers the prompt; IsTerminal says whether prompting is possible.
	Stdin      io.Reader
	IsTerminal bool
}

// hookLine matches the lines the README tells users to add to their rc
// file: eval "$(cpm hook ...)" and fish's `cpm hook ... | source`.
var hookLine = regexp.MustCompile(`^\s*(eval\s+"\$\(cpm hook[^)]*\)"|cpm hook[^|]*\|\s*source)\s*$`)

// Uninstall removes what cpm put outside its own directory: the wrapper
// scripts and the hook lines in shell rc files. With Purge it also deletes
// every profile's credentials and the files cpm keeps next to config.toml.
// Other lines that mention cpm, and .claude-profile files, are left alone.
func (a *App) Uninstall(cfg *Config, opts UninstallOptions) error {
	var steps []undoStep // only do is used; uninstalling is not reverted
	add := func(kind, detail string, do func() error) {
		steps = append(steps, undoStep{Step: Step{kind, detail}, do: do})
	}

	if entries, err := os.ReadDir(cfg.BinDir); err == nil {
		for _, e := range entries {
			path := filepath.Join(cfg.BinDir, e.Name())
			if strings.HasPrefix(e.Name(), wrapperPrefix) && e.Type().IsRegular() && isGeneratedWrapper(path) {
				add("wrapper", "remove "+path, func() error { return os.Remove(path) })
			}
		}
	}

	var untouched []string
	for _, rc := range rcFiles {
		path := filepath.Join(a.Home, rc)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var kept, removed []string
		for _, line := range strings.SplitAfter(string(data), "\n") {
			switch {
			case hookLine.MatchString(strings.TrimRight(line, "\n")):
				removed = append(removed, strings.TrimSpace(line))
			case strings.Contains(line, "cpm hook"):
				untouched = append(untouched, fmt.Sprintf("%s: %s", path, strings.TrimSpace(line)))
				kept = append(kept, line)
			default:
				kept = append(kept, line)
			}
		}
		if len(removed) == 0 {
			continue
		}
		mode := os.FileMode(0o644)
		if st, err := os.Stat(path); err == nil {
			mode = st.Mode().Perm()
		}
		updated := strings.Join(kept, "")
		add("hook", fmt.Sprintf("remove %s from %s (backup: %s.cpm-uninstall.bak)", strings.Join(removed, ", "), path, rc), func() error {
			if err := writeFileAtomic(path+".cpm-uninstall.bak", data, mode); err != nil {
				return err
			}
			return writeFileAtomic(path, []byte(updated), mode)
		})
	}

	if opts.Purge {
		a.planPurgeAll(cfg, add)
	}

	if len(steps) == 0 && len(untouched) == 0 {
		a.outln("Nothing to uninstall.")
		return nil
	}
	header := "cpm uninstall will:"
	if opts.DryRun {
		header = "Dry run — cpm uninstall would:"
	}
	a.outln(header)
	for _, s := range steps {
		a.outf("  %s\n", s.Step)
	}
	for _, u := range untouched {
		a.outf("  leave   %s (not a line cpm recognizes; remove it by hand)\n", u)
	}
	if opts.DryRun {
		return nil
	}
	if !opts.Yes {
		if !opts.IsTerminal {
			return fmt.Errorf("refusing to uninstall non-interactively without --yes")
		}
		a.outf("\nProceed? [y/N] ")
		if !confirm(opts.Stdin) {
			a.outln("Aborted.")
			return nil
		}
	}

	for _, s := range steps {
		if err := s.do(); err != nil {
			return fmt.Errorf("%s: %w", s.Step, err)
		}
	}
	a.outln("\nUninstalled.")
	if !opts.Purge {
		a.outf("Profiles and their logins are kept in %s; 'cpm uninstall --purge' deletes them too.\n", a.BaseDir)
	}
	a.outln(".claude-profile files in your projects are left alone.")
	a.outln(binaryRemovalHint())
	return nil
}

// planPurgeAll adds the steps that delete every profile with its
// credentials and the files cpm keeps next to config.toml. The directory
// itself goes only when nothing else is left in it.
func (a *App) planPurgeAll(cfg *Config, add func(kind, detail string, do func() error)) {
	names := append(SortedProfileNames(cfg), orphanProfiles(cfg, a.BaseDir)...)
	for _, name := range names {
		dir := filepath.Join(a.BaseDir, name)
		add("login", fmt.Sprintf("delete the login of %s (Keychain %q / .credentials.json)", name, KeychainServiceName(dir)), func() error {
			return DeleteProfileCredentials(a.Keychain, dir)
		})
		if fileExistsAt(dir) {
			add("profile", "delete "+dir, func() error { return os.RemoveAll(dir) })
		}
	}
	for _, name := range []string{"secrets.toml", "cloud", ".cpm", filepath.Base(a.expand(a.ConfigPath))} {
		path := filepath.Join(a.BaseDir, name)
		if _, err := os.Lstat(path); err == nil {
			add("files", "delete "+path, func() error { return os.RemoveAll(path) })
		}
	}
	add("files", "delete "+a.BaseDir+" if empty", func() error {
		if err := os.Remove(a.BaseDir); err != nil && !os.IsNotExist(err) && !errors.Is(err, syscall.ENOTEMPTY) && !errors.Is(err, syscall.EEXIST) {
			return err
		}
		return nil
	})
}

// binaryRemovalHint says how to remove the cpm binary itself.
func binaryRemovalHint() string {
	exe, err := executablePath()
	if err != nil {
		return "Finally, remove the cpm binary."
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	switch {
	case isHomebrewPath(exe):
		return "Finally, remove the binary: brew uninstall cpm"
	case strings.HasPrefix(exe, "/nix/store/"):
		return "Finally, remove the binary: nix profile remove cpm (or drop it from your flake)"
	}
	return "Finally, remove the binary: rm " + exe
}
