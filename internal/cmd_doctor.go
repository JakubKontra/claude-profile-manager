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
func DoctorReportFor(cfg *Config, profilesBase string, opts DoctorOptions) DoctorReport {
	checks := RunDoctorWithOptions(cfg, profilesBase, opts)
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
func RenderDoctorReport(r DoctorReport) {
	out("cpm doctor\n\n")
	PrintChecks(r.Checks)
	if r.OK {
		outln("\nAll checks passed.")
	} else {
		outln("\nSome checks failed. Fix the issues above.")
	}
}
