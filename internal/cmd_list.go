package internal

import (
	"os"
	"path/filepath"
)

// ProfileListEntry is one row of `cpm list`.
type ProfileListEntry struct {
	Name             string `json:"name"`
	Command          string `json:"command"`
	Description      string `json:"description"`
	Dir              string `json:"dir"`
	Installed        bool   `json:"installed"`
	Authenticated    bool   `json:"authenticated"`
	Account          string `json:"account,omitempty"`
	CredentialSource string `json:"credential_source,omitempty"`
	Expired          bool   `json:"expired"`
	Current          bool   `json:"current"`
}

// ListProfiles gathers the status of every configured profile.
func ListProfiles(cfg *Config, profilesBase string) []ProfileListEntry {
	current := CurrentProfile()
	var entries []ProfileListEntry

	for _, name := range SortedProfileNames(cfg) {
		profile := cfg.Profiles[name]
		profileDir := filepath.Join(profilesBase, name)

		entry := ProfileListEntry{
			Name:        name,
			Command:     wrapperPrefix + name,
			Description: profile.Description,
			Dir:         profileDir,
			Current:     name == current,
		}
		if _, err := os.Stat(profileDir); err == nil {
			entry.Installed = true
			if status, err := GetCredentialStatus(profileDir); err == nil {
				entry.Authenticated = status.Authenticated
				entry.Account = status.Account
				entry.CredentialSource = status.Source
				entry.Expired = status.Expired
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// RenderProfileList prints the human-readable list.
func RenderProfileList(entries []ProfileListEntry) {
	for _, e := range entries {
		status := "not installed"
		switch {
		case e.Authenticated && e.Expired:
			status = "expired"
		case e.Authenticated:
			status = "authenticated"
		case e.Installed:
			status = "installed"
		}

		desc := e.Description
		if desc == "" {
			desc = "(no description)"
		}

		marker := "  "
		if e.Current {
			marker = "* "
		}

		outf("%s%-26s %s  [%s]\n", marker, e.Command, desc, status)
	}
}
