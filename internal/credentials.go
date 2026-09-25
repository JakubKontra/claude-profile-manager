package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"time"
)

// keychainServiceBase is the macOS Keychain service Claude Code writes its
// OAuth token under. For a custom CLAUDE_CONFIG_DIR it appends a suffix
// derived from that path, which is how profiles stay isolated from each other
// and from the default ~/.claude login.
const keychainServiceBase = "Claude Code-credentials"

// keychainAccountPattern mirrors Claude Code's own validation of $USER before
// it is used as the Keychain account name.
var keychainAccountPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// keychainLookup reports whether a Keychain entry exists. Replaced in tests.
var keychainLookup = defaultKeychainLookup

// CredentialStatus describes where a profile's login lives and which account
// it belongs to.
type CredentialStatus struct {
	Authenticated    bool
	Source           string // "keychain" or "file"
	Account          string
	Organization     string
	SubscriptionType string
	Expired          bool
	ExpiresAt        time.Time
	LastUpdated      time.Time
}

// CredentialFileInfo is what a .credentials.json file (or a Keychain token
// blob, which has the same shape) tells us.
type CredentialFileInfo struct {
	Account          string
	SubscriptionType string
	ExpiresAt        time.Time
	Expired          bool
}

// credentialsJSON mirrors the parts of Claude Code's credential blob we read.
// Tokens are deliberately not decoded.
type credentialsJSON struct {
	ClaudeAiOauth struct {
		ExpiresAt        int64  `json:"expiresAt"` // milliseconds since epoch
		SubscriptionType string `json:"subscriptionType"`
	} `json:"claudeAiOauth"`

	// Legacy / alternative shapes.
	Email       string  `json:"email"`
	Subject     string  `json:"subject"`
	AccountUUID string  `json:"account_uuid"`
	ExpiresAt   float64 `json:"expires_at"` // seconds since epoch
	ExpiresIn   float64 `json:"expires_in"` // seconds, relative to file mtime
}

// parseCredentialsJSON extracts account and expiry from a credential blob.
// now is injected for tests.
func parseCredentialsJSON(data []byte, fileModTime, now time.Time) (CredentialFileInfo, error) {
	var creds credentialsJSON
	if err := json.Unmarshal(data, &creds); err != nil {
		return CredentialFileInfo{}, fmt.Errorf("cannot parse credentials")
	}

	info := CredentialFileInfo{SubscriptionType: creds.ClaudeAiOauth.SubscriptionType}

	switch {
	case creds.Email != "":
		info.Account = creds.Email
	case creds.Subject != "":
		info.Account = creds.Subject
	case creds.AccountUUID != "":
		info.Account = creds.AccountUUID
	}

	switch {
	case creds.ClaudeAiOauth.ExpiresAt > 0:
		info.ExpiresAt = time.UnixMilli(creds.ClaudeAiOauth.ExpiresAt)
	case creds.ExpiresAt > 0:
		info.ExpiresAt = time.Unix(int64(creds.ExpiresAt), 0)
	case creds.ExpiresIn > 0 && !fileModTime.IsZero():
		info.ExpiresAt = fileModTime.Add(time.Duration(creds.ExpiresIn) * time.Second)
	}
	if !info.ExpiresAt.IsZero() {
		info.Expired = now.After(info.ExpiresAt)
	}
	return info, nil
}

// GetCredentialInfo reads <profileDir>/.credentials.json.
func GetCredentialInfo(profileDir string) (CredentialFileInfo, error) {
	credPath := filepath.Join(profileDir, ".credentials.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		return CredentialFileInfo{}, fmt.Errorf("no credentials found")
	}
	var modTime time.Time
	if st, err := os.Stat(credPath); err == nil {
		modTime = st.ModTime()
	}
	return parseCredentialsJSON(data, modTime, time.Now())
}

// KeychainServiceName returns the Keychain service Claude Code uses for a
// given CLAUDE_CONFIG_DIR: the base service plus the first 8 hex characters of
// the SHA-256 of the config dir path. The default config dir (~/.claude) is
// used without a suffix, so profiles never collide with it.
//
// Claude Code NFC-normalizes the path first; that only matters for config dirs
// containing decomposed non-ASCII characters, which cpm's own profile paths
// never have.
func KeychainServiceName(configDir string) string {
	sum := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("%s-%s", keychainServiceBase, hex.EncodeToString(sum[:])[:8])
}

// keychainAccount mirrors Claude Code's account-name resolution.
func keychainAccount() string {
	name := os.Getenv("USER")
	if name == "" {
		if u, err := user.Current(); err == nil {
			name = u.Username
		}
	}
	if !keychainAccountPattern.MatchString(name) {
		return "claude-code-user"
	}
	return name
}

// defaultKeychainLookup checks for the entry's existence only. It deliberately
// omits `-w`, so the token is never read and macOS shows no access prompt.
func defaultKeychainLookup(service, account string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	cmd := exec.Command("security", "find-generic-password", "-a", account, "-s", service)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// GetCredentialStatus reports whether a profile is logged in, checking the
// macOS Keychain first and falling back to .credentials.json (Linux, CI, or
// Keychain-less setups). On macOS the file never exists, which is why a
// file-only check reports a false "not authenticated".
func GetCredentialStatus(profileDir string) (CredentialStatus, error) {
	account, org, updated := readAccountInfo(profileDir)

	if keychainLookup(KeychainServiceName(profileDir), keychainAccount()) {
		return CredentialStatus{
			Authenticated: true,
			Source:        "keychain",
			Account:       account,
			Organization:  org,
			LastUpdated:   updated,
		}, nil
	}

	fileInfo, err := GetCredentialInfo(profileDir)
	if err != nil {
		return CredentialStatus{}, err
	}
	if account == "" {
		account = fileInfo.Account
	}
	if info, statErr := os.Stat(filepath.Join(profileDir, ".credentials.json")); statErr == nil {
		updated = info.ModTime()
	}

	return CredentialStatus{
		Authenticated:    true,
		Source:           "file",
		Account:          account,
		Organization:     org,
		SubscriptionType: fileInfo.SubscriptionType,
		Expired:          fileInfo.Expired,
		ExpiresAt:        fileInfo.ExpiresAt,
		LastUpdated:      updated,
	}, nil
}

// readAccountInfo pulls the logged-in account out of the profile's
// .claude.json, which Claude Code writes regardless of where the token itself
// is stored.
func readAccountInfo(profileDir string) (account, organization string, updated time.Time) {
	path := filepath.Join(profileDir, ".claude.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", time.Time{}
	}

	var parsed struct {
		OAuthAccount struct {
			EmailAddress     string `json:"emailAddress"`
			OrganizationUUID string `json:"organizationUuid"`
		} `json:"oauthAccount"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", "", time.Time{}
	}

	if info, err := os.Stat(path); err == nil {
		updated = info.ModTime()
	}
	return parsed.OAuthAccount.EmailAddress, parsed.OAuthAccount.OrganizationUUID, updated
}
