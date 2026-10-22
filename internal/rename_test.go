package internal

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const renameConfig = `[profiles.work] # the job
model = "opus"

[profiles.work.env]
TEAM = "a"

[profiles.home]

[[match]]
remote = "github.com/company/*"
profile = "work" # company repos

[[match]]
path = "~/p/**"
profile = 'home'
`

// renameEnv installs the profiles of renameConfig, logs work in through the
// Keychain and adds a secrets.toml entry for it.
func renameEnv(t *testing.T) testEnv {
	t.Helper()
	env := newTestEnv(t, renameConfig)
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	env.Keychain.login(filepath.Join(env.profilesBase(), "work"), `{"token":"t"}`)
	secrets := filepath.Join(env.profilesBase(), "secrets.toml")
	mustWrite(t, secrets, "[profiles.work.env]\nAPI_TOKEN = \"s\"\n")
	if err := os.Chmod(secrets, 0o600); err != nil {
		t.Fatal(err)
	}
	env.Out.Reset()
	return env
}

func TestRenameProfile(t *testing.T) {
	t.Parallel()
	env := renameEnv(t)
	base := env.profilesBase()
	oldDir, newDir := filepath.Join(base, "work"), filepath.Join(base, "job")

	if err := renameProfile(env.App, env.load(t), "work", "job", false); err != nil {
		t.Fatal(err)
	}
	cfg := env.load(t)
	job := cfg.Profiles["job"]
	if cfg.Profiles["work"] != nil || job == nil || job.Model != "opus" || job.Env["TEAM"] != "a" || job.LocalEnv["API_TOKEN"] != "s" {
		t.Fatalf("config not renamed: %+v", cfg.Profiles)
	}
	if cfg.Match[0].Profile != "job" || cfg.Match[1].Profile != "home" {
		t.Errorf("match rules: %+v", cfg.Match)
	}
	data, _ := os.ReadFile(env.ConfigPath)
	for _, want := range []string{"[profiles.job] # the job", `profile = "job" # company repos`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config lost formatting, missing %q:\n%s", want, data)
		}
	}
	if fileExists(oldDir) || !fileExists(filepath.Join(newDir, "settings.json")) {
		t.Error("profile directory not moved")
	}
	if marker, _ := os.ReadFile(filepath.Join(newDir, ".cpm", "profile")); string(marker) != "job\n" {
		t.Errorf("marker = %q", marker)
	}
	if env.Keychain.Exists(KeychainServiceName(oldDir), "") {
		t.Error("old Keychain login not deleted")
	}
	if secret, err := env.Keychain.Read(KeychainServiceName(newDir), ""); err != nil || string(secret) != `{"token":"t"}` {
		t.Errorf("Keychain login not moved: %q %v", secret, err)
	}
	if fileExists(filepath.Join(env.BinDir, "claude-work")) || !isGeneratedWrapper(filepath.Join(env.BinDir, "claude-job")) {
		t.Error("wrapper not renamed")
	}
	// The result is what a fresh install would produce.
	plan, err := env.App.PlanInstall(cfg, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 0 {
		t.Errorf("install after rename still wants changes: %+v", plan)
	}
}

func TestRenameProfileDryRun(t *testing.T) {
	t.Parallel()
	env := renameEnv(t)
	before := snapshotTree(t, env.Root)
	if err := renameProfile(env.App, env.load(t), "work", "job", true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshotTree(t, env.Root)) {
		t.Error("dry run changed files")
	}
	if !env.Keychain.Exists(KeychainServiceName(filepath.Join(env.profilesBase(), "work")), "") {
		t.Error("dry run touched the Keychain")
	}
	for _, want := range []string{"keychain  copy login", "move ", "and 1 [[match]] rule", "secrets ", "wrapper ", "keychain  delete old login"} {
		if !strings.Contains(env.Out.String(), want) {
			t.Errorf("dry run missing %q:\n%s", want, env.Out.String())
		}
	}
}

func TestRenameProfileRollsBack(t *testing.T) {
	t.Parallel()
	env := renameEnv(t)
	before := contentTree(t, env.Root)
	// The wrapper is the last step; make it fail.
	if err := os.Chmod(env.BinDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(env.BinDir, 0o755) })

	err := renameProfile(env.App, env.load(t), "work", "job", false)
	if err == nil || !strings.Contains(err.Error(), "wrapper") {
		t.Fatalf("err = %v, want wrapper failure", err)
	}
	if err := os.Chmod(env.BinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if after := contentTree(t, env.Root); !reflect.DeepEqual(before, after) {
		for path := range after {
			if before[path] != after[path] {
				t.Errorf("%s differs after rollback", path)
			}
		}
		for path := range before {
			if _, ok := after[path]; !ok {
				t.Errorf("%s missing after rollback", path)
			}
		}
	}
	work, job := filepath.Join(env.profilesBase(), "work"), filepath.Join(env.profilesBase(), "job")
	if !env.Keychain.Exists(KeychainServiceName(work), "") || env.Keychain.Exists(KeychainServiceName(job), "") {
		t.Error("Keychain not restored")
	}
	if !strings.Contains(env.Out.String(), "reverted  move") {
		t.Errorf("output:\n%s", env.Out.String())
	}
}

// contentTree maps every path under root to its mode and content (or link
// target), ignoring modification times.
func contentTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += string(data)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(path)
			value += target
		}
		tree[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestRenameProfileKeychainWriteFails(t *testing.T) {
	t.Parallel()
	env := renameEnv(t)
	env.Keychain.writeErr = errors.New("denied")
	if err := renameProfile(env.App, env.load(t), "work", "job", false); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err = %v", err)
	}
	if cfg := env.load(t); cfg.Profiles["work"] == nil || !fileExists(filepath.Join(env.profilesBase(), "work")) {
		t.Error("nothing should change when the Keychain write fails")
	}
}

func TestRenameProfileRejects(t *testing.T) {
	t.Parallel()
	env := renameEnv(t)
	mustMkdir(t, filepath.Join(env.profilesBase(), "taken"))
	env.Keychain.login(filepath.Join(env.profilesBase(), "stale"), "x")
	cases := []struct{ old, new, want string }{
		{"nope", "x", `unknown profile "nope"`},
		{"work", "home", `profile "home" already exists`},
		{"work", "bad name", "invalid profile name"},
		{"work", "auto", "reserved"},
		{"work", "cloud", "reserved"},
		{"work", "taken", "already exists"},
		{"work", "stale", "Keychain already has an entry"},
	}
	for _, c := range cases {
		if err := renameProfile(env.App, env.load(t), c.old, c.new, false); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("rename %s -> %s: err = %v, want %q", c.old, c.new, err, c.want)
		}
	}
	if env.load(t).Profiles["work"] == nil {
		t.Error("a rejected rename changed the config")
	}
}

func TestRenameProfileNotInstalled(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.work]\n")
	if err := renameProfile(env.App, env.load(t), "work", "job", false); err != nil {
		t.Fatal(err)
	}
	if env.load(t).Profiles["job"] == nil {
		t.Error("config not renamed")
	}
	if fileExists(filepath.Join(env.BinDir, "claude-job")) {
		t.Error("an uninstalled profile should not get a wrapper")
	}
}

func TestRenameProfileTableForms(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"[profiles.work]\na = 1\n", "[profiles.job]\na = 1\n"},
		{"[ profiles . \"work\" ]\n", "[ profiles . job ]\n"},
		{"[profiles.work.mcp_servers.\"my srv\"]\ncommand = \"x\"\n", "[profiles.job.mcp_servers.\"my srv\"]\ncommand = \"x\"\n"},
		{"[profiles.workshop]\n", "[profiles.workshop]\n"},
	}
	for _, c := range cases {
		got, _, err := RenameProfileTable([]byte(c.in), "work", "job")
		if err != nil || string(got) != c.want {
			t.Errorf("RenameProfileTable(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	if _, _, err := RenameProfileTable([]byte("profiles.work.model = \"x\"\n"), "work", "job"); err == nil {
		t.Error("dotted keys must be refused")
	}
	if _, _, err := RenameMatchProfile([]byte("match = [{profile = \"work\"}]\n"), "work", "job"); err == nil {
		t.Error("inline match rules must be refused")
	}
}

func renameProfile(app *App, cfg *Config, old, new string, dryRun bool) error {
	_, err := app.RenameProfile(cfg, old, new, dryRun)
	return err
}
