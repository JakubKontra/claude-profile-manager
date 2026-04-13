package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GenerateUseOutput outputs shell commands to be eval'd by the user's shell.
// Usage: eval "$(cpm use <profile>)"
func GenerateUseOutput(name string, profileDir string, profile *Profile) string {
	var b strings.Builder

	// Unset CLAUDE_*/ANTHROPIC_* vars
	b.WriteString("unset $(env | grep -E '^(CLAUDE_|ANTHROPIC_)' | cut -d= -f1) 2>/dev/null;\n")

	b.WriteString(fmt.Sprintf("export CLAUDE_CONFIG_DIR=\"%s\";\n", profileDir))
	b.WriteString(fmt.Sprintf("export CLAUDE_PROFILE=\"%s\";\n", name))

	for k, v := range profile.Env {
		b.WriteString(fmt.Sprintf("export %s=\"%s\";\n", k, v))
	}

	b.WriteString(fmt.Sprintf("echo \"Switched to profile: %s\";\n", name))

	return b.String()
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

// GenerateShellHook generates a shell hook function for auto-switching.
// Add to .zshrc/.bashrc: eval "$(cpm hook)"
func GenerateShellHook() string {
	return `# cpm auto-switch hook — add to your .zshrc or .bashrc:
#   eval "$(cpm hook)"
_cpm_auto_switch() {
  local profile_file=""
  local dir="$PWD"
  while [ "$dir" != "/" ]; do
    if [ -f "$dir/.claude-profile" ]; then
      profile_file="$dir/.claude-profile"
      break
    fi
    dir="$(dirname "$dir")"
  done
  if [ -n "$profile_file" ]; then
    local target
    target="$(cat "$profile_file" | tr -d '[:space:]')"
    if [ -n "$target" ] && [ "$target" != "${CLAUDE_PROFILE:-}" ]; then
      eval "$(cpm use "$target" 2>/dev/null)"
      echo "[cpm] using profile: $target"
    fi
  elif [ -n "${CLAUDE_PROFILE:-}" ]; then
    unset CLAUDE_CONFIG_DIR CLAUDE_PROFILE
    unset $(env | grep -E '^(CLAUDE_|ANTHROPIC_)' | cut -d= -f1) 2>/dev/null
    echo "[cpm] profile unset (no .claude-profile found)"
  fi
}
if [ -n "$ZSH_VERSION" ]; then
  autoload -Uz add-zsh-hook
  add-zsh-hook chpwd _cpm_auto_switch
else
  _cpm_original_cd() { builtin cd "$@" && _cpm_auto_switch; }
  alias cd='_cpm_original_cd'
fi
_cpm_auto_switch
`
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
			fmt.Println("  added .claude-profile to .gitignore")
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
