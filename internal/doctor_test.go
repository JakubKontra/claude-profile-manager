package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunDoctorBasic(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profilesBase := filepath.Join(tmpDir, "profiles")
	binDir := filepath.Join(tmpDir, "bin")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(profilesBase, 0o755)
	os.MkdirAll(binDir, 0o755)

	cfg := &Config{
		SourceDir: sourceDir,
		BinDir:    binDir,
		Profiles: map[string]*Profile{
			"test": {Description: "Test"},
		},
	}

	checks := RunDoctor(cfg, profilesBase)

	// Should have at least checks for claude binary, source dir, bin dir
	if len(checks) < 3 {
		t.Errorf("expected at least 3 checks, got %d", len(checks))
	}

	// Source dir should be OK
	found := false
	for _, c := range checks {
		if c.Name == "source directory" && c.Status == "ok" {
			found = true
			break
		}
	}
	if !found {
		t.Error("source directory check should be OK")
	}
}

func TestRunDoctorMissingSourceDir(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		SourceDir: filepath.Join(tmpDir, "nonexistent"),
		BinDir:    tmpDir,
		Profiles:  map[string]*Profile{"test": {}},
	}

	checks := RunDoctor(cfg, tmpDir)

	found := false
	for _, c := range checks {
		if c.Name == "source directory" && c.Status == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing source directory should produce error check")
	}
}

func TestRunDoctorProfileNotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0o755)

	cfg := &Config{
		SourceDir: sourceDir,
		BinDir:    tmpDir,
		Profiles:  map[string]*Profile{"missing": {Description: "Missing"}},
	}

	checks := RunDoctor(cfg, tmpDir)

	found := false
	for _, c := range checks {
		if c.Name == "profile/missing" && c.Status == "warn" {
			found = true
			break
		}
	}
	if !found {
		t.Error("uninstalled profile should produce warn check")
	}
}

func TestGetCredentialInfoNoFile(t *testing.T) {
	dir := t.TempDir()
	_, _, err := GetCredentialInfo(dir)
	if err == nil {
		t.Error("expected error for missing credentials")
	}
}

func TestGetCredentialInfoWithEmail(t *testing.T) {
	dir := t.TempDir()
	creds := map[string]any{
		"email":      "user@example.com",
		"expires_at": float64(time.Now().Add(time.Hour).Unix()),
	}
	data, _ := json.Marshal(creds)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), data, 0o644)

	account, expired, err := GetCredentialInfo(dir)
	if err != nil {
		t.Fatalf("GetCredentialInfo failed: %v", err)
	}
	if account != "user@example.com" {
		t.Errorf("account = %q, want user@example.com", account)
	}
	if expired {
		t.Error("credentials should not be expired")
	}
}

func TestGetCredentialInfoExpired(t *testing.T) {
	dir := t.TempDir()
	creds := map[string]any{
		"email":      "user@example.com",
		"expires_at": float64(time.Now().Add(-time.Hour).Unix()),
	}
	data, _ := json.Marshal(creds)
	os.WriteFile(filepath.Join(dir, ".credentials.json"), data, 0o644)

	_, expired, err := GetCredentialInfo(dir)
	if err != nil {
		t.Fatalf("GetCredentialInfo failed: %v", err)
	}
	if !expired {
		t.Error("credentials should be expired")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{5 * time.Hour, "5h"},
		{3 * 24 * time.Hour, "3d"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestPrintChecks(t *testing.T) {
	// Just verify it doesn't panic
	checks := []Check{
		{"test ok", "ok", "details"},
		{"test warn", "warn", "details"},
		{"test error", "error", "details"},
	}
	PrintChecks(checks)
}
