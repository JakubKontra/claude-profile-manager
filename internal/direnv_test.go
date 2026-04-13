package internal

import (
	"strings"
	"testing"
)

func TestGenerateDirenvSnippet(t *testing.T) {
	out := GenerateDirenvSnippet("work", "/home/user/.claude-profiles/work")

	mustContain := []string{
		`CLAUDE_CONFIG_DIR="/home/user/.claude-profiles/work"`,
		`CLAUDE_PROFILE="work"`,
	}

	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("direnv snippet missing %q", s)
		}
	}
}
