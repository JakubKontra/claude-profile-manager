package internal

import (
	"fmt"
	"os"
	"strings"
)

// AddOptions holds the flags of `cpm add`.
type AddOptions struct {
	ShareProjects *bool
	InheritMCP    *bool
	Description   string
	Model         string
	AddDirs       []string
	Env           map[string]string
	// NoInstall skips running the install pipeline afterwards.
	NoInstall bool
}

// AddProfile appends a profile to config.toml and installs it.
func (a *App) AddProfile(name string, opts AddOptions) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	for k := range opts.Env {
		if !isValidEnvName(k) {
			return fmt.Errorf("invalid environment variable name %q", k)
		}
	}

	path := ExpandPath(a.ConfigPath)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read config: %w", err)
	}

	profile := &Profile{
		Description: opts.Description,
		Model:       opts.Model,
		AddDirs:     opts.AddDirs,
		Env:         opts.Env,
		InheritMCP:  opts.InheritMCP,
	}
	if opts.ShareProjects != nil && !*opts.ShareProjects {
		profile.Isolate = []string{"projects"}
	}
	updated, err := AppendProfileTable(data, name, profile)
	if err != nil {
		return err
	}
	if err := writeConfigAtomic(path, updated); err != nil {
		return err
	}
	a.outf("Added [profiles.%s] to %s\n", name, path)

	if opts.NoInstall {
		a.outln("Run 'cpm install' to create the profile directory and wrapper script.")
		return nil
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if err := a.InstallProfiles(cfg, InstallOptions{}); err != nil {
		return err
	}
	a.outf("\nAuthenticate with: claude-%s\n", name)
	return nil
}

// ParseEnvFlag turns repeated KEY=VALUE flags into a map.
func ParseEnvFlag(kv []string) (map[string]string, error) {
	if len(kv) == 0 {
		return nil, nil
	}
	env := make(map[string]string, len(kv))
	for _, item := range kv {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --env value %q (expected KEY=VALUE)", item)
		}
		if !isValidEnvName(key) {
			return nil, fmt.Errorf("invalid environment variable name %q", key)
		}
		env[key] = value
	}
	return env, nil
}
