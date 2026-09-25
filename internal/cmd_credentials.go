package internal

import "path/filepath"

// CredentialEntry is one row of `cpm credentials`.
type CredentialEntry struct {
	Name          string `json:"name"`
	Authenticated bool   `json:"authenticated"`
	Source        string `json:"source,omitempty"`
	Account       string `json:"account,omitempty"`
	Organization  string `json:"organization,omitempty"`
	Expired       bool   `json:"expired"`
	Error         string `json:"error,omitempty"`
}

// CredentialReport gathers the credential status of every profile.
func CredentialReport(cfg *Config, profilesBase string) []CredentialEntry {
	var entries []CredentialEntry
	for _, name := range SortedProfileNames(cfg) {
		profileDir := filepath.Join(profilesBase, name)
		entry := CredentialEntry{Name: name}

		status, err := GetCredentialStatus(profileDir)
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.Authenticated = status.Authenticated
			entry.Source = status.Source
			entry.Account = status.Account
			entry.Organization = status.Organization
			entry.Expired = status.Expired
		}
		entries = append(entries, entry)
	}
	return entries
}

// RenderCredentialReport prints the human-readable report.
func RenderCredentialReport(entries []CredentialEntry) {
	for _, e := range entries {
		if e.Error != "" {
			outf("  claude-%-20s %s\n", e.Name, e.Error)
			continue
		}
		account := e.Account
		if account == "" {
			account = "(unknown account)"
		}
		state := "valid"
		if e.Expired {
			state = "EXPIRED"
		}
		detail := state + ", " + e.Source
		if e.Organization != "" {
			detail += ", org " + e.Organization
		}
		outf("  claude-%-20s %s  [%s]\n", e.Name, account, detail)
	}
}
