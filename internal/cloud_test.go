package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGatherSyncFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0o755)

	// Create some test files
	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{"test": true}`), 0o644)
	os.WriteFile(filepath.Join(sourceDir, "CLAUDE.md"), []byte("# Test"), 0o644)

	// Create commands dir
	cmdDir := filepath.Join(sourceDir, "commands")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "test.md"), []byte("test command"), 0o644)

	cfg := &Config{SourceDir: sourceDir}

	// Use a config path that doesn't exist (no cpm/config.toml)
	files, err := GatherSyncFiles(cfg, filepath.Join(tmpDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := files["settings.json"]; !ok {
		t.Error("expected settings.json in gathered files")
	}
	if _, ok := files["CLAUDE.md"]; !ok {
		t.Error("expected CLAUDE.md in gathered files")
	}
	if _, ok := files["commands/test.md"]; !ok {
		t.Error("expected commands/test.md in gathered files")
	}
	// settings.local.json should not be present (doesn't exist)
	if _, ok := files["settings.local.json"]; ok {
		t.Error("did not expect settings.local.json (file doesn't exist)")
	}
}

func TestGatherSyncFilesWithExclude(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0o755)

	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(sourceDir, "CLAUDE.md"), []byte("# Test"), 0o644)

	cfg := &Config{
		SourceDir: sourceDir,
		Cloud: &CloudConfig{
			Exclude: []string{"CLAUDE.md"},
		},
	}

	files, err := GatherSyncFiles(cfg, filepath.Join(tmpDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := files["CLAUDE.md"]; ok {
		t.Error("CLAUDE.md should be excluded")
	}
	if _, ok := files["settings.json"]; !ok {
		t.Error("settings.json should still be included")
	}
}

func TestGatherSyncFilesDirExclude(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	cmdDir := filepath.Join(sourceDir, "commands")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "test.md"), []byte("test"), 0o644)

	cfg := &Config{
		SourceDir: sourceDir,
		Cloud: &CloudConfig{
			Exclude: []string{"commands/"},
		},
	}

	files, err := GatherSyncFiles(cfg, filepath.Join(tmpDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := files["commands/test.md"]; ok {
		t.Error("commands/ directory should be excluded")
	}
}

func TestDistributeSyncFilesRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	repoDir := filepath.Join(tmpDir, "repo")
	restoreDir := filepath.Join(tmpDir, "restore")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(repoDir, 0o755)
	os.MkdirAll(restoreDir, 0o755)

	// Create source files
	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{"key": "value"}`), 0o644)

	cmdDir := filepath.Join(sourceDir, "commands")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "hello.md"), []byte("hello world"), 0o644)

	// Gather files
	cfg := &Config{SourceDir: sourceDir}
	files, _ := GatherSyncFiles(cfg, filepath.Join(tmpDir, "config.toml"))

	// Copy to repo
	for repoPath, srcPath := range files {
		dst := filepath.Join(repoDir, repoPath)
		os.MkdirAll(filepath.Dir(dst), 0o755)
		cloudCopyFile(srcPath, dst)
	}

	// Distribute from repo to restore dir
	restoreCfg := &Config{SourceDir: restoreDir}
	DistributeSyncFiles(repoDir, restoreCfg)

	// Check files were restored
	data, err := os.ReadFile(filepath.Join(restoreDir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not restored: %v", err)
	}
	if string(data) != `{"key": "value"}` {
		t.Errorf("settings.json content mismatch: %s", data)
	}

	data, err = os.ReadFile(filepath.Join(restoreDir, "commands", "hello.md"))
	if err != nil {
		t.Fatalf("commands/hello.md not restored: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("commands/hello.md content mismatch: %s", data)
	}
}

func TestFilesAreDifferent(t *testing.T) {
	tmpDir := t.TempDir()

	fileA := filepath.Join(tmpDir, "a.txt")
	fileB := filepath.Join(tmpDir, "b.txt")
	fileC := filepath.Join(tmpDir, "c.txt")

	os.WriteFile(fileA, []byte("same"), 0o644)
	os.WriteFile(fileB, []byte("same"), 0o644)
	os.WriteFile(fileC, []byte("different"), 0o644)

	if filesAreDifferent(fileA, fileB) {
		t.Error("identical files reported as different")
	}
	if !filesAreDifferent(fileA, fileC) {
		t.Error("different files reported as same")
	}
	if !filesAreDifferent(fileA, filepath.Join(tmpDir, "nonexistent")) {
		t.Error("existing vs nonexistent should be different")
	}
}

func TestIsExcluded(t *testing.T) {
	cfg := &Config{
		Cloud: &CloudConfig{
			Exclude: []string{"CLAUDE.md", "commands/"},
		},
	}

	if !isExcluded("CLAUDE.md", cfg) {
		t.Error("CLAUDE.md should be excluded")
	}
	if !isExcluded("commands/", cfg) {
		t.Error("commands/ should be excluded")
	}
	if isExcluded("settings.json", cfg) {
		t.Error("settings.json should not be excluded")
	}

	// nil cloud config
	nilCfg := &Config{}
	if isExcluded("CLAUDE.md", nilCfg) {
		t.Error("nothing should be excluded with nil cloud config")
	}
}

func TestCleanDeletedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo", "commands")
	srcDir := filepath.Join(tmpDir, "src", "commands")

	os.MkdirAll(repoDir, 0o755)
	os.MkdirAll(srcDir, 0o755)

	// File exists in both
	os.WriteFile(filepath.Join(repoDir, "keep.md"), []byte("keep"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "keep.md"), []byte("keep"), 0o644)

	// File exists only in repo (was deleted from source)
	os.WriteFile(filepath.Join(repoDir, "deleted.md"), []byte("deleted"), 0o644)

	cleanDeletedFiles(repoDir, srcDir)

	if _, err := os.Stat(filepath.Join(repoDir, "keep.md")); os.IsNotExist(err) {
		t.Error("keep.md should still exist in repo")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "deleted.md")); !os.IsNotExist(err) {
		t.Error("deleted.md should have been removed from repo")
	}
}
