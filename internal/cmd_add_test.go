package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddProfileAppendsAndInstalls(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, twoProfiles)
	app, buf := env.App, env.Out
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
	err := app.AddProfile("client", AddOptions{
		Description: "Client",
		Model:       "opus",
		AddDirs:     []string{"~/c"},
		Env:         map[string]string{"K": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := env.load(t)
	p := cfg.Profiles["client"]
	if p == nil || p.Model != "opus" || p.Env["K"] != "v" || len(p.AddDirs) != 1 {
		t.Errorf("profile not saved: %+v", p)
	}
	if !fileExists(filepath.Join(env.BinDir, "claude-client")) || !fileExists(filepath.Join(env.profilesBase(), "client", "settings.json")) {
		t.Error("profile not installed")
	}
	if !strings.Contains(buf.String(), "Authenticate with: claude-client") {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestAddProfileNoInstallAndErrors(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, twoProfiles)
	app := env.App

	if err := app.AddProfile("x", AddOptions{NoInstall: true}); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(env.BinDir, "claude-x")) {
		t.Error("--no-install must not install")
	}
	if err := app.AddProfile("work", AddOptions{NoInstall: true}); err == nil {
		t.Error("duplicate must be rejected")
	}
	if err := app.AddProfile("bad name", AddOptions{NoInstall: true}); err == nil {
		t.Error("invalid name must be rejected")
	}
	if err := app.AddProfile("y", AddOptions{NoInstall: true, Env: map[string]string{"BAD-KEY": "1"}}); err == nil {
		t.Error("invalid env name must be rejected")
	}
}

func TestAddProfileCreatesConfigWhenMissing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	app, _ := testAppAt(t, path)
	if err := app.AddProfile("first", AddOptions{NoInstall: true, Description: "d"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.Profiles["first"] == nil {
		t.Errorf("config not created: %v", err)
	}
}

func TestParseEnvFlag(t *testing.T) {
	t.Parallel()
	env, err := ParseEnvFlag([]string{"A=1", "B=x=y", "C="})
	if err != nil || env["A"] != "1" || env["B"] != "x=y" || env["C"] != "" {
		t.Errorf("unexpected: %v %v", env, err)
	}
	for _, bad := range []string{"NOEQ", "=v", "BAD-KEY=1"} {
		if _, err := ParseEnvFlag([]string{bad}); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if env, err := ParseEnvFlag(nil); env != nil || err != nil {
		t.Errorf("nil input should give nil map")
	}
}

func installedEnv(t *testing.T) testEnv {
	t.Helper()
	env := newTestEnv(t, twoProfiles)
	app := env.App
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
	if err := app.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(env.profilesBase(), "work", ".credentials.json"), `{}`)
	return env
}

func TestRemoveProfileKeepsDirWithoutPurge(t *testing.T) {
	t.Parallel()
	env := installedEnv(t)
	app := env.App

	err := app.RemoveProfile(env.load(t), "work", RemoveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(env.BinDir, "claude-work")) {
		t.Error("wrapper should be removed")
	}
	if !fileExists(filepath.Join(env.profilesBase(), "work", ".credentials.json")) {
		t.Error("profile data must be kept without --purge")
	}
	if len(env.Keychain.deleted) != 0 {
		t.Error("keychain must not be touched without --purge")
	}
	cfg := env.load(t)
	if _, still := cfg.Profiles["work"]; still {
		t.Error("config entry should be removed")
	}
	if cfg.Profiles["personal"] == nil {
		t.Error("other profiles must survive")
	}
}

func TestRemoveProfilePurgeDeletesEverything(t *testing.T) {
	t.Parallel()
	env := installedEnv(t)
	app := env.App
	profileDir := filepath.Join(env.profilesBase(), "work")

	err := app.RemoveProfile(env.load(t), "work", RemoveOptions{Purge: true, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if fileExists(profileDir) {
		t.Error("profile dir should be deleted")
	}
	if len(env.Keychain.deleted) != 1 || env.Keychain.deleted[0] != KeychainServiceName(profileDir) {
		t.Errorf("keychain entry not deleted with the right service: %v", env.Keychain.deleted)
	}
	if fileExists(filepath.Join(env.BinDir, "claude-work")) {
		t.Error("wrapper should be removed")
	}
}

func TestRemoveProfilePromptAndNonInteractive(t *testing.T) {
	t.Parallel()
	env := installedEnv(t)
	app := env.App
	profileDir := filepath.Join(env.profilesBase(), "work")

	err := app.RemoveProfile(env.load(t), "work", RemoveOptions{Purge: true, IsTerminal: false})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("non-interactive purge must require --yes, got %v", err)
	}

	// Declined prompt: nothing happens.
	err = app.RemoveProfile(env.load(t), "work", RemoveOptions{Purge: true, IsTerminal: true, Stdin: strings.NewReader("n\n")})
	if err != nil {
		t.Fatal(err)
	}
	if !fileExists(profileDir) || !fileExists(filepath.Join(env.BinDir, "claude-work")) {
		t.Error("declined prompt must not delete anything")
	}

	// Accepted prompt.
	err = app.RemoveProfile(env.load(t), "work", RemoveOptions{Purge: true, IsTerminal: true, Stdin: strings.NewReader("y\n")})
	if err != nil {
		t.Fatal(err)
	}
	if fileExists(profileDir) {
		t.Error("accepted prompt should delete the directory")
	}
}

func TestRemoveProfileErrors(t *testing.T) {
	t.Parallel()
	env := installedEnv(t)
	app := env.App
	if err := app.RemoveProfile(env.load(t), "nope", RemoveOptions{}); err == nil {
		t.Error("unknown profile should error")
	}
	if err := app.RemoveProfile(env.load(t), "../x", RemoveOptions{}); err == nil {
		t.Error("invalid name should error")
	}
}

func TestRemoveProfileKeepsForeignBinary(t *testing.T) {
	t.Parallel()
	env := installedEnv(t)
	app := env.App
	// A user-provided script with the wrapper's name but no marker.
	mustWrite(t, filepath.Join(env.BinDir, "claude-personal"), "#!/bin/sh\necho mine\n")
	if err := app.RemoveProfile(env.load(t), "personal", RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(env.BinDir, "claude-personal")) {
		t.Error("a script cpm did not generate must not be removed")
	}
}

func TestEditorCommand(t *testing.T) {
	t.Parallel()
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := EditorCommand(get(map[string]string{"VISUAL": "code --wait", "EDITOR": "vim"})); strings.Join(got, " ") != "code --wait" {
		t.Errorf("VISUAL should win: %v", got)
	}
	if got := EditorCommand(get(map[string]string{"EDITOR": "nano"})); got[0] != "nano" {
		t.Errorf("EDITOR fallback: %v", got)
	}
	if got := EditorCommand(get(map[string]string{})); got[0] != "vi" {
		t.Errorf("vi default: %v", got)
	}
}

func TestEditConfigRunsEditorAndValidates(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, twoProfiles)
	app, buf := env.App, env.Out
	var ran []string
	run := func(c *exec.Cmd) error {
		ran = c.Args
		return nil
	}
	getenv := func(k string) string {
		if k == "EDITOR" {
			return "myedit --flag"
		}
		return ""
	}
	if err := app.EditConfig(getenv, run); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ran, " ") != "myedit --flag "+env.ConfigPath {
		t.Errorf("editor invoked as %v", ran)
	}
	if !strings.Contains(buf.String(), "Config is valid") {
		t.Errorf("unexpected output: %q", buf.String())
	}

	// Invalid config after editing is reported, not fatal.
	buf.Reset()
	os.WriteFile(env.ConfigPath, []byte("[profiles.\"bad name\"]\n"), 0o644)
	if err := app.EditConfig(getenv, run); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Warning") {
		t.Errorf("invalid config should warn: %q", buf.String())
	}

	missing, _ := testAppAt(t, filepath.Join(t.TempDir(), "missing.toml"))
	if err := missing.EditConfig(getenv, run); err == nil {
		t.Error("missing config should error")
	}
}
