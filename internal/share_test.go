package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEffectiveShareDirsInheritance(t *testing.T) {
	join := func(d []string) string { return strings.Join(d, ",") }

	if got := EffectiveShareDirs(&Config{}, &Profile{}); join(got) != "commands,skills,agents,plugins,projects" {
		t.Errorf("default: %v", got)
	}
	cfg := &Config{Share: []string{"skills", "projects"}}
	if got := EffectiveShareDirs(cfg, &Profile{}); join(got) != "skills,projects" {
		t.Errorf("global: %v", got)
	}
	if got := EffectiveShareDirs(cfg, &Profile{Share: []string{"plans"}}); join(got) != "plans" {
		t.Errorf("profile override: %v", got)
	}
	if got := EffectiveShareDirs(cfg, &Profile{Isolate: []string{"projects"}}); join(got) != "skills" {
		t.Errorf("isolate: %v", got)
	}
	if got := EffectiveShareDirs(nil, &Profile{Share: []string{}}); len(got) != 0 {
		t.Errorf("explicit empty share must share nothing: %v", got)
	}
	if got := EffectiveShareDirs(nil, nil); join(got) != "commands,skills,agents,plugins,projects" {
		t.Errorf("nil profile: %v", got)
	}
}

func TestSetupProfileIsolateReplacesSymlinkWithDir(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	for _, d := range []string{"skills", "projects"} {
		mustMkdir(t, filepath.Join(env.SourceDir, d))
	}
	profileDir := filepath.Join(env.profilesBase(), "work")
	buf := captureOutput(t)

	if err := SetupProfile("work", profileDir, env.SourceDir, defaultShareDirs, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(filepath.Join(profileDir, "projects")); err != nil {
		t.Fatal("projects should be a symlink initially")
	}

	buf.Reset()
	dirs := EffectiveShareDirs(env.load(t), &Profile{Isolate: []string{"projects"}})
	if err := SetupProfile("work", profileDir, env.SourceDir, dirs, false); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(filepath.Join(profileDir, "projects"))
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		t.Errorf("projects should now be a real directory: %v %v", st, err)
	}
	if _, err := os.Readlink(filepath.Join(profileDir, "skills")); err != nil {
		t.Error("skills must stay shared")
	}
	if !strings.Contains(buf.String(), "isolated projects/") {
		t.Errorf("unexpected output: %q", buf.String())
	}

	// Sharing it again does not delete the (now real) directory.
	mustWrite(t, filepath.Join(profileDir, "projects", "data.txt"), "keep")
	buf.Reset()
	if err := SetupProfile("work", profileDir, env.SourceDir, defaultShareDirs, false); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(profileDir, "projects", "data.txt")) {
		t.Error("real directory must never be deleted")
	}
	if !strings.Contains(buf.String(), "skipped projects/") {
		t.Errorf("expected skip message: %q", buf.String())
	}
}

func TestSetupProfileLeavesForeignSymlinks(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	profileDir := filepath.Join(env.profilesBase(), "work")
	mustMkdir(t, profileDir)
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(profileDir, "mine")); err != nil {
		t.Fatal(err)
	}
	captureOutput(t)
	if err := SetupProfile("work", profileDir, env.SourceDir, defaultShareDirs, false); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(profileDir, "mine")); err != nil || target != other {
		t.Error("a symlink not pointing into the source dir must be left alone")
	}
}

func TestLoadConfigRejectsBadShareEntry(t *testing.T) {
	for _, body := range []string{
		"share = [\"../x\"]\n[profiles.a]\n",
		"[profiles.a]\nisolate = [\"a/b\"]\n",
		"[profiles.a]\nshare = [\".\"]\n",
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte(body), 0o644)
		if _, err := LoadConfig(path); err == nil {
			t.Errorf("config %q should be rejected", body)
		}
	}
}

func TestInstallProfilesHonoursShareConfig(t *testing.T) {
	env := newTestEnv(t, `share = ["skills"]
[profiles.a]
[profiles.b]
share = ["plans"]
`)
	for _, d := range []string{"skills", "plans", "projects"} {
		mustMkdir(t, filepath.Join(env.SourceDir, d))
	}
	captureOutput(t)
	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(env.profilesBase(), "a")
	b := filepath.Join(env.profilesBase(), "b")
	if _, err := os.Readlink(filepath.Join(a, "skills")); err != nil {
		t.Error("a should share skills")
	}
	if fileExists(filepath.Join(a, "projects")) || fileExists(filepath.Join(a, "plans")) {
		t.Error("a should not share projects or plans")
	}
	if _, err := os.Readlink(filepath.Join(b, "plans")); err != nil {
		t.Error("b should share plans")
	}
	if fileExists(filepath.Join(b, "skills")) {
		t.Error("b's share list replaces the global one")
	}
}
