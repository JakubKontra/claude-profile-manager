package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddProfileAppendsAndInstalls(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
	stubKeychain(t, false)
	buf := captureOutput(t)

	err := AddProfile(env.ConfigPath, "client", AddOptions{
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
	env := newTestEnv(t, twoProfiles)
	captureOutput(t)

	if err := AddProfile(env.ConfigPath, "x", AddOptions{NoInstall: true}); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(env.BinDir, "claude-x")) {
		t.Error("--no-install must not install")
	}
	if err := AddProfile(env.ConfigPath, "work", AddOptions{NoInstall: true}); err == nil {
		t.Error("duplicate must be rejected")
	}
	if err := AddProfile(env.ConfigPath, "bad name", AddOptions{NoInstall: true}); err == nil {
		t.Error("invalid name must be rejected")
	}
	if err := AddProfile(env.ConfigPath, "y", AddOptions{NoInstall: true, Env: map[string]string{"BAD-KEY": "1"}}); err == nil {
		t.Error("invalid env name must be rejected")
	}
}

func TestAddProfileCreatesConfigWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	captureOutput(t)
	if err := AddProfile(path, "first", AddOptions{NoInstall: true, Description: "d"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.Profiles["first"] == nil {
		t.Errorf("config not created: %v", err)
	}
}

func TestParseEnvFlag(t *testing.T) {
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

func stubKeychainDelete(t *testing.T) *[]string {
	t.Helper()
	var deleted []string
	prev := keychainDelete
	keychainDelete = func(service, account string) error {
		deleted = append(deleted, service)
		return nil
	}
	t.Cleanup(func() { keychainDelete = prev })
	return &deleted
}

func installedEnv(t *testing.T) testEnv {
	t.Helper()
	env := newTestEnv(t, twoProfiles)
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
	captureOutput(t)
	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(env.profilesBase(), "work", ".credentials.json"), `{}`)
	return env
}

func TestRemoveProfileKeepsDirWithoutPurge(t *testing.T) {
	env := installedEnv(t)
	deleted := stubKeychainDelete(t)

	err := RemoveProfile(env.load(t), env.ConfigPath, "work", RemoveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(env.BinDir, "claude-work")) {
		t.Error("wrapper should be removed")
	}
	if !fileExists(filepath.Join(env.profilesBase(), "work", ".credentials.json")) {
		t.Error("profile data must be kept without --purge")
	}
	if len(*deleted) != 0 {
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
	env := installedEnv(t)
	deleted := stubKeychainDelete(t)
	profileDir := filepath.Join(env.profilesBase(), "work")

	err := RemoveProfile(env.load(t), env.ConfigPath, "work", RemoveOptions{Purge: true, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if fileExists(profileDir) {
		t.Error("profile dir should be deleted")
	}
	if len(*deleted) != 1 || (*deleted)[0] != KeychainServiceName(profileDir) {
		t.Errorf("keychain entry not deleted with the right service: %v", *deleted)
	}
	if fileExists(filepath.Join(env.BinDir, "claude-work")) {
		t.Error("wrapper should be removed")
	}
}

func TestRemoveProfilePromptAndNonInteractive(t *testing.T) {
	env := installedEnv(t)
	stubKeychainDelete(t)
	profileDir := filepath.Join(env.profilesBase(), "work")

	err := RemoveProfile(env.load(t), env.ConfigPath, "work", RemoveOptions{Purge: true, IsTerminal: false})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("non-interactive purge must require --yes, got %v", err)
	}

	// Declined prompt: nothing happens.
	err = RemoveProfile(env.load(t), env.ConfigPath, "work", RemoveOptions{Purge: true, IsTerminal: true, Stdin: strings.NewReader("n\n")})
	if err != nil {
		t.Fatal(err)
	}
	if !fileExists(profileDir) || !fileExists(filepath.Join(env.BinDir, "claude-work")) {
		t.Error("declined prompt must not delete anything")
	}

	// Accepted prompt.
	err = RemoveProfile(env.load(t), env.ConfigPath, "work", RemoveOptions{Purge: true, IsTerminal: true, Stdin: strings.NewReader("y\n")})
	if err != nil {
		t.Fatal(err)
	}
	if fileExists(profileDir) {
		t.Error("accepted prompt should delete the directory")
	}
}

func TestRemoveProfileErrors(t *testing.T) {
	env := installedEnv(t)
	if err := RemoveProfile(env.load(t), env.ConfigPath, "nope", RemoveOptions{}); err == nil {
		t.Error("unknown profile should error")
	}
	if err := RemoveProfile(env.load(t), env.ConfigPath, "../x", RemoveOptions{}); err == nil {
		t.Error("invalid name should error")
	}
}

func TestRemoveProfileKeepsForeignBinary(t *testing.T) {
	env := installedEnv(t)
	// A user-provided script with the wrapper's name but no marker.
	mustWrite(t, filepath.Join(env.BinDir, "claude-personal"), "#!/bin/sh\necho mine\n")
	if err := RemoveProfile(env.load(t), env.ConfigPath, "personal", RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(env.BinDir, "claude-personal")) {
		t.Error("a script cpm did not generate must not be removed")
	}
}

func TestEditorCommand(t *testing.T) {
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
	env := newTestEnv(t, twoProfiles)
	buf := captureOutput(t)
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
	if err := EditConfig(env.ConfigPath, getenv, run); err != nil {
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
	if err := EditConfig(env.ConfigPath, getenv, run); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Warning") {
		t.Errorf("invalid config should warn: %q", buf.String())
	}

	if err := EditConfig(filepath.Join(t.TempDir(), "missing.toml"), getenv, run); err == nil {
		t.Error("missing config should error")
	}
}
