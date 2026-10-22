package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveThenPurgeLastProfile(t *testing.T) {
	env := newTestEnv(t, "[profiles.work]\n")
	app, _ := testAppAt(t, env.ConfigPath)
	cfg := env.load(t)
	if err := app.InstallProfiles(cfg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(env.profilesBase(), "work")
	// Avoid real Keychain access; use the existing test injection.
	oldDelete := keychainDelete
	keychainDelete = func(string, string) error { return nil }
	t.Cleanup(func() { keychainDelete = oldDelete })
	if err := app.RemoveProfile(cfg, "work", RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadCloudConfig(env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := orphanProfiles(cfg, env.profilesBase()); len(got) != 1 || got[0] != "work" {
		t.Fatalf("orphan missing: %v", got)
	}
	if err := app.RemoveProfile(cfg, "work", RemoveOptions{Purge: true}); err == nil {
		t.Fatal("unconfirmed purge accepted")
	}
	if err := app.RemoveProfile(cfg, "work", RemoveOptions{Purge: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if fileExistsAt(dir) {
		t.Fatal("orphan not removed")
	}
}

func TestOrphanPurgeProtectsInternalAndForeignDirectories(t *testing.T) {
	env := newTestEnv(t, "[profiles.work]\n")
	app, _ := testAppAt(t, env.ConfigPath)
	cfg := env.load(t)
	for _, name := range []string{"cloud", "foreign"} {
		mustMkdir(t, filepath.Join(env.profilesBase(), name))
		if err := app.RemoveProfile(cfg, name, RemoveOptions{Purge: true, Yes: true}); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	target := t.TempDir()
	mustWrite(t, filepath.Join(target, "settings.json"), "{}")
	os.Symlink(target, filepath.Join(env.profilesBase(), "linked"))
	if err := app.RemoveProfile(cfg, "linked", RemoveOptions{Purge: true, Yes: true}); err == nil {
		t.Fatal("accepted symlink")
	}
	if len(orphanProfiles(cfg, env.profilesBase())) != 0 {
		t.Fatal("foreign directories reported as profiles")
	}
}
