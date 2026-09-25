package internal

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

// Use resolves `cpm use <profile|auto>` and returns the shell snippet to eval.
func Use(cfg *Config, profilesBase, name, cwd string, opts UseOptions) (string, error) {
	if name == "auto" {
		if _, isProfile := cfg.Profiles[name]; !isProfile {
			detected, err := DetectProfileFile(cwd)
			if err != nil {
				return "", fmt.Errorf("no .claude-profile found in current or parent directories")
			}
			if _, ok := cfg.Profiles[detected]; !ok {
				return "", fmt.Errorf("profile %q from .claude-profile not found in config", detected)
			}
			name = detected
		}
	}

	profile, err := lookupProfile(cfg, name)
	if err != nil {
		return "", err
	}
	profileDir := filepath.Join(profilesBase, name)
	return GenerateUseOutput(name, profileDir, profile, opts), nil
}

// SelectProfileInteractive prints a numbered menu to prompt and reads the
// choice from in. The menu goes to prompt (stderr) so stdout stays eval-safe.
func SelectProfileInteractive(cfg *Config, in io.Reader, prompt io.Writer) (string, error) {
	names := SortedProfileNames(cfg)
	if len(names) == 0 {
		return "", fmt.Errorf("no profiles configured")
	}
	current := CurrentProfile()
	fmt.Fprintln(prompt, "Select a profile:")
	for i, name := range names {
		marker := "  "
		if name == current {
			marker = "* "
		}
		desc := cfg.Profiles[name].Description
		if desc != "" {
			desc = "  " + desc
		}
		fmt.Fprintf(prompt, "%s%d) %s%s\n", marker, i+1, name, desc)
	}
	fmt.Fprint(prompt, "Profile [1-"+strconv.Itoa(len(names))+" or name]: ")

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("no selection")
	}
	choice := strings.TrimSpace(line)
	if choice == "" {
		return "", fmt.Errorf("no selection")
	}
	if n, err := strconv.Atoi(choice); err == nil {
		if n < 1 || n > len(names) {
			return "", fmt.Errorf("invalid selection %d", n)
		}
		return names[n-1], nil
	}
	if _, ok := cfg.Profiles[choice]; !ok {
		return "", unknownProfileError(cfg, choice)
	}
	return choice, nil
}

// Direnv returns the .envrc snippet for a profile.
func Direnv(cfg *Config, profilesBase, name string) (string, error) {
	profile, err := lookupProfile(cfg, name)
	if err != nil {
		return "", err
	}
	return GenerateDirenvSnippet(name, filepath.Join(profilesBase, name), profile), nil
}
