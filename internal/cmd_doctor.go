package internal

import (
	"errors"
	"strings"
)

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
	if opts.User == "" {
		opts.User = a.Getenv("USER")
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

// DoctorFix runs the checks, repairs the ones `cpm install` fixes (missing
// profile directories, wrappers and bin dir, outdated wrappers, broken
// symlinks into the source dir) and returns the report after the repair.
// Other findings only carry a Fix hint. Cloud auto-pull and auto-push are
// skipped.
func (a *App) DoctorFix(cfg *Config, opts DoctorOptions) (DoctorReport, error) {
	report := a.DoctorReportFor(cfg, opts)
	var fixable []string
	for _, c := range report.Checks {
		if c.Status != "ok" && c.Fix == installFix {
			fixable = append(fixable, c.Name)
		}
	}
	if len(fixable) == 0 {
		return report, nil
	}
	a.progressf("Fixing %d check%s with cpm install: %s\n", len(fixable), pluralS(len(fixable)), strings.Join(fixable, ", "))
	if a.JSON {
		out := a.Out
		a.Out = a.Err
		defer func() { a.Out = out }()
	}
	if err := a.InstallProfiles(cfg, InstallOptions{skipCloud: true}); err != nil {
		return report, err
	}
	a.progressf("\n")
	return a.DoctorReportFor(cfg, opts), nil
}
