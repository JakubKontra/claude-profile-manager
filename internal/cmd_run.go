package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExecSpec is a fully resolved command for the caller to exec.
type ExecSpec struct {
	Path string
	Argv []string
	Env  []string
}

// BuildRunExec resolves `cpm run <profile> [claude args...]` into a command.
func BuildRunExec(cfg *Config, profilesBase, name string, userArgs []string) (ExecSpec, error) {
	profile, err := lookupProfile(cfg, name)
	if err != nil {
		return ExecSpec{}, err
	}
	profileDir := filepath.Join(profilesBase, name)

	env := profileEnviron(os.Environ(), name, profileDir, profile)

	argv := []string{"claude"}
	for _, d := range profile.AddDirs {
		argv = append(argv, "--add-dir", ExpandPath(d))
	}
	if profile.Model != "" && !hasModelArg(userArgs) {
		argv = append(argv, "--model", profile.Model)
	}
	argv = append(argv, userArgs...)

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return ExecSpec{}, fmt.Errorf("claude not found on PATH")
	}

	return ExecSpec{Path: claudePath, Argv: argv, Env: env}, nil
}

// profileEnviron returns base with every CLAUDE_*/ANTHROPIC_* variable removed
// and the profile's variables appended.
func profileEnviron(base []string, name, profileDir string, profile *Profile) []string {
	filtered := make([]string, 0, len(base)+2+len(profile.Env))
	for _, e := range base {
		if strings.HasPrefix(e, "CLAUDE_") || strings.HasPrefix(e, "ANTHROPIC_") {
			continue
		}
		filtered = append(filtered, e)
	}
	filtered = append(filtered,
		"CLAUDE_CONFIG_DIR="+profileDir,
		"CLAUDE_PROFILE="+name,
	)
	for _, k := range sortedEnvKeys(profile.Env) {
		filtered = append(filtered, k+"="+profile.Env[k])
	}
	return filtered
}

func hasModelArg(args []string) bool {
	for _, a := range args {
		if a == "--model" || strings.HasPrefix(a, "--model=") {
			return true
		}
	}
	return false
}
