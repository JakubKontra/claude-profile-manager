package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var copyFiles = []string{
	"settings.json",
	"settings.local.json",
	"CLAUDE.md",
}

// defaultShareDirs are symlinked from the source dir into every profile
// unless the config says otherwise.
var defaultShareDirs = []string{
	"commands",
	"skills",
	"agents",
	"plugins",
	"projects",
}

// EffectiveShareDirs returns the directories a profile shares with the
// source dir: the profile's share list, else the global one, else the
// default — minus the profile's isolate list.
func EffectiveShareDirs(cfg *Config, p *Profile) []string {
	base := defaultShareDirs
	if cfg != nil && cfg.Share != nil {
		base = cfg.Share
	}
	if p != nil && p.Share != nil {
		base = p.Share
	}
	isolated := map[string]bool{}
	if p != nil {
		for _, d := range p.Isolate {
			isolated[d] = true
		}
	}
	var dirs []string
	for _, d := range base {
		if !isolated[d] {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// SetupProfile creates the profile directory, copies mutable files and
// symlinks shareDirs from sourceDir. A directory that used to be shared but
// is no longer listed becomes an empty real directory; a real directory is
// never deleted.
func SetupProfile(name string, profileDir, sourceDir string, shareDirs []string, forceSync bool) error {
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return fmt.Errorf("cannot create profile dir: %w", err)
	}

	for _, filename := range copyFiles {
		src := filepath.Join(sourceDir, filename)
		dst := filepath.Join(profileDir, filename)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		if _, err := os.Stat(dst); err == nil && !forceSync {
			continue
		}

		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("cannot copy %s: %w", filename, err)
		}
		if err := copyFile(src, baselinePath(profileDir, filename)); err != nil {
			return fmt.Errorf("cannot record baseline for %s: %w", filename, err)
		}
		action := "copied"
		if forceSync {
			action = "synced"
		}
		outf("  %s %s\n", action, filename)
	}

	shared := map[string]bool{}
	for _, d := range shareDirs {
		shared[d] = true
	}

	// Symlinks into the source dir that are no longer in the share list get
	// replaced by an empty real directory.
	if entries, err := os.ReadDir(profileDir); err == nil {
		for _, entry := range entries {
			if shared[entry.Name()] || entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			link := filepath.Join(profileDir, entry.Name())
			target, err := os.Readlink(link)
			if err != nil {
				continue
			}
			absSource, _ := filepath.Abs(sourceDir)
			if !strings.HasPrefix(resolveLinkTarget(link, target), absSource+string(os.PathSeparator)) {
				continue
			}
			if err := os.Remove(link); err != nil {
				return fmt.Errorf("cannot remove symlink %s: %w", entry.Name(), err)
			}
			if err := os.MkdirAll(link, 0o755); err != nil {
				return fmt.Errorf("cannot create %s: %w", entry.Name(), err)
			}
			outf("  isolated %s/ (was shared)\n", entry.Name())
		}
	}

	for _, dirname := range shareDirs {
		src := filepath.Join(sourceDir, dirname)
		dst := filepath.Join(profileDir, dirname)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		linkTarget, err := os.Readlink(dst)
		if err == nil {
			// Existing symlink — check if target matches
			absSrc, _ := filepath.Abs(src)
			if absSrc == resolveLinkTarget(dst, linkTarget) {
				continue
			}
			os.Remove(dst)
		} else if _, err := os.Stat(dst); err == nil {
			// Real directory exists, don't replace
			outf("  skipped %s/ (real directory exists)\n", dirname)
			continue
		}

		if err := os.Symlink(src, dst); err != nil {
			return fmt.Errorf("cannot symlink %s: %w", dirname, err)
		}
		outf("  symlinked %s/ -> %s\n", dirname, src)
	}

	return nil
}

// baselinePath is where cpm keeps the source version a profile file was
// copied from, so local edits can be told apart from upstream changes.
func baselinePath(profileDir, filename string) string {
	return filepath.Join(profileDir, ".cpm", "baseline", filename)
}

// resolveLinkTarget returns the absolute path a symlink at link points to;
// relative targets are resolved against the link's own directory.
func resolveLinkTarget(link, target string) string {
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return target
	}
	return abs
}

// copyFile copies src to dst, creating dst's parent directory as needed.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
