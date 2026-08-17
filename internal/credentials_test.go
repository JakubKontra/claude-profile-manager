package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubKeychain replaces the Keychain lookup for the duration of a test.
func stubKeychain(t *testing.T, found bool) {
	t.Helper()
	original := keychainLookup
	keychainLookup = func(service, account string) bool { return found }
	t.Cleanup(func() { keychainLookup = original })
}

func TestKeychainServiceName(t *testing.T) {
	// Vector matches what Claude Code derives: base service plus the first 8
	// hex chars of sha256(configDir).
	got := KeychainServiceName("/tmp/profiles/work")
	want := "Claude Code-credentials-8ba64566"
	if got != want {
		t.Errorf("KeychainServiceName = %q, want %q", got, want)
	}

	if KeychainServiceName("/tmp/profiles/work") == KeychainServiceName("/tmp/profiles/personal") {
		t.Error("different config dirs must map to different Keychain services")
	}
}

func TestGetCredentialStatusFromKeychain(t *testing.T) {
	stubKeychain(t, true)

	dir := t.TempDir()
	writeClaudeJSON(t, dir, "work@example.com", "org-uuid")

	status, err := GetCredentialStatus(dir)
	if err != nil {
		t.Fatalf("GetCredentialStatus failed: %v", err)
	}
	if !status.Authenticated {
		t.Error("profile with a Keychain entry should be authenticated")
	}
	if status.Source != "keychain" {
		t.Errorf("source = %q, want keychain", status.Source)
	}
	if status.Account != "work@example.com" {
		t.Errorf("account = %q, want work@example.com", status.Account)
	}
	if status.Organization != "org-uuid" {
		t.Errorf("organization = %q, want org-uuid", status.Organization)
	}
}

func TestGetCredentialStatusFileFallback(t *testing.T) {
	stubKeychain(t, false)

	dir := t.TempDir()
	creds := map[string]any{
		"email":      "file@example.com",
		"expires_at": float64(time.Now().Add(time.Hour).Unix()),
	}
	data, _ := json.Marshal(creds)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), data, 0o644)

	status, err := GetCredentialStatus(dir)
	if err != nil {
		t.Fatalf("GetCredentialStatus failed: %v", err)
	}
	if status.Source != "file" {
		t.Errorf("source = %q, want file", status.Source)
	}
	if status.Account != "file@example.com" {
		t.Errorf("account = %q, want file@example.com", status.Account)
	}
	if status.Expired {
		t.Error("credentials should not be expired")
	}
}

func TestGetCredentialStatusNotAuthenticated(t *testing.T) {
	stubKeychain(t, false)

	dir := t.TempDir()
	writeClaudeJSON(t, dir, "stale@example.com", "org-uuid")

	if _, err := GetCredentialStatus(dir); err == nil {
		t.Error("expected error when neither Keychain nor credentials file has a login")
	}
}

func TestKeychainAccountFallsBackForOddUsernames(t *testing.T) {
	t.Setenv("USER", "not a valid/name")
	if got := keychainAccount(); got != "claude-code-user" {
		t.Errorf("keychainAccount = %q, want claude-code-user", got)
	}

	t.Setenv("USER", "jakub.kontra")
	if got := keychainAccount(); got != "jakub.kontra" {
		t.Errorf("keychainAccount = %q, want jakub.kontra", got)
	}
}

func writeClaudeJSON(t *testing.T, dir, email, org string) {
	t.Helper()
	payload := map[string]any{
		"oauthAccount": map[string]any{
			"emailAddress":     email,
			"organizationUuid": org,
		},
	}
	data, _ := json.Marshal(payload)
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), data, 0o644); err != nil {
		t.Fatalf("cannot write .claude.json: %v", err)
	}
}
