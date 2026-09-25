package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func CloneProfile(sourceName, targetName, profilesBase, sourceDir string, cfg *Config) error {
	if _, err := lookupProfile(cfg, sourceName); err != nil {
		return fmt.Errorf("unknown source profile %q", sourceName)
	}
	if err := ValidateProfileName(targetName); err != nil {
		return err
	}
	if _, exists := cfg.Profiles[targetName]; exists {
		return fmt.Errorf("target profile %q already exists in config", targetName)
	}

	srcDir := filepath.Join(profilesBase, sourceName)
	dstDir := filepath.Join(profilesBase, targetName)

	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return fmt.Errorf("source profile %q not installed (run cpm install first)", sourceName)
	}

	if _, err := os.Stat(dstDir); err == nil {
		return fmt.Errorf("target profile %q already exists", targetName)
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("cannot create target directory: %w", err)
	}

	// Copy mutable files from the source profile (not from ~/.claude)
	for _, filename := range copyFiles {
		src := filepath.Join(srcDir, filename)
		dst := filepath.Join(dstDir, filename)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		if err := cloneCopyFile(src, dst); err != nil {
			return fmt.Errorf("cannot copy %s: %w", filename, err)
		}
		outf("  copied %s\n", filename)
	}

	// Re-create symlinks pointing to the original source dir
	for _, dirname := range symlinkDirs {
		target := filepath.Join(sourceDir, dirname)
		link := filepath.Join(dstDir, dirname)

		if _, err := os.Stat(target); os.IsNotExist(err) {
			continue
		}

		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("cannot symlink %s: %w", dirname, err)
		}
		outf("  symlinked %s/ -> %s\n", dirname, target)
	}

	outf("\nProfile %q cloned from %q.\n", targetName, sourceName)
	outln("Note: credentials are NOT cloned — authenticate with: claude-" + targetName)
	outln("\nAdd the new profile to your config.toml:")
	outf("\n  [profiles.%s]\n  description = \"\"\n\n", targetName)
	outln("Then run 'cpm install' to generate the wrapper script.")

	return nil
}

func cloneCopyFile(src, dst string) error {
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
