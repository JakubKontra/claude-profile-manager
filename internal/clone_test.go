package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloneProfile(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profilesBase := filepath.Join(tmpDir, "profiles")
	srcProfile := filepath.Join(profilesBase, "original")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(srcProfile, 0o755)

	// Create source files
	os.WriteFile(filepath.Join(srcProfile, "settings.json"), []byte(`{"test": true}`), 0o644)
	os.WriteFile(filepath.Join(srcProfile, "CLAUDE.md"), []byte("# Claude"), 0o644)

	// Create source dirs for symlinks
	for _, dir := range []string{"skills", "plugins"} {
		os.MkdirAll(filepath.Join(sourceDir, dir), 0o755)
	}

	cfg := &Config{
		SourceDir: sourceDir,
		Profiles: map[string]*Profile{
			"original": {Description: "Original"},
		},
	}

	err := CloneProfile("original", "cloned", profilesBase, sourceDir, cfg)
	if err != nil {
		t.Fatalf("CloneProfile failed: %v", err)
	}

	clonedDir := filepath.Join(profilesBase, "cloned")

	// Check copied files
	data, err := os.ReadFile(filepath.Join(clonedDir, "settings.json"))
	if err != nil {
		t.Fatal("settings.json not cloned")
	}
	if string(data) != `{"test": true}` {
		t.Errorf("cloned settings.json = %q", string(data))
	}

	// Check symlinks point to source dir (not the original profile)
	for _, dir := range []string{"skills", "plugins"} {
		target, err := os.Readlink(filepath.Join(clonedDir, dir))
		if err != nil {
			t.Errorf("%s should be a symlink", dir)
			continue
		}
		expected := filepath.Join(sourceDir, dir)
		if target != expected {
			t.Errorf("%s symlink = %q, want %q", dir, target, expected)
		}
	}
}

func TestCloneProfileSourceNotInstalled(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		Profiles: map[string]*Profile{
			"missing": {Description: "Missing"},
		},
	}

	err := CloneProfile("missing", "new", tmpDir, tmpDir, cfg)
	if err == nil {
		t.Error("expected error for uninstalled source profile")
	}
}

func TestCloneProfileTargetExists(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "source"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "target"), 0o755)

	cfg := &Config{
		Profiles: map[string]*Profile{
			"source": {Description: "Source"},
		},
	}

	err := CloneProfile("source", "target", tmpDir, tmpDir, cfg)
	if err == nil {
		t.Error("expected error when target profile already exists")
	}
}

func TestCloneProfileDoesNotCopyCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profilesBase := filepath.Join(tmpDir, "profiles")
	srcProfile := filepath.Join(profilesBase, "original")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(srcProfile, 0o755)

	// Create credentials in source profile
	os.WriteFile(filepath.Join(srcProfile, ".credentials.json"), []byte(`{"token": "secret"}`), 0o644)
	os.WriteFile(filepath.Join(srcProfile, "settings.json"), []byte(`{}`), 0o644)

	cfg := &Config{
		SourceDir: sourceDir,
		Profiles: map[string]*Profile{
			"original": {Description: "Original"},
		},
	}

	CloneProfile("original", "cloned", profilesBase, sourceDir, cfg)

	clonedDir := filepath.Join(profilesBase, "cloned")
	if _, err := os.Stat(filepath.Join(clonedDir, ".credentials.json")); !os.IsNotExist(err) {
		t.Error("credentials should NOT be cloned")
	}
}
