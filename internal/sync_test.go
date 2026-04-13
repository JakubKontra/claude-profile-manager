package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchAttribution(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	os.WriteFile(settingsPath, []byte(`{"env": {}}`), 0o644)

	attr := &Attribution{
		Commit: "Co-Authored-By: Claude",
		PR:     "Generated with Claude Code",
	}

	if err := PatchAttribution(dir, attr); err != nil {
		t.Fatalf("PatchAttribution failed: %v", err)
	}

	data, _ := os.ReadFile(settingsPath)
	var settings map[string]any
	json.Unmarshal(data, &settings)

	attrMap, ok := settings["attribution"].(map[string]any)
	if !ok {
		t.Fatal("attribution not found in settings")
	}
	if attrMap["commit"] != "Co-Authored-By: Claude" {
		t.Errorf("commit = %v", attrMap["commit"])
	}
	if attrMap["pr"] != "Generated with Claude Code" {
		t.Errorf("pr = %v", attrMap["pr"])
	}
}

func TestPatchAttributionNil(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	original := `{"env": {}}`
	os.WriteFile(settingsPath, []byte(original), 0o644)

	// nil attribution should be a no-op
	if err := PatchAttribution(dir, nil); err != nil {
		t.Fatalf("PatchAttribution(nil) failed: %v", err)
	}

	data, _ := os.ReadFile(settingsPath)
	if string(data) != original {
		t.Error("nil attribution should not modify settings.json")
	}
}

func TestPatchAttributionIdempotent(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	os.WriteFile(settingsPath, []byte(`{"attribution": {"commit": "test"}}`), 0o644)

	attr := &Attribution{Commit: "test"}
	PatchAttribution(dir, attr)

	info1, _ := os.Stat(settingsPath)

	PatchAttribution(dir, attr)

	info2, _ := os.Stat(settingsPath)

	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("idempotent patch should not rewrite file")
	}
}

func TestCheckDivergenceInSync(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profileDir := filepath.Join(tmpDir, "profiles", "test")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(profileDir, 0o755)

	content := `{"env": {}}`
	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(content), 0o644)
	os.WriteFile(filepath.Join(profileDir, "settings.json"), []byte(content), 0o644)

	cfg := &Config{
		SourceDir: sourceDir,
		Profiles: map[string]*Profile{
			"test": {Description: "Test"},
		},
	}

	diverged := CheckDivergence(cfg, filepath.Join(tmpDir, "profiles"))
	if len(diverged) != 0 {
		t.Errorf("expected 0 diverged files, got %d", len(diverged))
	}
}

func TestCheckDivergenceDetectsChanges(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	profileDir := filepath.Join(tmpDir, "profiles", "test")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(profileDir, 0o755)

	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{"env": {}}`), 0o644)
	os.WriteFile(filepath.Join(profileDir, "settings.json"), []byte(`{"env": {}, "extra": true}`), 0o644)

	cfg := &Config{
		SourceDir: sourceDir,
		Profiles: map[string]*Profile{
			"test": {Description: "Test"},
		},
	}

	diverged := CheckDivergence(cfg, filepath.Join(tmpDir, "profiles"))
	if len(diverged) != 1 {
		t.Errorf("expected 1 diverged file, got %d", len(diverged))
	}
}

func TestSyncMCPServersNoSourceFile(t *testing.T) {
	dir := t.TempDir()
	// Should not fail when ~/.claude.json doesn't exist
	// We can't easily test this without mocking the home dir,
	// but at least verify it doesn't panic
	err := SyncMCPServers(dir)
	if err != nil {
		t.Errorf("SyncMCPServers should not error on missing source: %v", err)
	}
}

func TestPluralS(t *testing.T) {
	if s := pluralS(0); s != "s" {
		t.Errorf("pluralS(0) = %q, want %q", s, "s")
	}
	if s := pluralS(1); s != "" {
		t.Errorf("pluralS(1) = %q, want %q", s, "")
	}
	if s := pluralS(5); s != "s" {
		t.Errorf("pluralS(5) = %q, want %q", s, "s")
	}
}
