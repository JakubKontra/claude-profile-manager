package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupProfileCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profileDir := filepath.Join(tmpDir, "profiles", "test")

	os.MkdirAll(sourceDir, 0o755)

	if err := SetupProfile("test", profileDir, sourceDir, false); err != nil {
		t.Fatalf("SetupProfile failed: %v", err)
	}

	if _, err := os.Stat(profileDir); os.IsNotExist(err) {
		t.Error("profile directory should exist")
	}
}

func TestSetupProfileCopiesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profileDir := filepath.Join(tmpDir, "profiles", "test")

	os.MkdirAll(sourceDir, 0o755)
	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{"test": true}`), 0o644)
	os.WriteFile(filepath.Join(sourceDir, "CLAUDE.md"), []byte("# Claude"), 0o644)

	if err := SetupProfile("test", profileDir, sourceDir, false); err != nil {
		t.Fatalf("SetupProfile failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		t.Fatal("settings.json not copied")
	}
	if string(data) != `{"test": true}` {
		t.Errorf("settings.json content = %q", string(data))
	}

	data, err = os.ReadFile(filepath.Join(profileDir, "CLAUDE.md"))
	if err != nil {
		t.Fatal("CLAUDE.md not copied")
	}
	if string(data) != "# Claude" {
		t.Errorf("CLAUDE.md content = %q", string(data))
	}
}

func TestSetupProfileDoesNotOverwriteWithoutSync(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profileDir := filepath.Join(tmpDir, "profiles", "test")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(profileDir, 0o755)

	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{"new": true}`), 0o644)
	os.WriteFile(filepath.Join(profileDir, "settings.json"), []byte(`{"old": true}`), 0o644)

	SetupProfile("test", profileDir, sourceDir, false)

	data, _ := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if string(data) != `{"old": true}` {
		t.Error("settings.json should NOT be overwritten without sync")
	}
}

func TestSetupProfileOverwritesWithSync(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profileDir := filepath.Join(tmpDir, "profiles", "test")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(profileDir, 0o755)

	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{"new": true}`), 0o644)
	os.WriteFile(filepath.Join(profileDir, "settings.json"), []byte(`{"old": true}`), 0o644)

	SetupProfile("test", profileDir, sourceDir, true)

	data, _ := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if string(data) != `{"new": true}` {
		t.Error("settings.json should be overwritten with sync")
	}
}

func TestSetupProfileCreatesSymlinks(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profileDir := filepath.Join(tmpDir, "profiles", "test")

	os.MkdirAll(sourceDir, 0o755)

	for _, dir := range []string{"commands", "skills", "plugins"} {
		os.MkdirAll(filepath.Join(sourceDir, dir), 0o755)
	}

	SetupProfile("test", profileDir, sourceDir, false)

	for _, dir := range []string{"commands", "skills", "plugins"} {
		link := filepath.Join(profileDir, dir)
		target, err := os.Readlink(link)
		if err != nil {
			t.Errorf("%s should be a symlink: %v", dir, err)
			continue
		}
		expected := filepath.Join(sourceDir, dir)
		if target != expected {
			t.Errorf("%s symlink target = %q, want %q", dir, target, expected)
		}
	}
}

func TestSetupProfileSkipsSymlinkForRealDir(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profileDir := filepath.Join(tmpDir, "profiles", "test")

	os.MkdirAll(filepath.Join(sourceDir, "commands"), 0o755)
	os.MkdirAll(filepath.Join(profileDir, "commands"), 0o755)

	// Create a file inside the real directory
	os.WriteFile(filepath.Join(profileDir, "commands", "custom.md"), []byte("custom"), 0o644)

	SetupProfile("test", profileDir, sourceDir, false)

	// Should still be a real directory, not a symlink
	_, err := os.Readlink(filepath.Join(profileDir, "commands"))
	if err == nil {
		t.Error("commands should remain a real directory, not be replaced by symlink")
	}

	// Custom file should still exist
	data, _ := os.ReadFile(filepath.Join(profileDir, "commands", "custom.md"))
	if string(data) != "custom" {
		t.Error("custom file should be preserved")
	}
}
