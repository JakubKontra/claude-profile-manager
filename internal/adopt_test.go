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

func TestAdoptCopyKeepsSourceAndWarns(t *testing.T) {
	t.Parallel()
	env := adoptEnv(t)
	dir := filepath.Join(env.profilesBase(), "work")
	report, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{Copy: true})
	if err != nil {
		t.Fatal(err)
	}
	if secret, _ := env.Keychain.Read(KeychainServiceName(dir), ""); string(secret) != `{"token":"default"}` {
		t.Errorf("login not copied: %q", secret)
	}
	if !env.Keychain.Exists(keychainServiceBase, "") {
		t.Error("source login deleted with --copy")
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
	// The copy shares the refresh token (issue #50); the user is told.
	if !strings.Contains(report.Warning, "share one refresh token") || !strings.Contains(env.Out.String(), "Warning: "+report.Warning) {
		t.Errorf("warning = %q, output:\n%s", report.Warning, env.Out.String())
	}

	// A second adopt refuses: the profile is logged in now.
	if _, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{}); err == nil || !strings.Contains(err.Error(), "already logged in") {
		t.Errorf("err = %v", err)
	}
}

// Adopt moves by default: a login left in the source would share its
// refresh token with the profile (issue #50). --move asks for the same.
func TestAdoptMovesByDefault(t *testing.T) {
	t.Parallel()
	for _, opts := range []AdoptOptions{{}, {Move: true}} {
		env := adoptEnv(t)
		dir := filepath.Join(env.profilesBase(), "work")
		report, err := env.App.AdoptLogin(env.load(t), "work", opts)
		if err != nil {
			t.Fatal(err)
		}
		if env.Keychain.Exists(keychainServiceBase, "") {
			t.Errorf("%+v left the source login", opts)
		}
		if secret, _ := env.Keychain.Read(KeychainServiceName(dir), ""); string(secret) != `{"token":"default"}` {
			t.Errorf("%+v: login not moved: %q", opts, secret)
		}
		if n := len(report.Steps); n == 0 || report.Steps[n-1].Kind != "delete" {
			t.Errorf("%+v: steps %+v", opts, report.Steps)
		}
		if report.Warning != "" || !strings.Contains(env.Out.String(), "is logged out now") {
			t.Errorf("%+v: warning %q, output:\n%s", opts, report.Warning, env.Out.String())
		}
	}
	env := adoptEnv(t)
	if _, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{Copy: true, Move: true}); !errors.Is(err, ErrUsage) {
		t.Error("--copy with --move accepted")
	}
}

func TestAdoptRollsBack(t *testing.T) {
	t.Parallel()
	env := adoptEnv(t)
	dir := filepath.Join(env.profilesBase(), "work")
	// The account step fails on an unreadable profile .claude.json.
	mustWrite(t, filepath.Join(dir, ".claude.json"), "{oops")
	_, err := env.App.AdoptLogin(env.load(t), "work", AdoptOptions{})
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
	if len(report.Steps) != 4 || report.Steps[0].Kind != "config" || report.Steps[1].Kind != "file" || report.Steps[3].Kind != "delete" {
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
	if fileExists(filepath.Join(from, ".credentials.json")) {
		t.Error("source login file left behind")
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

func TestDoctorSharedLogin(t *testing.T) {
	t.Parallel()
	blob := func(token string) string {
		return `{"claudeAiOauth":{"accessToken":"a","refreshToken":"` + token + `"}}`
	}
	shared := func(env testEnv, opts DoctorOptions) (Check, bool) {
		opts.LookPath = noLookPath
		return findCheck(env.App.DoctorReportFor(env.load(t), opts).Checks, "shared login")
	}

	// Keychain logins are compared only with --verify.
	env := adoptEnv(t)
	work := filepath.Join(env.profilesBase(), "work")
	env.Keychain.login(work, blob("one"))
	env.Keychain.mu.Lock()
	env.Keychain.entries[keychainServiceBase] = []byte(blob("one"))
	env.Keychain.mu.Unlock()
	if c, ok := shared(env, DoctorOptions{}); ok {
		t.Errorf("Keychain read without --verify: %+v", c)
	}
	c, ok := shared(env, DoctorOptions{Verify: true})
	if !ok || c.Status != "warn" || !strings.Contains(c.Detail, "~/.claude and profile work share one refresh token") ||
		c.Fix != "claude-work auth logout && claude-work auth login" {
		t.Errorf("shared Keychain login: %+v", c)
	}
	if strings.Contains(c.Detail+c.Fix, "one\"") {
		t.Errorf("token leaked: %+v", c)
	}

	// Different tokens, or no refresh token at all, are fine.
	env.Keychain.login(work, blob("two"))
	if c, ok := shared(env, DoctorOptions{Verify: true}); ok {
		t.Errorf("separate logins reported: %+v", c)
	}
	env.Keychain.login(work, `{"token":"default"}`)
	env.Keychain.mu.Lock()
	env.Keychain.entries[keychainServiceBase] = []byte(`{"token":"default"}`)
	env.Keychain.mu.Unlock()
	if c, ok := shared(env, DoctorOptions{Verify: true}); ok {
		t.Errorf("logins without a refresh token reported: %+v", c)
	}

	// File logins need no --verify; two profiles can share one too.
	env = newTestEnv(t, twoProfiles)
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	names := SortedProfileNames(env.load(t))
	for _, name := range names {
		mustWrite(t, filepath.Join(env.profilesBase(), name, ".credentials.json"), blob("file"))
	}
	c, ok = shared(env, DoctorOptions{})
	want := "profile " + names[0] + " and profile " + names[1] + " share one refresh token"
	if !ok || !strings.Contains(c.Detail, want) || c.Fix != "claude-"+names[1]+" auth logout && claude-"+names[1]+" auth login" {
		t.Errorf("shared file login: %+v", c)
	}
}

func TestRefreshTokenHash(t *testing.T) {
	t.Parallel()
	a := refreshTokenHash([]byte(`{"claudeAiOauth":{"refreshToken":"sk-ant-ort01-x"}}`))
	if len(a) != 16 || strings.Contains(a, "sk-ant") {
		t.Errorf("hash = %q", a)
	}
	if b := refreshTokenHash([]byte(`{"claudeAiOauth":{"refreshToken":"sk-ant-ort01-y"}}`)); b == a {
		t.Error("different tokens, same hash")
	}
	for _, in := range []string{``, `{oops`, `{}`, `{"claudeAiOauth":{"accessToken":"x"}}`, `[1]`} {
		if got := refreshTokenHash([]byte(in)); got != "" {
			t.Errorf("refreshTokenHash(%q) = %q", in, got)
		}
	}
}
