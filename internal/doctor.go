package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok", "warn", "error"
	Detail string `json:"detail"`
}

// DoctorOptions tunes RunDoctor.
type DoctorOptions struct {
	// HomeDir is where shell rc files are looked up; empty means $HOME.
	HomeDir string
	// Verify reads the Keychain token (macOS) to check its expiry.
	Verify bool
	// LookPath is injected in tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

// rcFiles are the shell startup files scanned for the cpm hook.
var rcFiles = []string{".zshrc", ".bashrc", ".bash_profile", ".config/fish/config.fish"}

func RunDoctor(cfg *Config, profilesBase string) []Check {
	return RunDoctorWithOptions(cfg, profilesBase, DoctorOptions{})
}

func RunDoctorWithOptions(cfg *Config, profilesBase string, opts DoctorOptions) []Check {
	var checks []Check
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	home := opts.HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	// Check claude binary
	claudePath, err := lookPath("claude")
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

	// Unknown config keys are usually typos.
	if len(cfg.Undecoded) > 0 {
		checks = append(checks, Check{"config keys", "warn", "unknown keys (typo?): " + strings.Join(cfg.Undecoded, ", ")})
	}

	// git is needed once cloud sync is configured.
	if cfg.Cloud != nil {
		if gitPath, err := lookPath("git"); err != nil {
			checks = append(checks, Check{"git binary", "error", "git not found on PATH (needed for cpm cloud)"})
		} else {
			checks = append(checks, Check{"git binary", "ok", gitPath})
		}
	}

	// Shell hook installed?
	if rc := hookInstalledIn(home); rc != "" {
		checks = append(checks, Check{"shell hook", "ok", rc})
	} else {
		checks = append(checks, Check{"shell hook", "warn", "not found in shell rc files (add: eval \"$(cpm hook)\")"})
	}

	// Check each profile
	for _, name := range SortedProfileNames(cfg) {
		profileDir := filepath.Join(profilesBase, name)
		profile := cfg.Profiles[name]

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
		if err == nil && opts.Verify && status.Source == "keychain" {
			status = verifyKeychainToken(profileDir, status)
		}
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
		scriptPath := filepath.Join(cfg.BinDir, wrapperPrefix+name)
		wrapperCheck := fmt.Sprintf("profile/%s/wrapper", name)
		switch existing, err := os.ReadFile(scriptPath); {
		case os.IsNotExist(err):
			checks = append(checks, Check{wrapperCheck, "warn", "wrapper script missing (run cpm install)"})
		case err != nil:
			checks = append(checks, Check{wrapperCheck, "error", err.Error()})
		case string(existing) != GenerateWrapper(name, profileDir, profile):
			checks = append(checks, Check{wrapperCheck, "warn", "wrapper outdated (run cpm install)"})
		default:
			checks = append(checks, Check{wrapperCheck, "ok", scriptPath})
		}
	}

	return checks
}

// hookInstalledIn returns the rc file that references the cpm hook, or "".
func hookInstalledIn(home string) string {
	for _, rc := range rcFiles {
		path := filepath.Join(home, rc)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "cpm hook") {
			return path
		}
	}
	return ""
}

// verifyKeychainToken reads the token blob from the Keychain and fills in
// expiry and subscription. Failures leave the status as it was.
func verifyKeychainToken(profileDir string, status CredentialStatus) CredentialStatus {
	data, err := keychainRead(KeychainServiceName(profileDir), keychainAccount())
	if err != nil {
		return status
	}
	info, err := parseCredentialsJSON(data, time.Time{}, time.Now())
	if err != nil {
		return status
	}
	status.Expired = info.Expired
	status.ExpiresAt = info.ExpiresAt
	status.SubscriptionType = info.SubscriptionType
	if status.Account == "" {
		status.Account = info.Account
	}
	return status
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
	if s.SubscriptionType != "" {
		detail += " (" + s.SubscriptionType + ")"
	}
	if !s.ExpiresAt.IsZero() {
		detail += " valid until " + s.ExpiresAt.Local().Format("2006-01-02 15:04")
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
