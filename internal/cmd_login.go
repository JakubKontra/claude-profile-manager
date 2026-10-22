package internal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

// ErrLoginFailed is returned after the summary when a login did not succeed.
var ErrLoginFailed = errors.New("some logins failed")

// externalAuthVars are environment variables that make Claude Code use an
// API key, a long-lived token or a cloud provider instead of an OAuth login.
var externalAuthVars = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"CLAUDE_CODE_OAUTH_TOKEN",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
	"CLAUDE_CODE_USE_FOUNDRY",
}

// externalAuth returns the variable through which the profile authenticates
// without a login, or "" when it needs one.
func externalAuth(p *Profile) string {
	for _, key := range externalAuthVars {
		for _, env := range []map[string]string{p.LocalEnv, p.Env} {
			if v, ok := env[key]; ok && v != "" && v != "0" && v != "false" {
				return key
			}
		}
	}
	return ""
}

// LoginResult is one row of the `cpm login` summary.
type LoginResult struct {
	Profile string
	// Status is "logged in", "already logged in", "skipped" or "failed".
	Status string
	Detail string
}

// loginSkip returns why --all leaves a profile alone, or nil when it has no
// usable credentials.
func (a *App) loginSkip(name string, p *Profile) *LoginResult {
	if v := externalAuth(p); v != "" {
		return &LoginResult{name, "skipped", "uses " + v}
	}
	status, err := GetCredentialStatus(a.Keychain, filepath.Join(a.BaseDir, name))
	if err != nil || status.Expired {
		return nil
	}
	return &LoginResult{name, "already logged in", status.Account}
}

// Login runs `claude auth login` in each named profile, or with all in every
// profile without usable credentials, one at a time, then prints a summary.
func (a *App) Login(cfg *Config, names []string, all bool) error {
	if all == (len(names) > 0) {
		return fmt.Errorf("name the profiles to log in, or pass --all")
	}
	var results []LoginResult
	var targets []string
	if all {
		for _, name := range SortedProfileNames(cfg) {
			if skip := a.loginSkip(name, cfg.Profiles[name]); skip != nil {
				results = append(results, *skip)
			} else {
				targets = append(targets, name)
			}
		}
	} else {
		for _, name := range names {
			if _, err := lookupProfile(cfg, name); err != nil {
				return err
			}
			if !slices.Contains(targets, name) {
				targets = append(targets, name)
			}
		}
	}

	failed := false
	for i, name := range targets {
		result := LoginResult{Profile: name}
		dir := filepath.Join(a.BaseDir, name)
		if _, err := os.Stat(dir); err != nil {
			result.Status, result.Detail = "failed", "not installed (run cpm install)"
		} else if spec, err := a.BuildRunExec(cfg, name, []string{"auth", "login"}); err != nil {
			return err
		} else {
			a.outf("==> Logging in %s (%d/%d)\n", name, i+1, len(targets))
			if err := a.attach(spec); err != nil {
				result.Status, result.Detail = "failed", err.Error()
			} else if status, err := GetCredentialStatus(a.Keychain, dir); err != nil {
				result.Status, result.Detail = "failed", "no credentials after login"
			} else {
				result.Status, result.Detail = "logged in", status.Account
			}
		}
		failed = failed || result.Status == "failed"
		results = append(results, result)
	}

	a.renderLoginSummary(results)
	if failed {
		return ErrLoginFailed
	}
	return nil
}

func (a *App) renderLoginSummary(results []LoginResult) {
	if len(results) == 0 {
		a.outln("No profiles to log in.")
		return
	}
	a.outln("Summary:")
	for _, r := range results {
		line := r.Status
		if r.Detail != "" {
			line += " (" + r.Detail + ")"
		}
		a.outf("  %-20s %s\n", r.Profile, line)
	}
}

// attach runs spec with the app's stdio and waits for it.
func (a *App) attach(spec ExecSpec) error {
	if a.Attach != nil {
		return a.Attach(spec)
	}
	cmd := exec.Command(spec.Path, spec.Argv[1:]...)
	cmd.Env = spec.Env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = a.In, a.Out, a.Err
	return cmd.Run()
}
