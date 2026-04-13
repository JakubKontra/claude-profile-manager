package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/foo", filepath.Join(home, "foo")},
		{"~/.claude", filepath.Join(home, ".claude")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tt := range tests {
		got := ExpandPath(tt.input)
		if got != tt.expected {
			t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	content := `
source_dir = "~/.claude"
bin_dir = "~/.local/bin"

[profiles.personal]
description = "Personal account"

[profiles.work]
description = "Work account"
model = "sonnet"
add_dirs = ["~/Work/company"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(cfg.Profiles))
	}

	personal := cfg.Profiles["personal"]
	if personal == nil {
		t.Fatal("personal profile not found")
	}
	if personal.Description != "Personal account" {
		t.Errorf("personal description = %q, want %q", personal.Description, "Personal account")
	}

	work := cfg.Profiles["work"]
	if work == nil {
		t.Fatal("work profile not found")
	}
	if work.Model != "sonnet" {
		t.Errorf("work model = %q, want %q", work.Model, "sonnet")
	}
	if len(work.AddDirs) != 1 || work.AddDirs[0] != "~/Work/company" {
		t.Errorf("work add_dirs = %v, want [~/Work/company]", work.AddDirs)
	}
}

func TestLoadConfigWithAttribution(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	content := `
[profiles.work]
description = "Work"

[profiles.work.attribution]
commit = "Co-Authored-By: Claude"
pr = "Generated with Claude Code"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	work := cfg.Profiles["work"]
	if work.Attribution == nil {
		t.Fatal("attribution is nil")
	}
	if work.Attribution.Commit != "Co-Authored-By: Claude" {
		t.Errorf("commit = %q", work.Attribution.Commit)
	}
	if work.Attribution.PR != "Generated with Claude Code" {
		t.Errorf("pr = %q", work.Attribution.PR)
	}
}

func TestLoadConfigWithEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	content := `
[profiles.vertex]
description = "Vertex AI"

[profiles.vertex.env]
CLAUDE_CODE_USE_VERTEX = "1"
CLOUD_ML_REGION = "europe-west1"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	vertex := cfg.Profiles["vertex"]
	if vertex.Env["CLAUDE_CODE_USE_VERTEX"] != "1" {
		t.Errorf("env CLAUDE_CODE_USE_VERTEX = %q", vertex.Env["CLAUDE_CODE_USE_VERTEX"])
	}
	if vertex.Env["CLOUD_ML_REGION"] != "europe-west1" {
		t.Errorf("env CLOUD_ML_REGION = %q", vertex.Env["CLOUD_ML_REGION"])
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.toml")
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestLoadConfigNoProfiles(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	if err := os.WriteFile(configPath, []byte(`source_dir = "~/.claude"`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("expected error for config with no profiles")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	content := `
[profiles.test]
description = "Test"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	home, _ := os.UserHomeDir()
	expectedSource := filepath.Join(home, ".claude")
	expectedBin := filepath.Join(home, ".local", "bin")

	if cfg.SourceDir != expectedSource {
		t.Errorf("source_dir = %q, want %q", cfg.SourceDir, expectedSource)
	}
	if cfg.BinDir != expectedBin {
		t.Errorf("bin_dir = %q, want %q", cfg.BinDir, expectedBin)
	}
}

func TestProfilesBaseDir(t *testing.T) {
	got := ProfilesBaseDir("/home/user/.claude-profiles/config.toml")
	if got != "/home/user/.claude-profiles" {
		t.Errorf("ProfilesBaseDir = %q, want /home/user/.claude-profiles", got)
	}
}
