package internal

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// adoptEnv installs profile work and gives ~/.claude a Keychain login and
// ~/.claude.json an account.
func adoptEnv(t *testing.T) testEnv {
	t.Helper()
	env := newTestEnv(t, "[profiles.work]\n")
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	env.Keychain.mu.Lock()
	env.Keychain.entries[keychainServiceBase] = []byte(`{"token":"default"}`)
	env.Keychain.mu.Unlock()
	mustWrite(t, filepath.Join(env.Root, ".claude.json"), `{"oauthAccount":{"emailAddress":"me@example.com"},"theme":"dark"}`)
	env.Out.Reset()
	return env
}

func TestAdoptCopiesDefaultLogin(t *testing.T) {
	t.Parallel()
	env := adoptEnv(t)
	dir := filepath.Join(env.profilesBase(), "work")
	report, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if secret, _ := env.Keychain.Read(KeychainServiceName(dir), ""); string(secret) != `{"token":"default"}` {
		t.Errorf("login not copied: %q", secret)
	}
	if !env.Keychain.Exists(keychainServiceBase, "") {
		t.Error("source login deleted without --move")
	}
	status, err := GetCredentialStatus(env.Keychain, dir)
	if err != nil || status.Account != "me@example.com" {
		t.Errorf("status: %+v %v", status, err)
	}
	if got := readJSON(t, filepath.Join(dir, ".claude.json")); got["oauthAccount"] == nil {
		t.Errorf("profile .claude.json: %v", got)
	}
	kinds := []string{}
	for _, s := range report.Steps {
		kinds = append(kinds, s.Kind)
	}
	if !reflect.DeepEqual(kinds, []string{"keychain", "account"}) {
		t.Errorf("steps: %v", kinds)
	}

	// A second adopt refuses: the profile is logged in now.
	if _, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{}); err == nil || !strings.Contains(err.Error(), "already logged in") {
		t.Errorf("err = %v", err)
	}
}

func TestAdoptMove(t *testing.T) {
	t.Parallel()
	env := adoptEnv(t)
	if _, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{Move: true}); err != nil {
		t.Fatal(err)
	}
	if env.Keychain.Exists(keychainServiceBase, "") {
		t.Error("--move left the source login")
	}
}

func TestAdoptRollsBack(t *testing.T) {
	t.Parallel()
	env := adoptEnv(t)
	dir := filepath.Join(env.profilesBase(), "work")
	// The account step fails on an unreadable profile .claude.json.
	mustWrite(t, filepath.Join(dir, ".claude.json"), "{oops")
	_, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{Move: true})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("err = %v", err)
	}
	if env.Keychain.Exists(KeychainServiceName(dir), "") {
		t.Error("copied login not reverted")
	}
	if !env.Keychain.Exists(keychainServiceBase, "") {
		t.Error("source login must survive a failed adopt")
	}
}

func TestAdoptFileLoginAndNewProfile(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.work]\n")
	from := filepath.Join(env.Root, "custom")
	mustWrite(t, filepath.Join(from, ".credentials.json"), `{"claudeAiOauth":{"accessToken":"x"}}`)
	mustWrite(t, filepath.Join(from, ".claude.json"), `{"oauthAccount":{"emailAddress":"c@example.com"}}`)

	before := snapshotTree(t, env.Root)
	report, err := env.App.AdoptLogin(env.load(t), "fresh", AdoptOptions{From: from, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshotTree(t, env.Root)) {
		t.Error("dry run changed files")
	}
	if len(report.Steps) != 3 || report.Steps[0].Kind != "config" || report.Steps[1].Kind != "file" {
		t.Errorf("dry run steps: %+v", report.Steps)
	}

	if _, err := env.App.AdoptLogin(env.load(t), "fresh", AdoptOptions{From: from}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(env.profilesBase(), "fresh")
	if env.load(t).Profiles["fresh"] == nil || !fileExists(filepath.Join(dir, ".credentials.json")) {
		t.Fatal("profile not added or login not copied")
	}
	if st, _ := os.Stat(filepath.Join(dir, ".credentials.json")); st.Mode().Perm() != 0o600 {
		t.Errorf("credentials mode %v", st.Mode())
	}
	if status, err := GetCredentialStatus(env.Keychain, dir); err != nil || status.Account != "c@example.com" {
		t.Errorf("status: %+v %v", status, err)
	}
}

func TestAdoptErrors(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.work]\n")
	if _, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{}); err == nil || !strings.Contains(err.Error(), "no Claude Code login found") {
		t.Errorf("no source login: %v", err)
	}
	env = adoptEnv(t)
	env.Keychain.writeErr = errors.New("denied")
	if _, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{}); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Errorf("write failure: %v", err)
	}
	if _, err := env.App.AdoptLogin(env.load(t), "bad name", AdoptOptions{}); err == nil {
		t.Error("invalid name accepted")
	}
}

func TestDoctorSuggestsAdopt(t *testing.T) {
	t.Parallel()
	env := adoptEnv(t)
	report := env.App.DoctorReportFor(env.load(t), DoctorOptions{LookPath: noLookPath})
	if c, ok := findCheck(report.Checks, "default login"); !ok || !strings.Contains(c.Detail, "cpm adopt") {
		t.Errorf("default login check: %+v", c)
	}
	if _, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{}); err != nil {
		t.Fatal(err)
	}
	report = env.App.DoctorReportFor(env.load(t), DoctorOptions{LookPath: noLookPath})
	if _, ok := findCheck(report.Checks, "default login"); ok {
		t.Error("suggestion stays after adopting")
	}
}
