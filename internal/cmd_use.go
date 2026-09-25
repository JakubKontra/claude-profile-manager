package internal

import (
	"fmt"
	"path/filepath"
)

// Use resolves `cpm use <profile|auto>` and returns the shell snippet to eval.
func Use(cfg *Config, profilesBase, name, cwd string) (string, error) {
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
	return GenerateUseOutput(name, profileDir, profile), nil
}

// Direnv returns the .envrc snippet for a profile.
func Direnv(cfg *Config, profilesBase, name string) (string, error) {
	profile, err := lookupProfile(cfg, name)
	if err != nil {
		return "", err
	}
	return GenerateDirenvSnippet(name, filepath.Join(profilesBase, name), profile), nil
}
