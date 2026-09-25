package internal

// Link validates the profile and writes .claude-profile into dir.
func Link(cfg *Config, dir, name string) error {
	if _, err := lookupProfile(cfg, name); err != nil {
		return err
	}
	if err := LinkProfile(dir, name); err != nil {
		return err
	}
	outf("Linked profile %q to %s\n", name, dir)
	outln("\nTo auto-switch, add to your .zshrc:")
	outln("  eval \"$(cpm hook)\"")
	return nil
}

// Unlink removes .claude-profile from dir.
func Unlink(dir string) error {
	if err := UnlinkProfile(dir); err != nil {
		return err
	}
	outln("Removed .claude-profile")
	return nil
}
