package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	spec := BuildLaunchSpec(name, filepath.Join(profilesBase, name), profile)

	argv := append([]string{"claude"}, spec.ClaudeArgs(userArgs)...)
	argv = append(argv, userArgs...)

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return ExecSpec{}, fmt.Errorf("claude not found on PATH")
	}

	return ExecSpec{Path: claudePath, Argv: argv, Env: spec.Environ(os.Environ())}, nil
}

// BuildExecExec resolves `cpm exec <profile> -- <command...>`: any command
// run with the profile's environment.
func BuildExecExec(cfg *Config, profilesBase, name string, argv []string) (ExecSpec, error) {
	if len(argv) == 0 {
		return ExecSpec{}, fmt.Errorf("no command given (usage: cpm exec <profile> -- <command...>)")
	}
	profile, err := lookupProfile(cfg, name)
	if err != nil {
		return ExecSpec{}, err
	}
	spec := BuildLaunchSpec(name, filepath.Join(profilesBase, name), profile)

	path, err := exec.LookPath(argv[0])
	if err != nil {
		return ExecSpec{}, fmt.Errorf("%s not found on PATH", argv[0])
	}
	return ExecSpec{Path: path, Argv: argv, Env: spec.Environ(os.Environ())}, nil
}
