package internal

import (
	"fmt"
	"sort"
	"strings"
)

// SortedProfileNames returns the profile names in deterministic order.
func SortedProfileNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// lookupProfile resolves a profile by name or returns an error listing the
// available ones.
func lookupProfile(cfg *Config, name string) (*Profile, error) {
	profile, ok := cfg.Profiles[name]
	if !ok {
		return nil, unknownProfileError(cfg, name)
	}
	return profile, nil
}

func unknownProfileError(cfg *Config, name string) error {
	available := strings.Join(SortedProfileNames(cfg), ", ")
	return fmt.Errorf("unknown profile %q (available: %s)", name, available)
}

// sortedEnvKeys returns the keys of an env map in deterministic order.
func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
