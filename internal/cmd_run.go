package internal

import (
	"fmt"
	"path/filepath"
)

// ExecSpec is a fully resolved command for the caller to exec.
type ExecSpec struct {
	Path string
	Argv []string
	Env  []string
}

// BuildRunExec resolves `cpm run <profile> [claude args...]` into a command.
func (a *App) BuildRunExec(cfg *Config, name string, userArgs []string) (ExecSpec, error) {
	userArgs = trimCommandSeparator(userArgs)
	profile, err := lookupProfile(cfg, name)
	if err != nil {
		return ExecSpec{}, err
	}
	spec := BuildLaunchSpec(name, filepath.Join(a.BaseDir, name), profile)

	argv := append([]string{"claude"}, spec.ClaudeArgs(userArgs)...)
	argv = append(argv, userArgs...)

	claudePath, err := a.lookPath("claude")
	if err != nil {
		return ExecSpec{}, fmt.Errorf("claude not found on PATH")
	}

	return ExecSpec{Path: claudePath, Argv: argv, Env: spec.Environ(a.Env)}, nil
}

// BuildExecExec resolves `cpm exec <profile> -- <command...>`: any command
// run with the profile's environment.
func (a *App) BuildExecExec(cfg *Config, name string, argv []string) (ExecSpec, error) {
	argv = trimCommandSeparator(argv)
	if len(argv) == 0 {
		return ExecSpec{}, fmt.Errorf("no command given (usage: cpm exec <profile> -- <command...>)")
	}
	profile, err := lookupProfile(cfg, name)
	if err != nil {
		return ExecSpec{}, err
	}
	spec := BuildLaunchSpec(name, filepath.Join(a.BaseDir, name), profile)

	path, err := a.lookPath(argv[0])
	if err != nil {
		return ExecSpec{}, fmt.Errorf("%s not found on PATH", argv[0])
	}
	return ExecSpec{Path: path, Argv: argv, Env: spec.Environ(a.Env)}, nil
}

func trimCommandSeparator(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}
