package internal

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/json golden files")

var keychainHash = regexp.MustCompile(`(Claude Code-credentials-)[0-9a-f]{8}`)

// assertJSONGolden compares v, encoded as cpm prints it, with
// testdata/json/<name>.json. Each root is replaced by $ROOT first, so the
// files do not depend on temp dirs, and so are Keychain service hashes. Run `go test -update` to rewrite them.
// The goldens are the --json contract: a field may be added, never removed
// or changed in type.
func assertJSONGolden(t *testing.T, name string, v any, roots ...string) {
	t.Helper()
	var buf bytes.Buffer
	app := &App{Out: &buf}
	if err := app.PrintJSON(v); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, root := range roots {
		got = strings.ReplaceAll(got, root, "$ROOT")
	}
	// Keychain service names hash the profile path, which is a temp dir.
	got = keychainHash.ReplaceAllString(got, "${1}HASH")
	path := filepath.Join("testdata", "json", name+".json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal -run %s -update)", err, t.Name())
	}
	if got != string(want) {
		t.Errorf("%s changed (run go test -update if intended):\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

func TestJSONGoldens(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, twoProfiles+"[[match]]\npath = \"~/w/**\"\nprofile = \"work\"\n")
	app := env.App
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
	if err := app.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	env.Keychain.login(filepath.Join(env.profilesBase(), "work"), "token")
	app.Env = []string{"CLAUDE_PROFILE=work", "PATH=" + env.BinDir}
	cfg := env.load(t)
	roots := []string{env.Root}

	assertJSONGolden(t, "list", app.ListProfiles(cfg), roots...)
	assertJSONGolden(t, "which", app.Which(cfg, env.Root), roots...)
	assertJSONGolden(t, "credentials", app.CredentialReport(cfg), roots...)
	assertJSONGolden(t, "doctor", app.DoctorReportFor(cfg, DoctorOptions{LookPath: noLookPath, Platform: "linux/amd64"}), roots...)

	mustWrite(t, filepath.Join(env.profilesBase(), "work", "settings.json"), `{"local": true}`)
	assertJSONGolden(t, "status", app.Status(cfg), roots...)

	rename, err := app.RenameProfile(cfg, "work", "job", true)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONGolden(t, "rename-dry-run", rename, roots...)

	status, err := app.CloudStatus()
	if err != nil {
		t.Fatal(err)
	}
	assertJSONGolden(t, "cloud-status-uninitialized", status, roots...)

	mustWrite(t, filepath.Join(env.BinDir, "claude"), "")
	if err := os.Chmod(filepath.Join(env.BinDir, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	app.Attach = func(spec ExecSpec) error {
		env.Keychain.login(envValue(spec.Env, "CLAUDE_CONFIG_DIR"), "token")
		return nil
	}
	login, err := app.Login(cfg, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONGolden(t, "login", login, roots...)
}

func TestJSONGoldenCloud(t *testing.T) {
	t.Parallel()
	requireGit(t)
	env := newCloudEnv(t, "[profiles.a]\n")
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
	if err := env.App.CloudInit(""); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(env.SourceDir, "CLAUDE.md"), "new")
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"model": "opus"}`)
	diff, err := env.App.CloudDiff()
	if err != nil {
		t.Fatal(err)
	}
	assertJSONGolden(t, "cloud-diff", diff, env.Root)

	status, err := env.App.CloudStatus()
	if err != nil {
		t.Fatal(err)
	}
	// Commit hashes and dates differ per run; the shape is what matters.
	for i := range status.Recent {
		status.Recent[i] = CloudCommit{Hash: "HASH", Subject: "SUBJECT"}
	}
	assertJSONGolden(t, "cloud-status", status, env.Root)
}

func TestJSONGoldenVersion(t *testing.T) {
	// Not parallel: stubs the version and the GitHub API.
	stubVersion(t, "0.6.0")
	fakeReleaseList(t, "v0.7.0")
	r := VersionInfo(false)
	r.Commit = "abc1234"
	assertJSONGolden(t, "version", r)
}

// TestJSONGoldensAreValid makes sure every golden file parses, so a bad
// manual edit is caught even if no test writes that file.
func TestJSONGoldensAreValid(t *testing.T) {
	t.Parallel()
	files, _ := filepath.Glob(filepath.Join("testdata", "json", "*.json"))
	for _, f := range files {
		data, _ := os.ReadFile(f)
		if !json.Valid(data) {
			t.Errorf("%s is not valid JSON", f)
		}
	}
}
