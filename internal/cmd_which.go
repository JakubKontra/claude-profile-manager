package internal

// WhichResult describes which profile is active and where that came from.
type WhichResult struct {
	Active    bool   `json:"active"`
	Profile   string `json:"profile,omitempty"`
	Source    string `json:"source,omitempty"` // "env", "file" or "match"
	Rule      string `json:"rule,omitempty"`   // the [[match]] rule, for source "match"
	ConfigDir string `json:"config_dir,omitempty"`
	Command   string `json:"command,omitempty"`
}

// Which resolves the active profile from the environment, falling back to a
// .claude-profile file found from cwd upwards and then to [[match]] rules.
func (a *App) Which(cfg *Config, cwd string) WhichResult {
	if profile := a.CurrentProfile(); profile != "" {
		return WhichResult{
			Active:    true,
			Profile:   profile,
			Source:    "env",
			ConfigDir: a.CurrentConfigDir(),
			Command:   wrapperPrefix + profile,
		}
	}
	if res, ok := a.ResolveProfile(cfg, cwd); ok {
		return WhichResult{
			Profile: res.Profile,
			Source:  res.Source,
			Rule:    res.Rule,
			Command: wrapperPrefix + res.Profile,
		}
	}
	return WhichResult{}
}

// RenderWhich prints the human-readable form of a WhichResult.
func (a *App) RenderWhich(r WhichResult) {
	switch {
	case r.Active:
		a.outf("Profile:    %s\n", r.Profile)
		a.outf("Config dir: %s\n", r.ConfigDir)
		a.outln("Source:     environment (CLAUDE_PROFILE)")
		a.outf("Command:    %s\n", r.Command)
	case r.Profile != "":
		a.outf("Profile:    %s\n", r.Profile)
		source := ".claude-profile"
		if r.Source == "match" {
			source = "[[match]] " + r.Rule
		}
		a.outf("Source:     %s\n", source)
		a.outf("Command:    %s\n", r.Command)
		a.outf("\nNote: profile detected from %s but not active in this shell.\n", source)
		a.outf("Run: eval \"$(cpm use %s)\"\n", r.Profile)
		a.outln("Or add to .zshrc: eval \"$(cpm hook)\"")
	default:
		a.outln("No active profile.")
		a.outln("  - No CLAUDE_PROFILE env var set")
		a.outln("  - No .claude-profile file found in current or parent directories")
		a.outln("  - No [[match]] rule applies")
	}
}
