package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var copyFiles = []string{
	"settings.json",
	"settings.local.json",
	"CLAUDE.md",
}

var symlinkDirs = []string{
	"commands",
	"skills",
	"agents",
	"plugins",
	"projects",
}

func SetupProfile(name string, profileDir, sourceDir string, forceSync bool) error {
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
		action := "copied"
		if forceSync {
			action = "synced"
		}
		fmt.Printf("  %s %s\n", action, filename)
	}

	for _, dirname := range symlinkDirs {
		src := filepath.Join(sourceDir, dirname)
		dst := filepath.Join(profileDir, dirname)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		linkTarget, err := os.Readlink(dst)
		if err == nil {
			// Existing symlink — check if target matches
			absSrc, _ := filepath.Abs(src)
			absLink, _ := filepath.Abs(linkTarget)
			if absSrc == absLink {
				continue
			}
			os.Remove(dst)
		} else if _, err := os.Stat(dst); err == nil {
			// Real directory exists, don't replace
			fmt.Printf("  skipped %s/ (real directory exists)\n", dirname)
			continue
		}

		if err := os.Symlink(src, dst); err != nil {
			return fmt.Errorf("cannot symlink %s: %w", dirname, err)
		}
		fmt.Printf("  symlinked %s/ -> %s\n", dirname, src)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
