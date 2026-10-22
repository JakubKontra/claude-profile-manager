package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloudConflictPreservesWholePlanAndBaseline(t *testing.T) {
	app, _ := testAppAt(t, "")
	root := t.TempDir()
	t.Setenv("HOME", root)
	repo, live := filepath.Join(root, "repo"), filepath.Join(root, "live")
	cfg := &Config{SourceDir: live}
	mustWrite(t, filepath.Join(repo, "commands", "shared.md"), "base")
	if err := app.DistributeSyncFiles(repo, cfg); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(cloudStatePath(repo))
	mustWrite(t, filepath.Join(live, "commands", "shared.md"), "local")
	mustWrite(t, filepath.Join(live, "commands", "new-local.md"), "new")
	mustWrite(t, filepath.Join(repo, "commands", "shared.md"), "remote")
	mustWrite(t, filepath.Join(repo, "commands", "new-remote.md"), "new")
	if err := app.DistributeSyncFiles(repo, cfg); err == nil {
		t.Fatal("conflict accepted")
	}
	after, _ := os.ReadFile(cloudStatePath(repo))
	if string(before) != string(after) {
		t.Fatal("failed sync advanced baseline")
	}
	if fileExistsAt(filepath.Join(live, "commands", "new-remote.md")) {
		t.Fatal("partial apply on conflict")
	}
	mustWrite(t, filepath.Join(live, "commands", "shared.md"), "base")
	if err := app.DistributeSyncFiles(repo, cfg); err != nil {
		t.Fatal(err)
	}
	if !fileExistsAt(filepath.Join(live, "commands", "new-local.md")) {
		t.Fatal("local addition deleted")
	}
	backups, _ := filepath.Glob(filepath.Join(root, ".cpm", "backups", "*", "commands", "shared.md"))
	if len(backups) != 1 {
		t.Fatalf("missing backup: %v", backups)
	}
	data, _ := os.ReadFile(backups[0])
	if string(data) != "base" {
		t.Fatal("wrong backup")
	}
}

func TestCloudFirstPullConflictAndLastRemoteFileDeletion(t *testing.T) {
	app, _ := testAppAt(t, "")
	root := t.TempDir()
	t.Setenv("HOME", root)
	repo, live := filepath.Join(root, "repo"), filepath.Join(root, "live")
	cfg := &Config{SourceDir: live}
	path := filepath.Join("commands", "only.md")
	mustWrite(t, filepath.Join(repo, path), "remote")
	mustWrite(t, filepath.Join(live, path), "local")
	if err := app.DistributeSyncFiles(repo, cfg); err == nil {
		t.Fatal("first pull overwrote local file")
	}
	mustWrite(t, filepath.Join(live, path), "remote")
	if err := app.DistributeSyncFiles(repo, cfg); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(repo, "commands"))
	if err := app.DistributeSyncFiles(repo, cfg); err != nil {
		t.Fatal(err)
	}
	if fileExistsAt(filepath.Join(live, path)) {
		t.Fatal("tracked remote deletion not applied")
	}
}
