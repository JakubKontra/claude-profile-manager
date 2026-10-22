package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AdoptOptions holds the flags of `cpm adopt`.
type AdoptOptions struct {
	// From is the Claude Code config dir to take the login from; "" means
	// the default ~/.claude.
	From string
	// Move deletes the login from the source after copying it.
	Move   bool
	DryRun bool
}

// AdoptReport is the result of `cpm adopt`.
type AdoptReport struct {
	Profile string `json:"profile"`
	From    string `json:"from"`
	DryRun  bool   `json:"dry_run"`
	Steps   []Step `json:"steps"`
}

// loginSource is where Claude Code keeps the login of a config dir.
type loginSource struct {
	dir string
	// service is the Keychain service: bare for ~/.claude, hashed otherwise.
	service string
	// claudeJSON holds oauthAccount: ~/.claude.json for ~/.claude, the
	// dir's own .claude.json otherwise.
	claudeJSON string
}

func (a *App) loginSourceFor(from string) loginSource {
	def := filepath.Join(a.Home, ".claude")
	if from == "" || filepath.Clean(a.expand(from)) == def {
		return loginSource{dir: def, service: keychainServiceBase, claudeJSON: filepath.Join(a.Home, ".claude.json")}
	}
	dir := filepath.Clean(a.expand(from))
	return loginSource{dir: dir, service: KeychainServiceName(dir), claudeJSON: filepath.Join(dir, ".claude.json")}
}

// AdoptLogin copies an existing Claude Code login (by default the one in
// ~/.claude) into profile name, so it needs no new login. A profile missing
// from the config is added first. With Move the source login is deleted at
// the end; a failure before that reverts every step.
func (a *App) AdoptLogin(cfg *Config, name string, opts AdoptOptions) (AdoptReport, error) {
	src := a.loginSourceFor(opts.From)
	report := AdoptReport{Profile: name, From: src.dir, DryRun: opts.DryRun, Steps: []Step{}}
	if err := ValidateProfileName(name); err != nil {
		return report, err
	}
	account := keychainAccountFor(a.Getenv("USER"))
	inKeychain := a.Keychain.Exists(src.service, account)
	srcFile := filepath.Join(src.dir, ".credentials.json")
	inFile := fileExistsAt(srcFile)
	if !inKeychain && !inFile {
		return report, fmt.Errorf("no Claude Code login found for %s (Keychain %q or %s)", src.dir, src.service, srcFile)
	}

	dir := filepath.Join(a.BaseDir, name)
	if _, configured := cfg.Profiles[name]; !configured {
		if opts.DryRun {
			a.progressf("Dry run — adopt the login of %s into new profile %q:\n", src.dir, name)
			report.Steps = append(report.Steps, Step{"config", fmt.Sprintf("add and install profile %q", name)})
			a.progressf("  %s\n", report.Steps[0])
			// The rest depends on the new profile; show what would be copied.
			return a.adoptPreview(report, src, dir, inKeychain, inFile, opts.Move), nil
		}
		if err := a.AddProfile(name, AddOptions{}); err != nil {
			return report, err
		}
	}
	if !fileExistsAt(dir) {
		return report, fmt.Errorf("profile %q is not installed (run cpm install)", name)
	}
	if status, err := GetCredentialStatus(a.Keychain, dir); err == nil {
		return report, fmt.Errorf("profile %q is already logged in (%s); log it out first: claude-%s auth logout", name, status.Account, name)
	}

	steps, finish, err := a.planAdopt(src, dir, account, inKeychain, inFile, opts.Move)
	if err != nil {
		return report, err
	}
	if opts.DryRun {
		a.progressf("Dry run — adopt the login of %s into profile %q:\n", src.dir, name)
	} else {
		a.progressf("Adopting the login of %s into profile %q:\n", src.dir, name)
	}
	done, err := a.runSteps(steps, finish, opts.DryRun)
	report.Steps = append(report.Steps, done...)
	if err != nil || opts.DryRun {
		return report, err
	}
	a.progressf("\nProfile %q now uses that login: claude-%s auth status\n", name, name)
	return report, nil
}

// adoptPreview lists the copy steps for a profile that does not exist yet.
func (a *App) adoptPreview(report AdoptReport, src loginSource, dir string, inKeychain, inFile, move bool) AdoptReport {
	steps, finish, _ := a.planAdopt(src, dir, "", inKeychain, inFile, move)
	for _, s := range steps {
		a.progressf("  %s\n", s.Step)
		report.Steps = append(report.Steps, s.Step)
	}
	if finish != nil {
		a.progressf("  %s\n", finish.Step)
		report.Steps = append(report.Steps, finish.Step)
	}
	return report
}

func (a *App) planAdopt(src loginSource, dir, account string, inKeychain, inFile, move bool) ([]undoStep, *undoStep, error) {
	var steps []undoStep
	dstService := KeychainServiceName(dir)
	if inKeychain {
		steps = append(steps, undoStep{
			Step: Step{"keychain", fmt.Sprintf("copy login %q -> %q", src.service, dstService)},
			do: func() error {
				secret, err := a.Keychain.Read(src.service, account)
				if err != nil {
					return fmt.Errorf("cannot read the Keychain login: %w", err)
				}
				if err := a.Keychain.Write(dstService, account, secret); err != nil {
					return err
				}
				if got, err := a.Keychain.Read(dstService, account); err != nil || !bytes.Equal(got, secret) {
					return fmt.Errorf("the copied Keychain login does not match")
				}
				return nil
			},
			undo: func() error { return a.Keychain.Delete(dstService, account) },
		})
	}
	srcFile, dstFile := filepath.Join(src.dir, ".credentials.json"), filepath.Join(dir, ".credentials.json")
	if inFile {
		steps = append(steps, undoStep{
			Step: Step{"file", fmt.Sprintf("copy %s -> %s", srcFile, dstFile)},
			do: func() error {
				data, err := os.ReadFile(srcFile)
				if err != nil {
					return err
				}
				return writeFileAtomic(dstFile, data, 0o600)
			},
			undo: func() error { return os.Remove(dstFile) },
		})
	}

	// The account shown by claude and cpm comes from oauthAccount.
	if oauth := readOAuthAccount(src.claudeJSON); oauth != nil {
		path := filepath.Join(dir, ".claude.json")
		before, readErr := os.ReadFile(path)
		steps = append(steps, undoStep{
			Step: Step{"account", fmt.Sprintf("copy oauthAccount from %s", src.claudeJSON)},
			do: func() error {
				current := map[string]any{}
				if readErr == nil {
					if err := json.Unmarshal(before, &current); err != nil || current == nil {
						return fmt.Errorf("invalid %s", path)
					}
				}
				current["oauthAccount"] = oauth
				data, err := json.MarshalIndent(current, "", "  ")
				if err != nil {
					return err
				}
				return writeFileAtomic(path, append(data, '\n'), 0o600)
			},
			undo: func() error {
				if readErr != nil {
					return os.Remove(path)
				}
				return writeFileAtomic(path, before, 0o600)
			},
		})
	}

	if !move {
		return steps, nil, nil
	}
	finish := &undoStep{
		Step: Step{"delete", "the login from " + src.dir},
		do: func() error {
			if inKeychain {
				if err := a.Keychain.Delete(src.service, account); err != nil {
					return err
				}
			}
			if inFile {
				return os.Remove(srcFile)
			}
			return nil
		},
	}
	return steps, finish, nil
}

// readOAuthAccount returns the oauthAccount object of a .claude.json, or nil.
func readOAuthAccount(path string) any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var parsed map[string]any
	if json.Unmarshal(data, &parsed) != nil {
		return nil
	}
	return parsed["oauthAccount"]
}
