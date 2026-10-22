package internal

// Banner is the ASCII art shown by `cpm version` and the root help.
const Banner = `
   _____ ____  __  __
  / ____|  _ \|  \/  |
 | |    | |_) | \  / |
 | |    |  __/| |\/| |
 | |____| |   | |  | |
  \_____|_|   |_|  |_|
  Claude Profile Manager
`

// VersionReport is the result of `cpm version`. Latest is empty when the
// release check failed.
type VersionReport struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	prerelease      bool
}

// VersionInfo returns this build's version and the newest release;
// prerelease includes prereleases, as Upgrade does.
func VersionInfo(prerelease bool) VersionReport {
	report := VersionReport{Version: Version, Commit: Commit, prerelease: wantsPrerelease(prerelease)}
	if latest, err := CheckLatestVersion(report.prerelease); err == nil {
		report.Latest = latest
		report.UpdateAvailable = latest != "" && isNewer(latest)
	}
	return report
}

// PrintVersion prints the banner, version and a hint when a newer release
// exists.
func (a *App) PrintVersion(r VersionReport) {
	a.out(Banner)
	a.outf("  Version: %s\n  Commit:  %s\n", r.Version, r.Commit)
	if r.UpdateAvailable {
		a.outf("\nNew version available: %s (current: %s)\n", r.Latest, r.Version)
		if r.prerelease {
			a.outln("Run 'cpm upgrade --prerelease' to update.")
		} else {
			a.outln("Run 'cpm upgrade' to update.")
		}
	}
}
