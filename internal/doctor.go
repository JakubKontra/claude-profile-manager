package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Check struct {
	Name   string
	Status string // "ok", "warn", "error"
	Detail string
}

func RunDoctor(cfg *Config, profilesBase string) []Check {
	var checks []Check

	// Check claude binary
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		checks = append(checks, Check{"claude binary", "error", "claude not found on PATH"})
	} else {
		checks = append(checks, Check{"claude binary", "ok", claudePath})
	}

	// Check source dir
	if _, err := os.Stat(cfg.SourceDir); os.IsNotExist(err) {
		checks = append(checks, Check{"source directory", "error", fmt.Sprintf("%s does not exist", cfg.SourceDir)})
	} else {
		checks = append(checks, Check{"source directory", "ok", cfg.SourceDir})
	}

	// Check bin dir
	if _, err := os.Stat(cfg.BinDir); os.IsNotExist(err) {
		checks = append(checks, Check{"bin directory", "warn", fmt.Sprintf("%s does not exist (will be created on install)", cfg.BinDir)})
	} else {
		checks = append(checks, Check{"bin directory", "ok", cfg.BinDir})
	}

	// Check bin dir is on PATH
	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	binOnPath := false
	for _, d := range pathDirs {
		if d == cfg.BinDir {
			binOnPath = true
			break
		}
	}
	if binOnPath {
		checks = append(checks, Check{"bin dir on PATH", "ok", cfg.BinDir})
	} else {
		checks = append(checks, Check{"bin dir on PATH", "warn", fmt.Sprintf("%s is not on PATH", cfg.BinDir)})
	}

	// Check each profile
	for _, name := range SortedProfileNames(cfg) {
		profileDir := filepath.Join(profilesBase, name)

		if _, err := os.Stat(profileDir); os.IsNotExist(err) {
			checks = append(checks, Check{fmt.Sprintf("profile/%s", name), "warn", "not installed (run cpm install)"})
			continue
		}

		// Check symlinks
		for _, dir := range EffectiveShareDirs(cfg, cfg.Profiles[name]) {
			link := filepath.Join(profileDir, dir)
			target, err := os.Readlink(link)
			if err != nil {
				continue // Not a symlink or doesn't exist
			}
			if _, err := os.Stat(resolveLinkTarget(link, target)); os.IsNotExist(err) {
				checks = append(checks, Check{fmt.Sprintf("profile/%s/%s", name, dir), "error", fmt.Sprintf("broken symlink -> %s", target)})
			}
		}

		// Check credentials (Keychain on macOS, .credentials.json elsewhere)
		checkName := fmt.Sprintf("profile/%s/credentials", name)
		status, err := GetCredentialStatus(profileDir)
		switch {
		case err != nil:
			checks = append(checks, Check{checkName, "warn", "not authenticated (run claude-" + name + " auth login)"})
		case status.Expired:
			checks = append(checks, Check{checkName, "warn", fmt.Sprintf("credentials expired (run claude-%s auth login)", name)})
		case status.Source == "file" && time.Since(status.LastUpdated) > 7*24*time.Hour:
			checks = append(checks, Check{checkName, "warn", describeCredentials(status)})
		default:
			checks = append(checks, Check{checkName, "ok", describeCredentials(status)})
		}

		// Check wrapper script
		scriptPath := filepath.Join(cfg.BinDir, "claude-"+name)
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			checks = append(checks, Check{fmt.Sprintf("profile/%s/wrapper", name), "warn", "wrapper script missing (run cpm install)"})
		} else {
			checks = append(checks, Check{fmt.Sprintf("profile/%s/wrapper", name), "ok", scriptPath})
		}
	}

	return checks
}

func PrintChecks(checks []Check) {
	for _, c := range checks {
		var icon string
		switch c.Status {
		case "ok":
			icon = "  OK"
		case "warn":
			icon = "WARN"
		case "error":
			icon = " ERR"
		}
		outf("  [%s] %-35s %s\n", icon, c.Name, c.Detail)
	}
}

// describeCredentials renders "<source> — <account> (updated 3h ago)", leaving
// out the parts a profile doesn't have yet.
func describeCredentials(s CredentialStatus) string {
	detail := s.Source
	if s.Account != "" {
		detail += " — " + s.Account
	}
	if !s.LastUpdated.IsZero() {
		detail += fmt.Sprintf(" (updated %s ago)", formatDuration(time.Since(s.LastUpdated)))
	}
	return detail
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
