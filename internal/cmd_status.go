package internal

// ShowStatus reports copied files that differ from the source directory.
func (a *App) ShowStatus(cfg *Config) {
	diverged := CheckDivergence(cfg, a.BaseDir)

	if len(diverged) == 0 {
		a.outln("All profiles are in sync with source.")
		return
	}

	a.outln("Diverged files:")
	for _, d := range diverged {
		a.outf("  %s\n", d.Details)
	}
	a.outln("\nRun 'cpm install --sync' to re-sync.")
}
