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

// PrintVersion prints the banner, version and a hint when a newer release
// exists; prerelease includes prereleases, as Upgrade does.
func (a *App) PrintVersion(prerelease bool) {
	a.out(Banner)
	a.outf("  Version: %s\n  Commit:  %s\n", Version, Commit)

	pre := wantsPrerelease(prerelease)
	latest, err := CheckLatestVersion(pre)
	if err == nil && latest != "" && isNewer(latest) {
		a.outf("\nNew version available: %s (current: %s)\n", latest, Version)
		if pre {
			a.outln("Run 'cpm upgrade --prerelease' to update.")
		} else {
			a.outln("Run 'cpm upgrade' to update.")
		}
	}
}
