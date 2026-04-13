package internal

import "fmt"

func GenerateDirenvSnippet(name, profileDir string) string {
	return fmt.Sprintf(`# Claude Code profile: %s
# Add this to your .envrc file
export CLAUDE_CONFIG_DIR="%s"
export CLAUDE_PROFILE="%s"
`, name, profileDir, name)
}
