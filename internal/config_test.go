package internal

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	_, err := LoadConfig("/nonexistent/config.toml")
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestLoadConfigNoProfiles(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	got := ProfilesBaseDir("/home/user/.claude-profiles/config.toml")
	if got != "/home/user/.claude-profiles" {
		t.Errorf("ProfilesBaseDir = %q, want /home/user/.claude-profiles", got)
	}
}

func TestValidateProfileName(t *testing.T) {
	t.Parallel()
	valid := []string{"work", "personal", "work-vertex", "client_2", "A1"}
	for _, n := range valid {
		if err := ValidateProfileName(n); err != nil {
			t.Errorf("%q should be valid: %v", n, err)
		}
	}
	invalid := []string{"", "../x", "my profile", "a/b", "auto", "ünï", strings.Repeat("a", 65)}
	for _, n := range invalid {
		if err := ValidateProfileName(n); err == nil {
			t.Errorf("%q should be invalid", n)
		}
	}
}

func TestLoadConfigRejectsInvalidProfileName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte("[profiles.\"../x\"]\ndescription = \"bad\"\n"), 0o644)
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Errorf("expected invalid profile name error, got %v", err)
	}
}

func TestLoadConfigRejectsInvalidEnvName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte("[profiles.work]\n[profiles.work.env]\n\"BAD-NAME\" = \"1\"\n"), 0o644)
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "environment variable") {
		t.Errorf("expected env name error, got %v", err)
	}
}

func TestLoadConfigRecordsUndecodedKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte("sourcedir = \"x\"\n[profiles.work]\nmodl = \"sonnet\"\n"), 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cfg.Undecoded, ",")
	if got != "profiles.work.modl,sourcedir" {
		t.Errorf("Undecoded = %q", got)
	}
}

func TestLoadCloudConfigMissingFileUsesDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := LoadCloudConfig(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourceDir == "" || cfg.BinDir == "" || len(cfg.Profiles) != 0 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestConfigVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		body, wantErr string
	}{
		{"[profiles.a]\n", ""},
		{"version = 1\n[profiles.a]\n", ""},
		{"version = 2\n[profiles.a]\n", "config version 2 needs a newer cpm (this one supports 1); run cpm upgrade"},
		{"version = -1\n[profiles.a]\n", "invalid config version -1"},
	}
	for _, c := range cases {
		path := filepath.Join(t.TempDir(), "config.toml")
		mustWrite(t, path, c.body)
		cfg, err := loadConfig(t.TempDir(), path, true)
		switch {
		case c.wantErr == "" && (err != nil || cfg.Version != 1):
			t.Errorf("%q: version %v, err %v", c.body, cfg, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%q: err = %v, want %q", c.body, err, c.wantErr)
		}
		if c.wantErr == "" && cfg != nil && len(cfg.Undecoded) != 0 {
			t.Errorf("%q: version reported as unknown key: %v", c.body, cfg.Undecoded)
		}
	}
}

func TestConfigMigrationsRunInOrder(t *testing.T) {
	t.Parallel()
	var ran []int
	step := func(from int) func(*Config) error {
		return func(*Config) error {
			ran = append(ran, from)
			return nil
		}
	}
	migrations := map[int]func(*Config) error{1: step(1), 2: step(2)}
	cfg := &Config{}
	if err := migrateConfigTo(cfg, 3, migrations); err != nil || cfg.Version != 3 || !reflect.DeepEqual(ran, []int{1, 2}) {
		t.Errorf("err %v, version %d, ran %v", err, cfg.Version, ran)
	}
	migrations[1] = func(*Config) error { return errors.New("boom") }
	if err := migrateConfigTo(&Config{}, 3, migrations); err == nil || !strings.Contains(err.Error(), "from version 1: boom") {
		t.Errorf("err = %v", err)
	}
}

func TestNewConfigFilesGetVersion(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "")
	if err := os.Remove(env.ConfigPath); err != nil {
		t.Fatal(err)
	}
	if err := env.App.AddProfile("work", AddOptions{NoInstall: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(env.ConfigPath)
	if !strings.HasPrefix(string(data), "version = 1\n") {
		t.Errorf("new config:\n%s", data)
	}
	// Editing an existing file never adds it.
	path := filepath.Join(t.TempDir(), "config.toml")
	mustWrite(t, path, "[profiles.a]\n")
	if err := saveCloudRemote(path, "url"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "version") {
		t.Errorf("existing config got a version:\n%s", data)
	}
}

func TestMergeConfigRejectsNewerVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	local, pulled := filepath.Join(dir, "config.toml"), filepath.Join(dir, "pulled.toml")
	mustWrite(t, local, "[profiles.a]\n")
	mustWrite(t, pulled, "version = 2\n[profiles.a]\n[profiles.b]\n")
	app, _ := testApp(t)
	if err := app.mergeConfigTOML(pulled, local); err == nil || !strings.Contains(err.Error(), "needs a newer cpm") {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(local); string(data) != "[profiles.a]\n" {
		t.Errorf("local config changed:\n%s", data)
	}
}
