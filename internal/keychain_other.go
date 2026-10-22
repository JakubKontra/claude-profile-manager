//go:build !darwin

package internal

// platformKeychain returns a store with no entries; outside macOS Claude Code
// keeps logins in .credentials.json.
func platformKeychain() KeychainStore { return noKeychain{} }
