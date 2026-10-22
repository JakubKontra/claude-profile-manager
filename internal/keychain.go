package internal

import "fmt"

// KeychainStore is the OS credential store Claude Code keeps its OAuth token
// in. On macOS it is the login Keychain; elsewhere there is none and logins
// live in .credentials.json.
type KeychainStore interface {
	// Exists reports whether an entry exists without reading the secret, so
	// macOS shows no access prompt.
	Exists(service, account string) bool
	// Read returns the stored secret. It may trigger a macOS access prompt.
	Read(service, account string) ([]byte, error)
	// Delete removes the entry; a missing entry is not an error.
	Delete(service, account string) error
}

// noKeychain is the store on systems without a Keychain.
type noKeychain struct{}

func (noKeychain) Exists(string, string) bool { return false }

func (noKeychain) Read(string, string) ([]byte, error) {
	return nil, fmt.Errorf("keychain is only available on macOS")
}

func (noKeychain) Delete(string, string) error { return nil }
