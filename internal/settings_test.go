package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeepMergeReplacesArraysMergesMaps(t *testing.T) {
	dst := map[string]any{
		"permissions": map[string]any{"allow": []any{"a"}, "deny": []any{"x"}},
		"model":       "opus",
		"keep":        true,
	}
	src := map[string]any{
		"permissions": map[string]any{"allow": []any{"b"}},
		"model":       "sonnet",
		"new":         map[string]any{"k": 1},
	}
	got := deepMerge(dst, src)
	perms := got["permissions"].(map[string]any)
	if len(perms["allow"].([]any)) != 1 || perms["allow"].([]any)[0] != "b" {
		t.Errorf("arrays must be replaced, got %v", perms["allow"])
	}
	if perms["deny"] == nil || got["keep"] != true || got["model"] != "sonnet" || got["new"] == nil {
		t.Errorf("unexpected merge result: %v", got)
	}
}

func TestPatchSettingsDottedKeysAndIdempotent(t *testing.T) {
	env := newTestEnv(t, `[profiles.work]
[profiles.work.settings]
effortLevel = "high"
permissions.allow = ["Bash(gh *)"]
[profiles.work.attribution]
commit = "Co-Authored-By: X"
`)
	profileDir := filepath.Join(env.profilesBase(), "work")
	mustWrite(t, filepath.Join(profileDir, "settings.json"), `{"model": "opus", "permissions": {"deny": ["rm"]}, "attribution": {"commit": "old"}}`)
	buf := captureOutput(t)

	p := env.load(t).Profiles["work"]
	if err := PatchSettings(profileDir, EffectiveSettingsOverrides(p)); err != nil {
		t.Fatal(err)
	}
	got := readJSON(t, filepath.Join(profileDir, "settings.json"))
	if got["effortLevel"] != "high" || got["model"] != "opus" {
		t.Errorf("top-level merge wrong: %v", got)
	}
	perms := got["permissions"].(map[string]any)
	if perms["deny"] == nil || perms["allow"] == nil {
		t.Errorf("dotted key should land as nested map and keep siblings: %v", perms)
	}
	if got["attribution"].(map[string]any)["commit"] != "Co-Authored-By: X" {
		t.Errorf("attribution not applied: %v", got["attribution"])
	}
	if !strings.Contains(buf.String(), "patched settings.json") {
		t.Errorf("unexpected output: %q", buf.String())
	}

	buf.Reset()
	if err := PatchSettings(profileDir, EffectiveSettingsOverrides(p)); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "" {
		t.Errorf("second patch must be a no-op: %q", buf.String())
	}
}

func TestEffectiveSettingsAttributionWins(t *testing.T) {
	p := &Profile{
		Settings:    map[string]any{"attribution": map[string]any{"commit": "from settings"}, "x": 1},
		Attribution: &Attribution{Commit: "from attribution"},
	}
	got := EffectiveSettingsOverrides(p)
	if got["attribution"].(map[string]any)["commit"] != "from attribution" || got["x"] != 1 {
		t.Errorf("unexpected overrides: %v", got)
	}
	if p.Settings["attribution"].(map[string]any)["commit"] != "from settings" {
		t.Error("profile settings must not be mutated")
	}
	if EffectiveSettingsOverrides(&Profile{}) != nil {
		t.Error("no overrides should be nil")
	}
}

func TestCheckDivergenceWithSettingsOverrides(t *testing.T) {
	env := newTestEnv(t, `[profiles.work]
[profiles.work.settings]
effortLevel = "high"
`)
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"model": "opus"}`)
	captureOutput(t)
	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	// The patched file must not count as diverged.
	if d := CheckDivergence(env.load(t), env.profilesBase()); len(d) != 0 {
		t.Errorf("overrides reported as divergence: %v", d)
	}
	mustWrite(t, filepath.Join(env.profilesBase(), "work", "settings.json"), `{"model": "opus", "effortLevel": "high", "local": 1}`)
	if d := CheckDivergence(env.load(t), env.profilesBase()); len(d) != 1 {
		t.Errorf("real local change not detected: %v", d)
	}
}

func TestCheckDivergenceUsesBaseline(t *testing.T) {
	env := newTestEnv(t, "[profiles.work]\n")
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), "{\n  \"v\": 1\n}\n")
	captureOutput(t)
	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(env.profilesBase(), "work", ".cpm", "baseline", "settings.json")) {
		t.Fatal("baseline not recorded")
	}

	// Upstream changes, profile untouched: not diverged, sync overwrites.
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), "{\n  \"v\": 2\n}\n")
	if d := CheckDivergence(env.load(t), env.profilesBase()); len(d) != 0 {
		t.Errorf("upstream change must not count as local divergence: %v", d)
	}
	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{Sync: true}); err != nil {
		t.Fatalf("sync should succeed: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(env.profilesBase(), "work", "settings.json"))
	if string(data) != "{\n  \"v\": 2\n}\n" {
		t.Errorf("profile not synced: %q", data)
	}

	// Local edit: diverged, sync refuses without --force.
	mustWrite(t, filepath.Join(env.profilesBase(), "work", "settings.json"), "{\n  \"v\": 2,\n  \"mine\": true\n}\n")
	if d := CheckDivergence(env.load(t), env.profilesBase()); len(d) != 1 {
		t.Errorf("local edit not detected: %v", d)
	}
	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{Sync: true}); err == nil {
		t.Error("sync must refuse to overwrite local edits")
	}
}
