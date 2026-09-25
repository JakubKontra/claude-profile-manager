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

// PrintVersion prints the banner, version and a hint when a newer release exists.
func PrintVersion() {
	out(Banner)
	outf("  Version: %s\n  Commit:  %s\n", Version, Commit)

	latest, err := CheckLatestVersion()
	if err == nil && latest != "" && latest != "v"+Version && latest != Version {
		outf("\nNew version available: %s (current: %s)\n", latest, Version)
		outln("Run 'cpm upgrade' to update.")
	}
}
