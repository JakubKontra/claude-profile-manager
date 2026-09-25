package internal

// WhichResult describes which profile is active and where that came from.
type WhichResult struct {
	Active    bool   `json:"active"`
	Profile   string `json:"profile,omitempty"`
	Source    string `json:"source,omitempty"` // "env" or "file"
	ConfigDir string `json:"config_dir,omitempty"`
	Command   string `json:"command,omitempty"`
}

// Which resolves the active profile from the environment, falling back to a
// .claude-profile file found from cwd upwards.
func Which(cwd string) WhichResult {
	if profile := CurrentProfile(); profile != "" {
		return WhichResult{
			Active:    true,
			Profile:   profile,
			Source:    "env",
			ConfigDir: CurrentConfigDir(),
			Command:   wrapperPrefix + profile,
		}
	}
	if detected, err := DetectProfileFile(cwd); err == nil {
		return WhichResult{
			Profile: detected,
			Source:  "file",
			Command: wrapperPrefix + detected,
		}
	}
	return WhichResult{}
}

// RenderWhich prints the human-readable form of a WhichResult.
func RenderWhich(r WhichResult) {
	switch {
	case r.Active:
		outf("Profile:    %s\n", r.Profile)
		outf("Config dir: %s\n", r.ConfigDir)
		outln("Source:     environment (CLAUDE_PROFILE)")
		outf("Command:    %s\n", r.Command)
	case r.Profile != "":
		outf("Profile:    %s\n", r.Profile)
		outln("Source:     .claude-profile")
		outf("Command:    %s\n", r.Command)
		outln("\nNote: profile detected from .claude-profile but not active in this shell.")
		outf("Run: eval \"$(cpm use %s)\"\n", r.Profile)
		outln("Or add to .zshrc: eval \"$(cpm hook)\"")
	default:
		outln("No active profile.")
		outln("  - No CLAUDE_PROFILE env var set")
		outln("  - No .claude-profile file found in current or parent directories")
	}
}
