package internal

import "errors"

// ErrDoctorFailed is returned when at least one doctor check reported an error.
// The report has already been printed by then.
var ErrDoctorFailed = errors.New("doctor: some checks failed")

// DoctorReport is the result of `cpm doctor`.
type DoctorReport struct {
	OK       bool    `json:"ok"`
	Errors   int     `json:"errors"`
	Warnings int     `json:"warnings"`
	Checks   []Check `json:"checks"`
}

// DoctorReportFor runs all checks and summarizes them.
func (a *App) DoctorReportFor(cfg *Config, opts DoctorOptions) DoctorReport {
	if opts.Keychain == nil {
		opts.Keychain = a.Keychain
	}
	if opts.HomeDir == "" {
		opts.HomeDir = a.Home
	}
	if opts.PathEnv == "" {
		opts.PathEnv = a.Getenv("PATH")
	}
	if opts.LookPath == nil {
		opts.LookPath = a.lookPath
	}
	checks := RunDoctorWithOptions(cfg, a.BaseDir, opts)
	report := DoctorReport{Checks: checks}
	for _, c := range checks {
		switch c.Status {
		case "error":
			report.Errors++
		case "warn":
			report.Warnings++
		}
	}
	report.OK = report.Errors == 0
	return report
}

// RenderDoctorReport prints the human-readable report.
func (a *App) RenderDoctorReport(r DoctorReport) {
	a.out("cpm doctor\n\n")
	a.PrintChecks(r.Checks)
	if r.OK {
		a.outln("\nAll checks passed.")
	} else {
		a.outln("\nSome checks failed. Fix the issues above.")
	}
}
