package internal

import (
	"path/filepath"
	"time"
)

// CredentialEntry is one row of `cpm credentials`.
type CredentialEntry struct {
	Name          string     `json:"name"`
	Authenticated bool       `json:"authenticated"`
	Source        string     `json:"source,omitempty"`
	Account       string     `json:"account,omitempty"`
	Organization  string     `json:"organization,omitempty"`
	Subscription  string     `json:"subscription_type,omitempty"`
	Expired       bool       `json:"expired"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Error         string     `json:"error,omitempty"`
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
			entry.Subscription = status.SubscriptionType
			entry.Expired = status.Expired
			if !status.ExpiresAt.IsZero() {
				expires := status.ExpiresAt
				entry.ExpiresAt = &expires
			}
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
		switch {
		case e.Expired:
			state = "EXPIRED"
		case e.ExpiresAt != nil:
			state = "valid until " + e.ExpiresAt.Local().Format("2006-01-02 15:04")
		}
		detail := state
		if e.Subscription != "" {
			detail += ", " + e.Subscription
		}
		detail += ", " + e.Source
		if e.Organization != "" {
			detail += ", org " + e.Organization
		}
		outf("  claude-%-20s %s  [%s]\n", e.Name, account, detail)
	}
}
