package internal

// Link validates the profile and writes .claude-profile into dir.
func (a *App) Link(cfg *Config, dir, name string) error {
	if _, err := lookupProfile(cfg, name); err != nil {
		return err
	}
	if err := a.LinkProfile(dir, name); err != nil {
		return err
	}
	a.outf("Linked profile %q to %s\n", name, dir)
	a.outln("\nTo auto-switch, add to your .zshrc:")
	a.outln("  eval \"$(cpm hook)\"")
	return nil
}

// Unlink removes .claude-profile from dir.
func (a *App) Unlink(dir string) error {
	if err := UnlinkProfile(dir); err != nil {
		return err
	}
	a.outln("Removed .claude-profile")
	return nil
}
