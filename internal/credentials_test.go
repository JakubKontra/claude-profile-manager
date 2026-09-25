package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestParseCredentialsJSONClaudeAiOauth(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	future := now.Add(2 * time.Hour)
	data := []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-x","refreshToken":"r","expiresAt":%d,"scopes":["user:inference"],"subscriptionType":"max"}}`, future.UnixMilli()))

	info, err := parseCredentialsJSON(data, time.Time{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if info.Expired || !info.ExpiresAt.Equal(future) || info.SubscriptionType != "max" {
		t.Errorf("unexpected info: %+v", info)
	}

	past := now.Add(-time.Minute)
	data = []byte(fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, past.UnixMilli()))
	info, _ = parseCredentialsJSON(data, time.Time{}, now)
	if !info.Expired {
		t.Errorf("token expired a minute ago should be reported expired: %+v", info)
	}
}

func TestParseCredentialsJSONLegacyShapes(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	info, err := parseCredentialsJSON([]byte(`{"email":"a@b.c","expires_at":`+fmt.Sprint(now.Add(time.Hour).Unix())+`}`), time.Time{}, now)
	if err != nil || info.Account != "a@b.c" || info.Expired {
		t.Errorf("legacy expires_at: %v %+v", err, info)
	}

	modTime := now.Add(-2 * time.Hour)
	info, _ = parseCredentialsJSON([]byte(`{"subject":"sub","expires_in":3600}`), modTime, now)
	if info.Account != "sub" || !info.Expired {
		t.Errorf("expires_in relative to mtime should be expired: %+v", info)
	}

	info, _ = parseCredentialsJSON([]byte(`{"account_uuid":"u1"}`), time.Time{}, now)
	if info.Account != "u1" || info.Expired || !info.ExpiresAt.IsZero() {
		t.Errorf("no expiry info should mean not expired: %+v", info)
	}

	if _, err := parseCredentialsJSON([]byte(`{not json`), time.Time{}, now); err == nil {
		t.Error("invalid JSON should error")
	}
}

func TestGetCredentialStatusFromFileReportsSubscription(t *testing.T) {
	stubKeychain(t, false)
	dir := t.TempDir()
	future := time.Now().Add(time.Hour)
	data := fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d,"subscriptionType":"pro"}}`, future.UnixMilli())
	os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(data), 0o600)

	status, err := GetCredentialStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Source != "file" || status.SubscriptionType != "pro" || status.Expired || status.ExpiresAt.IsZero() {
		t.Errorf("unexpected status: %+v", status)
	}

	buf := captureOutput(t)
	base := filepath.Dir(dir)
	RenderCredentialReport(CredentialReport(&Config{Profiles: map[string]*Profile{filepath.Base(dir): {}}}, base))
	if !strings.Contains(buf.String(), "valid until") || !strings.Contains(buf.String(), "pro, file") {
		t.Errorf("unexpected render: %q", buf.String())
	}
}
