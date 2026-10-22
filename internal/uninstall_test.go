package internal

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const zshrc = "export PATH=$HOME/bin:$PATH\neval \"$(cpm hook --default work)\"\n# cpm hook is great\nalias x=y\n"

func uninstallEnv(t *testing.T) testEnv {
	t.Helper()
	env := newTestEnv(t, "[profiles.work]\n[profiles.home]\n")
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(env.Root, ".zshrc"), zshrc)
	mustWrite(t, filepath.Join(env.Root, ".config", "fish", "config.fish"), "cpm hook --shell fish | source\n")
	mustWrite(t, filepath.Join(env.BinDir, "claude-foreign"), "#!/bin/sh\n")
	env.Keychain.login(filepath.Join(env.profilesBase(), "work"), "token")
	env.Out.Reset()
	return env
}

func TestUninstallKeepsProfiles(t *testing.T) {
	t.Parallel()
	env := uninstallEnv(t)
	before := snapshotTree(t, env.Root)
	if err := env.App.Uninstall(env.load(t), UninstallOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshotTree(t, env.Root)) {
		t.Error("dry run changed files")
	}
	if err := env.App.Uninstall(env.load(t), UninstallOptions{}); err == nil || !strings.Contains(err.Error(), "without --yes") {
		t.Errorf("non-interactive without --yes: %v", err)
	}

	if err := env.App.Uninstall(env.load(t), UninstallOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(env.BinDir, "claude-work")) || !fileExists(filepath.Join(env.BinDir, "claude-foreign")) {
		t.Error("only generated wrappers may be removed")
	}
	got, _ := os.ReadFile(filepath.Join(env.Root, ".zshrc"))
	if want := strings.Replace(zshrc, "eval \"$(cpm hook --default work)\"\n", "", 1); string(got) != want {
		t.Errorf(".zshrc = %q, want %q", got, want)
	}
	if backup, _ := os.ReadFile(filepath.Join(env.Root, ".zshrc.cpm-uninstall.bak")); string(backup) != zshrc {
		t.Errorf("backup = %q", backup)
	}
	if fish, _ := os.ReadFile(filepath.Join(env.Root, ".config", "fish", "config.fish")); len(fish) != 0 {
		t.Errorf("fish hook left: %q", fish)
	}
	if !strings.Contains(env.Out.String(), "# cpm hook is great") {
		t.Errorf("unrecognized line not reported:\n%s", env.Out.String())
	}
	if !fileExists(env.ConfigPath) || !env.Keychain.Exists(KeychainServiceName(filepath.Join(env.profilesBase(), "work")), "") {
		t.Error("without --purge profiles and logins stay")
	}
}

func TestUninstallPurge(t *testing.T) {
	t.Parallel()
	env := uninstallEnv(t)
	other := filepath.Join(env.profilesBase(), "notes.txt")
	mustWrite(t, other, "mine")
	if err := env.App.Uninstall(env.load(t), UninstallOptions{Purge: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if env.Keychain.Exists(KeychainServiceName(filepath.Join(env.profilesBase(), "work")), "") {
		t.Error("login not deleted")
	}
	for _, gone := range []string{env.ConfigPath, filepath.Join(env.profilesBase(), "work"), filepath.Join(env.profilesBase(), "home")} {
		if fileExists(gone) {
			t.Errorf("%s not deleted", gone)
		}
	}
	if !fileExists(other) {
		t.Error("a file cpm does not own was deleted")
	}

	// With nothing foreign left, the directory goes too.
	env = uninstallEnv(t)
	if err := env.App.Uninstall(env.load(t), UninstallOptions{Purge: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if fileExists(env.profilesBase()) {
		t.Error("empty profiles directory left")
	}
}

func TestUninstallNothing(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.work]\n")
	if err := env.App.Uninstall(env.load(t), UninstallOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.Out.String(), "Nothing to uninstall") {
		t.Errorf("output: %q", env.Out.String())
	}
}
