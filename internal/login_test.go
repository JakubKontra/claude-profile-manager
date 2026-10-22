package internal

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// loginEnv installs profiles and gives the App a PATH with a claude binary.
// Logins are recorded instead of run; a successful one stores a Keychain
// entry the way claude would.
func loginEnv(t *testing.T, profilesTOML string, fail map[string]bool) (testEnv, *[]string) {
	t.Helper()
	env := newTestEnv(t, profilesTOML)
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(env.BinDir, "claude"), "")
	if err := os.Chmod(filepath.Join(env.BinDir, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	env.App.Env = []string{"PATH=" + env.BinDir, "CLAUDE_PROFILE=other"}
	env.Out.Reset()
	var calls []string
	env.App.Attach = func(spec ExecSpec) error {
		profile := envValue(spec.Env, "CLAUDE_PROFILE")
		calls = append(calls, profile+": "+strings.Join(spec.Argv, " "))
		if fail[profile] {
			return errors.New("exit status 1")
		}
		env.Keychain.login(envValue(spec.Env, "CLAUDE_CONFIG_DIR"), "token")
		return nil
	}
	return env, &calls
}

func envValue(env []string, key string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}

func TestLoginAllSkipsLoggedInAndExternalAuth(t *testing.T) {
	t.Parallel()
	env, calls := loginEnv(t, `[profiles.b]
[profiles.a]
model = "opus"
[profiles.done]
[profiles.vertex.env]
CLAUDE_CODE_USE_VERTEX = "1"
`, nil)
	env.Keychain.login(filepath.Join(env.profilesBase(), "done"), "token")

	if err := loginAndRender(env.App, env.load(t), nil, true); err != nil {
		t.Fatal(err)
	}
	// Sorted order, no profile flags for the auth subcommand.
	want := []string{"a: claude auth login", "b: claude auth login"}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("calls = %q, want %q", *calls, want)
	}
	out := env.Out.String()
	for _, line := range []string{"==> Logging in a (1/2)", "done                 already logged in", "vertex               skipped (uses CLAUDE_CODE_USE_VERTEX)", "a                    logged in"} {
		if !strings.Contains(out, line) {
			t.Errorf("output missing %q:\n%s", line, out)
		}
	}

	// Everything is logged in now.
	*calls = nil
	env.Out.Reset()
	if err := loginAndRender(env.App, env.load(t), nil, true); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("second run logged in again: %q", *calls)
	}
}

func TestLoginNamedRunsEvenWhenLoggedIn(t *testing.T) {
	t.Parallel()
	env, calls := loginEnv(t, "[profiles.a]\n[profiles.b]\n", nil)
	env.Keychain.login(filepath.Join(env.profilesBase(), "b"), "token")
	if err := loginAndRender(env.App, env.load(t), []string{"b", "a", "b"}, false); err != nil {
		t.Fatal(err)
	}
	if want := []string{"b: claude auth login", "a: claude auth login"}; !reflect.DeepEqual(*calls, want) {
		t.Errorf("calls = %q, want %q", *calls, want)
	}
}

func TestLoginContinuesAfterFailure(t *testing.T) {
	t.Parallel()
	env, calls := loginEnv(t, "[profiles.a]\n[profiles.b]\n", map[string]bool{"a": true})
	err := loginAndRender(env.App, env.load(t), nil, true)
	if !errors.Is(err, ErrLoginFailed) {
		t.Fatalf("err = %v, want ErrLoginFailed", err)
	}
	if len(*calls) != 2 {
		t.Errorf("calls = %q, want both profiles", *calls)
	}
	if out := env.Out.String(); !strings.Contains(out, "a                    failed (exit status 1)") || !strings.Contains(out, "b                    logged in") {
		t.Errorf("summary:\n%s", out)
	}
}

func TestLoginArgumentErrors(t *testing.T) {
	t.Parallel()
	env, calls := loginEnv(t, "[profiles.a]\n", nil)
	cfg := env.load(t)
	for _, c := range []struct {
		names []string
		all   bool
		want  string
	}{
		{nil, false, "or pass --all"},
		{[]string{"a"}, true, "or pass --all"},
		{[]string{"nope"}, false, `unknown profile "nope"`},
	} {
		if err := loginAndRender(env.App, cfg, c.names, c.all); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Login(%v, %v) = %v, want %q", c.names, c.all, err, c.want)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("unexpected logins: %q", *calls)
	}
}

func TestLoginNotInstalled(t *testing.T) {
	t.Parallel()
	env, calls := loginEnv(t, "[profiles.a]\n", nil)
	writeTestConfig(t, env, "[profiles.a]\n[profiles.new]\n")
	if err := loginAndRender(env.App, env.load(t), []string{"new"}, false); !errors.Is(err, ErrLoginFailed) {
		t.Fatalf("err = %v", err)
	}
	if len(*calls) != 0 || !strings.Contains(env.Out.String(), "not installed (run cpm install)") {
		t.Errorf("calls %q, output:\n%s", *calls, env.Out.String())
	}
}

func TestExternalAuth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p    Profile
		want string
	}{
		{Profile{}, ""},
		{Profile{Env: map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"}}, "CLAUDE_CODE_USE_BEDROCK"},
		{Profile{Env: map[string]string{"CLAUDE_CODE_USE_VERTEX": "0"}}, ""},
		{Profile{LocalEnv: map[string]string{"ANTHROPIC_API_KEY": "sk"}}, "ANTHROPIC_API_KEY"},
		{Profile{Env: map[string]string{"OTHER": "1"}}, ""},
	}
	for _, c := range cases {
		if got := externalAuth(&c.p); got != c.want {
			t.Errorf("externalAuth(%+v) = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestDoctorSuggestsLogin(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.work]\n[profiles.vertex.env]\nCLAUDE_CODE_USE_VERTEX = \"1\"\n")
	mustMkdir(t, filepath.Join(env.profilesBase(), "work"))
	mustMkdir(t, filepath.Join(env.profilesBase(), "vertex"))
	checks := RunDoctorWithOptions(env.load(t), env.profilesBase(), DoctorOptions{HomeDir: t.TempDir(), LookPath: noLookPath, Keychain: env.Keychain})
	if c, _ := findCheck(checks, "profile/work/credentials"); c.Status != "warn" || c.Detail != "not authenticated (run cpm login work)" {
		t.Errorf("work: %+v", c)
	}
	if c, _ := findCheck(checks, "profile/vertex/credentials"); c.Status != "ok" || c.Detail != "no login needed (uses CLAUDE_CODE_USE_VERTEX)" {
		t.Errorf("vertex: %+v", c)
	}
}

// loginAndRender runs Login and prints its summary like the command does.
func loginAndRender(app *App, cfg *Config, names []string, all bool) error {
	report, err := app.Login(cfg, names, all)
	app.RenderLoginReport(report)
	return err
}
