package internal

// StatusReport is the result of `cpm status`.
type StatusReport struct {
	InSync   bool            `json:"in_sync"`
	Diverged []DivergedEntry `json:"diverged"`
}

// DivergedEntry is a profile file with local changes.
type DivergedEntry struct {
	Profile string `json:"profile"`
	File    string `json:"file"`
	details string
}

// Status reports copied files that differ from the source directory.
func (a *App) Status(cfg *Config) StatusReport {
	report := StatusReport{Diverged: []DivergedEntry{}}
	for _, d := range CheckDivergence(cfg, a.BaseDir) {
		report.Diverged = append(report.Diverged, DivergedEntry{Profile: d.Profile, File: d.Filename, details: d.Details})
	}
	report.InSync = len(report.Diverged) == 0
	return report
}

// RenderStatus prints the human-readable status report.
func (a *App) RenderStatus(report StatusReport) {
	if report.InSync {
		a.outln("All profiles are in sync with source.")
		return
	}

	a.outln("Diverged files:")
	for _, d := range report.Diverged {
		a.outf("  %s\n", d.details)
	}
	a.outln("\nRun 'cpm install --sync' to re-sync.")
}
