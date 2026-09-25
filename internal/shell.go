package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// managedVarsName holds the names of the variables cpm set in this shell, so
// a later switch or unset only touches those and never the user's own
// CLAUDE_*/ANTHROPIC_* variables.
const managedVarsName = "CPM_MANAGED_VARS"

// UseOptions controls the shell snippet `cpm use` prints.
type UseOptions struct {
	// Shell is "bash", "zsh", "sh" (all POSIX) or "fish"; empty means POSIX.
	Shell string
	// Quiet suppresses the "Switched to profile" echo.
	Quiet bool
}

// IsFishShell reports whether the shell name refers to fish.
func IsFishShell(shell string) bool {
	return strings.TrimSpace(strings.ToLower(shell)) == "fish"
}

// DetectShell guesses the user's shell from $SHELL ("fish" or "posix").
func DetectShell(getenv func(string) string) string {
	if IsFishShell(filepath.Base(getenv("SHELL"))) {
		return "fish"
	}
	return "posix"
}

// GenerateUseOutput outputs shell commands to be eval'd by the user's shell.
// Usage: eval "$(cpm use <profile>)"  or  cpm use --shell fish work | source
func GenerateUseOutput(name string, profileDir string, profile *Profile, opts UseOptions) string {
	spec := BuildLaunchSpec(name, profileDir, profile)
	var b strings.Builder

	names := make([]string, 0, len(spec.Env))
	for _, v := range spec.Env {
		names = append(names, v.Key)
	}

	if IsFishShell(opts.Shell) {
		fmt.Fprintf(&b, "if set -q %s; for _cpm_v in $%s; set -e $_cpm_v; end; end;\n", managedVarsName, managedVarsName)
		for _, v := range spec.Env {
			fmt.Fprintf(&b, "set -gx %s %s;\n", v.Key, ShellQuote(v.Value))
		}
		fmt.Fprintf(&b, "set -gx %s %s;\n", managedVarsName, strings.Join(names, " "))
		if !opts.Quiet {
			fmt.Fprintf(&b, "echo %s;\n", ShellQuote("Switched to profile: "+name))
		}
		return b.String()
	}

	// Unset only what a previous 'cpm use' exported. eval splits the list
	// the same way in bash and zsh.
	fmt.Fprintf(&b, "eval \"unset ${%s:-}\" 2>/dev/null;\n", managedVarsName)
	for _, v := range spec.Env {
		fmt.Fprintf(&b, "export %s=%s;\n", v.Key, ShellQuote(v.Value))
	}
	fmt.Fprintf(&b, "export %s=%s;\n", managedVarsName, ShellQuote(strings.Join(names, " ")))
	if !opts.Quiet {
		fmt.Fprintf(&b, "echo %s;\n", ShellQuote("Switched to profile: "+name))
	}
	return b.String()
}

// GenerateUnsetOutput prints the commands that undo a previous 'cpm use'.
func GenerateUnsetOutput(shell string) string {
	if IsFishShell(shell) {
		return fmt.Sprintf("if set -q %s; for _cpm_v in $%s; set -e $_cpm_v; end; set -e %s; end;\n", managedVarsName, managedVarsName, managedVarsName)
	}
	return fmt.Sprintf("eval \"unset ${%s:-}\" 2>/dev/null; unset %s;\n", managedVarsName, managedVarsName)
}

// CurrentProfile returns the name of the currently active profile from env.
func CurrentProfile() string {
	return os.Getenv("CLAUDE_PROFILE")
}

// CurrentConfigDir returns the currently active CLAUDE_CONFIG_DIR from env.
func CurrentConfigDir() string {
	return os.Getenv("CLAUDE_CONFIG_DIR")
}

// DetectProfileFile walks up from the given directory looking for .claude-profile
func DetectProfileFile(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, ".claude-profile")
		if data, err := os.ReadFile(candidate); err == nil {
			name := strings.TrimSpace(string(data))
			if name != "" {
				return name, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no .claude-profile file found")
}

// HookOptions controls the generated auto-switch hook.
type HookOptions struct {
	// Shell is "bash", "zsh" or "fish"; empty prints a bash/zsh hook that
	// detects which one it runs in.
	Shell string
	// DefaultProfile is used when no .claude-profile is found instead of
	// unsetting the profile.
	DefaultProfile string
}

// GenerateShellHook generates the hook for auto-switching on directory
// change. Add to .zshrc/.bashrc: eval "$(cpm hook)"; fish: cpm hook --shell fish | source
func GenerateShellHook(opts HookOptions) string {
	if IsFishShell(opts.Shell) {
		return generateFishHook(opts)
	}
	return generatePosixHook(opts)
}

func generatePosixHook(opts HookOptions) string {
	var b strings.Builder
	b.WriteString(`# cpm auto-switch hook — add to your .zshrc or .bashrc:
#   eval "$(cpm hook)"
_cpm_auto_switch() {
  local profile_file=""
  local dir="$PWD"
  while [ "$dir" != "/" ] && [ -n "$dir" ]; do
    if [ -f "$dir/.claude-profile" ]; then
      profile_file="$dir/.claude-profile"
      break
    fi
    dir="$(dirname "$dir")"
  done
  local target=""
  if [ -n "$profile_file" ]; then
    target="$(tr -d '[:space:]' < "$profile_file")"
  fi
`)
	if opts.DefaultProfile != "" {
		fmt.Fprintf(&b, "  if [ -z \"$target\" ]; then target=%s; fi\n", ShellQuote(opts.DefaultProfile))
	}
	fmt.Fprintf(&b, `  if [ -n "$target" ]; then
    if [ "$target" != "${CLAUDE_PROFILE:-}" ]; then
      local snippet
      if snippet="$(cpm use --quiet "$target" 2>/dev/null)"; then
        eval "$snippet"
        echo "[cpm] using profile: $target"
      fi
    fi
  elif [ -n "${%s:-}" ]; then
    %s    echo "[cpm] profile unset (no .claude-profile found)"
  fi
}
`, managedVarsName, GenerateUnsetOutput("posix"))
	b.WriteString(`if [ -n "${ZSH_VERSION:-}" ]; then
  autoload -Uz add-zsh-hook
  add-zsh-hook chpwd _cpm_auto_switch
else
  _cpm_prompt_hook() {
    if [ "${_CPM_LAST_PWD:-}" != "$PWD" ]; then
      _CPM_LAST_PWD="$PWD"
      _cpm_auto_switch
    fi
  }
  case ";${PROMPT_COMMAND:-};" in
    *";_cpm_prompt_hook;"*) ;;
    *) PROMPT_COMMAND="_cpm_prompt_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
  esac
fi
_cpm_auto_switch
`)
	return b.String()
}

func generateFishHook(opts HookOptions) string {
	var b strings.Builder
	b.WriteString(`# cpm auto-switch hook — add to ~/.config/fish/config.fish:
#   cpm hook --shell fish | source
function _cpm_auto_switch --on-variable PWD
  set -l profile_file ""
  set -l dir $PWD
  while test "$dir" != "/" -a -n "$dir"
    if test -f "$dir/.claude-profile"
      set profile_file "$dir/.claude-profile"
      break
    end
    set dir (dirname "$dir")
  end
  set -l target ""
  if test -n "$profile_file"
    set target (string trim < "$profile_file")
  end
`)
	if opts.DefaultProfile != "" {
		fmt.Fprintf(&b, "  if test -z \"$target\"; set target %s; end\n", ShellQuote(opts.DefaultProfile))
	}
	fmt.Fprintf(&b, `  if test -n "$target"
    if test "$target" != "$CLAUDE_PROFILE"
      if cpm use --shell fish --quiet "$target" 2>/dev/null | source
        echo "[cpm] using profile: $target"
      end
    end
  else if set -q %s
    %s    echo "[cpm] profile unset (no .claude-profile found)"
  end
end
_cpm_auto_switch
`, managedVarsName, GenerateUnsetOutput("fish"))
	return b.String()
}

// LinkProfile creates a .claude-profile file in the given directory and adds it to .gitignore.
func LinkProfile(dir, profileName string) error {
	profilePath := filepath.Join(dir, ".claude-profile")
	if err := os.WriteFile(profilePath, []byte(profileName+"\n"), 0o644); err != nil {
		return fmt.Errorf("cannot write .claude-profile: %w", err)
	}

	// Add to .gitignore if it exists and doesn't already contain .claude-profile
	gitignorePath := filepath.Join(dir, ".gitignore")
	if data, err := os.ReadFile(gitignorePath); err == nil {
		lines := strings.Split(string(data), "\n")
		found := false
		for _, line := range lines {
			if strings.TrimSpace(line) == ".claude-profile" {
				found = true
				break
			}
		}
		if !found {
			entry := "\n# Claude profile (cpm)\n.claude-profile\n"
			if err := os.WriteFile(gitignorePath, append(data, []byte(entry)...), 0o644); err != nil {
				return fmt.Errorf("cannot update .gitignore: %w", err)
			}
			outln("  added .claude-profile to .gitignore")
		}
	}

	return nil
}

// UnlinkProfile removes the .claude-profile file from the given directory.
func UnlinkProfile(dir string) error {
	profilePath := filepath.Join(dir, ".claude-profile")
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		return fmt.Errorf("no .claude-profile in current directory")
	}
	return os.Remove(profilePath)
}

func PromptString() string {
	profile := CurrentProfile()
	if profile == "" {
		return ""
	}
	return profile
}
