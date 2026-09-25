package internal

import (
	"os"
	"path/filepath"
	"strings"
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
		copyFile(srcPath, dst)
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

func TestSaveCloudRemoteWithCommentedHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte(userConfig), 0o644)

	if err := saveCloudRemote(path, "git@github.com:me/settings.git"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadCloudConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cloud == nil || cfg.Cloud.Remote != "git@github.com:me/settings.git" {
		data, _ := os.ReadFile(path)
		t.Errorf("remote not saved:\n%s", data)
	}

	// Update in place, no duplicate key.
	if err := saveCloudRemote(path, "git@github.com:me/other.git"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "\nremote = ") != 1 {
		t.Errorf("remote duplicated:\n%s", data)
	}
	cfg, _ = LoadCloudConfig(path)
	if cfg.Cloud.Remote != "git@github.com:me/other.git" {
		t.Errorf("remote not updated: %s", cfg.Cloud.Remote)
	}
}

func TestSaveCloudRemoteCreatesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := saveCloudRemote(path, "url"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadCloudConfig(path)
	if err != nil || cfg.Cloud == nil || cfg.Cloud.Remote != "url" {
		t.Errorf("config not created: %v %+v", err, cfg)
	}
}

func TestMergeConfigTOMLAddsNewProfiles(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "config.toml")
	pulled := filepath.Join(dir, "pulled.toml")
	os.WriteFile(local, []byte(userConfig), 0o644)
	os.WriteFile(pulled, []byte(`source_dir = "/elsewhere"
[profiles.work]
description = "changed remotely"
[profiles.client]
description = "Client"
model = "opus"
add_dirs = ["~/c"]
[profiles.client.env]
K = "v"
[cloud]
remote = "git@example:x.git"
`), 0o644)
	captureOutput(t)

	if err := mergeConfigTOML(pulled, local); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(local)
	if err != nil {
		t.Fatal(err)
	}
	client := cfg.Profiles["client"]
	if client == nil || client.Model != "opus" || client.Env["K"] != "v" || len(client.AddDirs) != 1 {
		t.Errorf("client profile not merged fully: %+v", client)
	}
	if cfg.Profiles["work"].Description != "Company team subscription" {
		t.Error("existing profile must not be overwritten")
	}
	if cfg.SourceDir != ExpandPath("~/.claude") {
		t.Errorf("local source_dir must be preserved, got %s", cfg.SourceDir)
	}
	if cfg.Cloud == nil || cfg.Cloud.Remote != "git@example:x.git" {
		t.Errorf("cloud section not merged: %+v", cfg.Cloud)
	}

	// Second merge is a no-op.
	before, _ := os.ReadFile(local)
	if err := mergeConfigTOML(pulled, local); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(local)
	if string(before) != string(after) {
		t.Error("merge must be idempotent")
	}
}
