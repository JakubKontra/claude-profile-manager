package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		value := fmt.Sprintf("%v %d", info.Mode(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += string(data)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += link
		}
		result[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInstallDryRunIsReadOnlyAndMatchesApply(t *testing.T) {
	env := newTestEnv(t, "[profiles.work]\n[cloud]\nauto_push = true\nauto_pull_on_install = true\n")
	t.Setenv("HOME", env.Root)
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"env":{"API_TOKEN":"private-secret"},"theme":"dark"}`)
	mustMkdir(t, filepath.Join(env.SourceDir, "skills"))
	mustWrite(t, filepath.Join(env.Root, ".claude.json"), `{"hasCompletedOnboarding":true,"mcpServers":{"api":{"headers":{"Authorization":"Bearer hidden"}}}}`)
	cfg := env.load(t)
	app, buf := testAppAt(t, env.ConfigPath)
	before := snapshotTree(t, env.Root)
	if err := app.InstallProfiles(cfg, InstallOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshotTree(t, env.Root)) {
		t.Fatal("dry run changed filesystem")
	}
	if strings.Contains(buf.String(), "private-secret") || strings.Contains(buf.String(), "Bearer hidden") {
		t.Fatal("preview leaked secret")
	}
	for _, expected := range []string{"@@", "symlink", ".claude.json", "claude-work", "cloud auto-pull/auto-push skipped"} {
		if !strings.Contains(buf.String(), expected) {
			t.Fatalf("preview missing %s", expected)
		}
	}
	plan, err := app.PlanInstall(cfg, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.ApplyInstall(plan); err != nil {
		t.Fatal(err)
	}
	for _, a := range plan {
		if a.Kind == "write" {
			data, _ := os.ReadFile(a.Path)
			if string(data) != string(a.Data) {
				t.Fatalf("plan mismatch: %s", a.Path)
			}
		}
	}
	again, err := app.PlanInstall(cfg, InstallOptions{})
	if err != nil || len(again) != 0 {
		t.Fatalf("not idempotent: %v %v", again, err)
	}
}

func TestInstallPreviewSyncForceAndForeignWrapper(t *testing.T) {
	env := newTestEnv(t, "[profiles.work]\n")
	t.Setenv("HOME", env.Root)
	app, _ := testAppAt(t, env.ConfigPath)
	mustWrite(t, filepath.Join(env.SourceDir, "CLAUDE.md"), "base")
	cfg := env.load(t)
	if err := app.InstallProfiles(cfg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(env.profilesBase(), "work", "CLAUDE.md"), "local")
	before := snapshotTree(t, env.Root)
	if err := app.InstallProfiles(cfg, InstallOptions{DryRun: true, Sync: true}); err == nil {
		t.Fatal("divergence accepted")
	}
	if err := app.InstallProfiles(cfg, InstallOptions{DryRun: true, Sync: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshotTree(t, env.Root)) {
		t.Fatal("force preview mutated files")
	}
	mustWrite(t, filepath.Join(env.BinDir, "claude-work"), "foreign")
	if _, err := app.PlanInstall(cfg, InstallOptions{}); err == nil {
		t.Fatal("foreign wrapper accepted")
	}
}
