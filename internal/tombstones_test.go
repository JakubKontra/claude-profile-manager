package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudPropagatesRenameAndRemove(t *testing.T) {
	t.Parallel()
	requireGit(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := gitCommand(t, "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	install := func(env cloudEnv) {
		t.Helper()
		if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	a := newCloudEnv(t, "[profiles.work]\n[profiles.home]\n")
	mustWrite(t, filepath.Join(a.SourceDir, "settings.json"), `{}`)
	install(a)
	if err := a.App.CloudInit(bare); err != nil {
		t.Fatal(err)
	}
	if err := a.App.CloudPush(PushOptions{}); err != nil {
		t.Fatal(err)
	}

	b := newCloudEnv(t, "[profiles.personal]\n")
	if err := b.App.CloudInit(bare); err != nil {
		t.Fatal(err)
	}
	install(b)
	b.Keychain.login(filepath.Join(b.profilesBase(), "work"), "b-token")

	// A renames and removes, then pushes.
	if _, err := a.App.RenameProfile(a.load(t), "work", "job", false); err != nil {
		t.Fatal(err)
	}
	if err := a.App.RemoveProfile(a.load(t), "home", RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := a.App.CloudPush(PushOptions{}); err != nil {
		t.Fatal(err)
	}

	// Without a terminal and --yes, B only lists them, and does not add
	// "job" as a second profile.
	b.Out.Reset()
	if err := b.App.CloudPull(PullOptions{}); err != nil {
		t.Fatal(err)
	}
	out := b.Out.String()
	if !strings.Contains(out, "rename work -> job") || !strings.Contains(out, "remove home") || !strings.Contains(out, "Not applied") {
		t.Errorf("pending changes not listed:\n%s", out)
	}
	if cfg := b.load(t); cfg.Profiles["job"] != nil || cfg.Profiles["work"] == nil || cfg.Profiles["home"] == nil {
		t.Errorf("profiles changed before confirmation: %v", SortedProfileNames(cfg))
	}

	// With --yes they are applied: the login moves along.
	if err := b.App.CloudPull(PullOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	cfg := b.load(t)
	if cfg.Profiles["job"] == nil || cfg.Profiles["work"] != nil || cfg.Profiles["home"] != nil || cfg.Profiles["personal"] == nil {
		t.Fatalf("B profiles: %v", SortedProfileNames(cfg))
	}
	if secret, _ := b.Keychain.Read(KeychainServiceName(filepath.Join(b.profilesBase(), "job")), ""); string(secret) != "b-token" {
		t.Errorf("login not moved with the rename: %q", secret)
	}
	if !fileExists(filepath.Join(b.profilesBase(), "home")) {
		t.Error("a removal from the cloud must keep the profile data")
	}
	if journal, _ := readTombstones(b.App.tombstonesPath()); len(journal) != 2 {
		t.Errorf("B journal: %+v", journal)
	}

	// Nothing is offered again, and a profile re-created under a removed
	// name survives later pulls.
	if err := b.App.AddProfile("home", AddOptions{NoInstall: true}); err != nil {
		t.Fatal(err)
	}
	b.Out.Reset()
	if err := b.App.CloudPull(PullOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.Out.String(), "Profile changes") || b.load(t).Profiles["home"] == nil {
		t.Errorf("changes replayed twice:\n%s", b.Out.String())
	}

	// A machine that never had "work" just gets "job".
	c := newCloudEnv(t, "[profiles.other]\n")
	if err := c.App.CloudInit(bare); err != nil {
		t.Fatal(err)
	}
	if err := c.App.CloudPull(PullOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if cfg := c.load(t); cfg.Profiles["job"] == nil || cfg.Profiles["work"] != nil {
		t.Errorf("C profiles: %v", SortedProfileNames(cfg))
	}
}

func TestRecordChangeOnlyWithCloud(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.work]\n[profiles.home]\n")
	if err := env.App.RemoveProfile(env.load(t), "home", RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.App.tombstonesPath()); !os.IsNotExist(err) {
		t.Error("journal written without cloud sync")
	}
}

func TestMergeTombstones(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	local, pulled := filepath.Join(dir, "local.toml"), filepath.Join(dir, "pulled.toml")
	if err := writeTombstones(local, []ProfileChange{{ID: "1", Kind: "removed", Name: "a", At: "2026-01-02T00:00:00Z"}}); err != nil {
		t.Fatal(err)
	}
	if err := writeTombstones(pulled, []ProfileChange{
		{ID: "0", Kind: "renamed", From: "x", To: "y", At: "2026-01-01T00:00:00Z"},
		{ID: "1", Kind: "removed", Name: "a", At: "2026-01-02T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := mergeTombstones(pulled, local); err != nil {
		t.Fatal(err)
	}
	got, _ := readTombstones(local)
	if len(got) != 2 || got[0].ID != "0" || got[1].ID != "1" {
		t.Errorf("merged: %+v", got)
	}
}
