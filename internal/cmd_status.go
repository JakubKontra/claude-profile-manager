package internal

// ShowStatus reports copied files that differ from the source directory.
func ShowStatus(cfg *Config, profilesBase string) {
	diverged := CheckDivergence(cfg, profilesBase)

	if len(diverged) == 0 {
		outln("All profiles are in sync with source.")
		return
	}

	outln("Diverged files:")
	for _, d := range diverged {
		outf("  %s\n", d.Details)
	}
	outln("\nRun 'cpm install --sync' to re-sync.")
}
