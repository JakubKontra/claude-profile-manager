package internal

import (
	"strings"
)

// EnvVar is one environment variable a profile sets.
type EnvVar struct {
	Key   string
	Value string
}

// LaunchSpec is everything needed to start claude (or any command) inside a
// profile. Wrappers, `cpm use`, `cpm run`, `cpm exec` and direnv snippets are
// all rendered from it so they cannot drift apart.
type LaunchSpec struct {
	Name      string
	ConfigDir string
	// Env lists CLAUDE_CONFIG_DIR, CLAUDE_PROFILE and then the profile's own
	// variables in sorted order.
	Env     []EnvVar
	AddDirs []string
	Model   string
}

// bypassSubcommands are claude subcommands that must not receive --add-dir or
// --model; they only need the environment.
var bypassSubcommands = []string{
	"mcp", "auth", "doctor", "install", "setup-token", "update", "upgrade",
	"agents", "auto-mode", "plugin", "plugins",
}

// BuildLaunchSpec resolves a profile into a LaunchSpec.
func BuildLaunchSpec(name, profileDir string, p *Profile) LaunchSpec {
	spec := LaunchSpec{
		Name:      name,
		ConfigDir: profileDir,
		Model:     p.Model,
		Env: []EnvVar{
			{"CLAUDE_CONFIG_DIR", profileDir},
			{"CLAUDE_PROFILE", name},
		},
	}
	for _, k := range sortedEnvKeys(p.Env) {
		spec.Env = append(spec.Env, EnvVar{k, p.Env[k]})
	}
	for _, d := range p.AddDirs {
		spec.AddDirs = append(spec.AddDirs, ExpandPath(d))
	}
	return spec
}

// IsBypassSubcommand reports whether the first claude argument is a
// subcommand that should be exec'd without profile flags.
func IsBypassSubcommand(arg string) bool {
	for _, s := range bypassSubcommands {
		if arg == s {
			return true
		}
	}
	return false
}

// HasModelArg reports whether the user already passed --model.
func HasModelArg(args []string) bool {
	for _, a := range args {
		if a == "--model" || strings.HasPrefix(a, "--model=") {
			return true
		}
	}
	return false
}

// ClaudeArgs returns the profile flags to put before userArgs: nothing for
// bypass subcommands, otherwise --add-dir for each dir and --model unless the
// user passed one.
func (s LaunchSpec) ClaudeArgs(userArgs []string) []string {
	if len(userArgs) > 0 && IsBypassSubcommand(userArgs[0]) {
		return nil
	}
	var args []string
	for _, d := range s.AddDirs {
		args = append(args, "--add-dir", d)
	}
	if s.Model != "" && !HasModelArg(userArgs) {
		args = append(args, "--model", s.Model)
	}
	return args
}

// Environ returns base with every CLAUDE_*/ANTHROPIC_* variable removed and
// the spec's variables appended.
func (s LaunchSpec) Environ(base []string) []string {
	filtered := make([]string, 0, len(base)+len(s.Env))
	for _, e := range base {
		if isManagedEnvPrefix(e) {
			continue
		}
		filtered = append(filtered, e)
	}
	for _, v := range s.Env {
		filtered = append(filtered, v.Key+"="+v.Value)
	}
	return filtered
}

func isManagedEnvPrefix(kv string) bool {
	return strings.HasPrefix(kv, "CLAUDE_") || strings.HasPrefix(kv, "ANTHROPIC_")
}

// ShellQuote wraps s in single quotes so POSIX shells (and fish) take it
// literally; embedded single quotes become '\”.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
