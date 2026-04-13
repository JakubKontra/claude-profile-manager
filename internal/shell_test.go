package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateUseOutput(t *testing.T) {
	profile := &Profile{
		Description: "Test",
		Env: map[string]string{
			"CUSTOM_VAR": "value",
		},
	}

	out := GenerateUseOutput("test", "/profiles/test", profile)

	mustContain := []string{
		`CLAUDE_CONFIG_DIR="/profiles/test"`,
		`CLAUDE_PROFILE="test"`,
		`CUSTOM_VAR="value"`,
		"Switched to profile: test",
	}

	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("use output missing %q", s)
		}
	}
}

func TestDetectProfileFile(t *testing.T) {
	// Create nested directory structure with .claude-profile at root
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	subDir := filepath.Join(projectDir, "src", "components")

	os.MkdirAll(subDir, 0o755)
	os.WriteFile(filepath.Join(projectDir, ".claude-profile"), []byte("work\n"), 0o644)

	// Should find profile from nested subdirectory
	name, err := DetectProfileFile(subDir)
	if err != nil {
		t.Fatalf("DetectProfileFile failed: %v", err)
	}
	if name != "work" {
		t.Errorf("detected profile = %q, want %q", name, "work")
	}
}

func TestDetectProfileFileInCurrentDir(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, ".claude-profile"), []byte("personal"), 0o644)

	name, err := DetectProfileFile(tmpDir)
	if err != nil {
		t.Fatalf("DetectProfileFile failed: %v", err)
	}
	if name != "personal" {
		t.Errorf("detected profile = %q, want %q", name, "personal")
	}
}

func TestDetectProfileFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := DetectProfileFile(tmpDir)
	if err == nil {
		t.Error("expected error when no .claude-profile exists")
	}
}

func TestDetectProfileFileEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, ".claude-profile"), []byte("  \n"), 0o644)

	_, err := DetectProfileFile(tmpDir)
	if err == nil {
		t.Error("expected error for empty .claude-profile")
	}
}

func TestLinkProfile(t *testing.T) {
	tmpDir := t.TempDir()

	if err := LinkProfile(tmpDir, "work"); err != nil {
		t.Fatalf("LinkProfile failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude-profile"))
	if err != nil {
		t.Fatal(".claude-profile not created")
	}
	if strings.TrimSpace(string(data)) != "work" {
		t.Errorf(".claude-profile content = %q", string(data))
	}
}

func TestLinkProfileUpdatesGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("node_modules\n"), 0o644)

	LinkProfile(tmpDir, "work")

	data, _ := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if !strings.Contains(string(data), ".claude-profile") {
		t.Error(".gitignore should contain .claude-profile")
	}
	if !strings.Contains(string(data), "node_modules") {
		t.Error(".gitignore should preserve existing entries")
	}
}

func TestLinkProfileGitignoreIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(".claude-profile\n"), 0o644)

	LinkProfile(tmpDir, "work")

	data, _ := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	count := strings.Count(string(data), ".claude-profile")
	if count != 1 {
		t.Errorf(".claude-profile appears %d times in .gitignore, want 1", count)
	}
}

func TestLinkProfileNoGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	// Should not fail if .gitignore doesn't exist
	if err := LinkProfile(tmpDir, "work"); err != nil {
		t.Fatalf("LinkProfile failed without .gitignore: %v", err)
	}

	// .claude-profile should still be created
	if _, err := os.Stat(filepath.Join(tmpDir, ".claude-profile")); err != nil {
		t.Error(".claude-profile should exist even without .gitignore")
	}
}

func TestUnlinkProfile(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, ".claude-profile"), []byte("work"), 0o644)

	if err := UnlinkProfile(tmpDir); err != nil {
		t.Fatalf("UnlinkProfile failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".claude-profile")); !os.IsNotExist(err) {
		t.Error(".claude-profile should be deleted")
	}
}

func TestUnlinkProfileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	err := UnlinkProfile(tmpDir)
	if err == nil {
		t.Error("expected error when no .claude-profile exists")
	}
}

func TestGenerateShellHook(t *testing.T) {
	hook := GenerateShellHook()

	mustContain := []string{
		"_cpm_auto_switch",
		".claude-profile",
		"cpm use",
		"chpwd",
		"ZSH_VERSION",
	}

	for _, s := range mustContain {
		if !strings.Contains(hook, s) {
			t.Errorf("hook missing %q", s)
		}
	}
}

func TestPromptStringEmpty(t *testing.T) {
	os.Unsetenv("CLAUDE_PROFILE")
	if p := PromptString(); p != "" {
		t.Errorf("PromptString() = %q, want empty", p)
	}
}

func TestPromptStringSet(t *testing.T) {
	os.Setenv("CLAUDE_PROFILE", "work")
	defer os.Unsetenv("CLAUDE_PROFILE")

	if p := PromptString(); p != "work" {
		t.Errorf("PromptString() = %q, want %q", p, "work")
	}
}

func TestCurrentProfile(t *testing.T) {
	os.Setenv("CLAUDE_PROFILE", "personal")
	defer os.Unsetenv("CLAUDE_PROFILE")

	if p := CurrentProfile(); p != "personal" {
		t.Errorf("CurrentProfile() = %q, want %q", p, "personal")
	}
}

func TestCurrentConfigDir(t *testing.T) {
	os.Setenv("CLAUDE_CONFIG_DIR", "/test/path")
	defer os.Unsetenv("CLAUDE_CONFIG_DIR")

	if d := CurrentConfigDir(); d != "/test/path" {
		t.Errorf("CurrentConfigDir() = %q, want %q", d, "/test/path")
	}
}
