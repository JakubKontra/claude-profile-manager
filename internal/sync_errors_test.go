package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDistributeReportsPartialFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	mustWrite(t, src, "valid")
	good := filepath.Join(dir, "good")
	app, _ := testAppAt(t, "")
	err := app.ApplyDistribute([]DistributeAction{
		{Kind: "create", RepoPath: "missing", Source: filepath.Join(dir, "absent"), Dest: filepath.Join(dir, "bad")},
		{Kind: "create", RepoPath: "good", Source: src, Dest: good},
		{Kind: "merge", RepoPath: "config", Source: src, Dest: filepath.Join(dir, "config")},
	})
	if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "config") {
		t.Fatalf("missing failures: %v", err)
	}
	data, _ := os.ReadFile(good)
	if string(data) != "valid" {
		t.Fatal("successful action not applied")
	}
}

func TestStageSyncFilesFailsOnCopyError(t *testing.T) {
	if err := stageSyncFiles(t.TempDir(), &Config{}, map[string]string{"missing": filepath.Join(t.TempDir(), "absent")}); err == nil {
		t.Fatal("copy failure hidden")
	}
}
