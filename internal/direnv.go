package internal

import (
	"fmt"
	"strings"
)

// GenerateDirenvSnippet renders the .envrc lines for a profile.
func GenerateDirenvSnippet(name, profileDir string, profile *Profile) string {
	spec := BuildLaunchSpec(name, profileDir, profile)
	var b strings.Builder
	fmt.Fprintf(&b, "# Claude Code profile: %s\n# Add this to your .envrc file\n", name)
	for _, v := range spec.Env {
		fmt.Fprintf(&b, "export %s=%s\n", v.Key, ShellQuote(v.Value))
	}
	return b.String()
}
