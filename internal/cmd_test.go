package internal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoProfiles = `[profiles.work]
description = "Work"
model = "sonnet"

[profiles.personal]
description = "Personal"
`

func TestInstallProfilesCreatesDirsAndWrappers(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"model": "opus"}`)
	mustMkdir(t, filepath.Join(env.SourceDir, "skills"))
	buf := captureOutput(t)

	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{}); err != nil {
		t.Fatalf("InstallProfiles: %v", err)
	}

	for _, name := range []string{"work", "personal"} {
		if !fileExists(filepath.Join(env.profilesBase(), name, "settings.json")) {
			t.Errorf("profile %s: settings.json not copied", name)
		}
		if _, err := os.Readlink(filepath.Join(env.profilesBase(), name, "skills")); err != nil {
			t.Errorf("profile %s: skills not symlinked: %v", name, err)
		}
		if !fileExists(filepath.Join(env.BinDir, "claude-"+name)) {
			t.Errorf("profile %s: wrapper not installed", name)
		}
	}
	if !strings.Contains(buf.String(), "Done.") {
		t.Errorf("output missing Done: %q", buf.String())
	}

	// Second run must be idempotent (no re-install messages).
	buf.Reset()
	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "installed ") {
		t.Errorf("second install should not rewrite wrappers: %q", buf.String())
	}
}

func TestInstallProfilesRefusesWhenDiverged(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"a": 1}`)
	mustWrite(t, filepath.Join(env.profilesBase(), "work", "settings.json"), `{"a": 1, "local": true}`)
	captureOutput(t)

	err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{Sync: true})
	if !errors.Is(err, ErrDiverged) {
		t.Fatalf("expected ErrDiverged, got %v", err)
	}

	// --force overwrites.
	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{Sync: true, Force: true}); err != nil {
		t.Fatalf("forced sync: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(env.profilesBase(), "work", "settings.json"))
	if string(data) != `{"a": 1}` {
		t.Errorf("forced sync did not overwrite: %q", data)
	}
}

func TestListProfilesStatuses(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	mustMkdir(t, filepath.Join(env.profilesBase(), "work"))
	stubKeychain(t, true)
	t.Setenv("CLAUDE_PROFILE", "work")

	entries := ListProfiles(env.load(t), env.profilesBase())
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Sorted: personal, work.
	personal, work := entries[0], entries[1]
	if personal.Name != "personal" || work.Name != "work" {
		t.Fatalf("unexpected order: %s, %s", personal.Name, work.Name)
	}
	if personal.Installed || personal.Authenticated {
		t.Errorf("personal should be not installed: %+v", personal)
	}
	if !work.Installed || !work.Authenticated || work.CredentialSource != "keychain" || !work.Current {
		t.Errorf("work should be installed+authenticated via keychain and current: %+v", work)
	}

	buf := captureOutput(t)
	RenderProfileList(entries)
	if !strings.Contains(buf.String(), "* claude-work") || !strings.Contains(buf.String(), "[authenticated]") {
		t.Errorf("unexpected render: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "[not installed]") {
		t.Errorf("personal should render as not installed: %q", buf.String())
	}
}

func TestListProfilesNotAuthenticatedWithoutKeychainOrFile(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	mustMkdir(t, filepath.Join(env.profilesBase(), "work"))
	stubKeychain(t, false)

	entries := ListProfiles(env.load(t), env.profilesBase())
	if entries[1].Authenticated {
		t.Errorf("work should not be authenticated: %+v", entries[1])
	}
}

func TestWhichFromEnv(t *testing.T) {
	t.Setenv("CLAUDE_PROFILE", "work")
	t.Setenv("CLAUDE_CONFIG_DIR", "/p/work")
	r := Which(t.TempDir())
	if !r.Active || r.Profile != "work" || r.Source != "env" || r.ConfigDir != "/p/work" || r.Command != "claude-work" {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestWhichFromFile(t *testing.T) {
	t.Setenv("CLAUDE_PROFILE", "")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".claude-profile"), "personal\n")
	sub := filepath.Join(dir, "a", "b")
	mustMkdir(t, sub)

	r := Which(sub)
	if r.Active || r.Profile != "personal" || r.Source != "file" {
		t.Errorf("unexpected result: %+v", r)
	}
	buf := captureOutput(t)
	RenderWhich(r)
	if !strings.Contains(buf.String(), "not active in this shell") {
		t.Errorf("render should hint about activation: %q", buf.String())
	}
}

func TestWhichNone(t *testing.T) {
	t.Setenv("CLAUDE_PROFILE", "")
	r := Which(t.TempDir())
	if r.Active || r.Profile != "" {
		t.Errorf("expected empty result, got %+v", r)
	}
}

func TestDoctorReportCountsAndExitCondition(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	cfg := env.load(t)
	cfg.SourceDir = filepath.Join(env.Root, "missing") // forces an error check
	stubKeychain(t, false)

	report := DoctorReportFor(cfg, env.profilesBase())
	if report.OK || report.Errors == 0 {
		t.Errorf("expected failing report, got %+v", report)
	}
	buf := captureOutput(t)
	RenderDoctorReport(report)
	if !strings.Contains(buf.String(), "Some checks failed") {
		t.Errorf("unexpected render: %q", buf.String())
	}
}

func TestBuildRunExecArgsAndEnv(t *testing.T) {
	env := newTestEnv(t, `[profiles.work]
model = "sonnet"
add_dirs = ["/extra"]
env = { FOO = "bar" }
`)
	// Fake claude on PATH.
	fakeBin := t.TempDir()
	mustWrite(t, filepath.Join(fakeBin, "claude"), "#!/bin/sh\n")
	os.Chmod(filepath.Join(fakeBin, "claude"), 0o755)
	t.Setenv("PATH", fakeBin)
	t.Setenv("CLAUDE_PROFILE", "stale")
	t.Setenv("ANTHROPIC_API_KEY", "leak")

	spec, err := BuildRunExec(env.load(t), env.profilesBase(), "work", []string{"-p", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	wantArgv := []string{"claude", "--add-dir", "/extra", "--model", "sonnet", "-p", "hi"}
	if strings.Join(spec.Argv, " ") != strings.Join(wantArgv, " ") {
		t.Errorf("argv = %v, want %v", spec.Argv, wantArgv)
	}
	joined := strings.Join(spec.Env, "\n")
	if strings.Contains(joined, "CLAUDE_PROFILE=stale") || strings.Contains(joined, "ANTHROPIC_API_KEY=leak") {
		t.Errorf("inherited CLAUDE_/ANTHROPIC_ vars leaked: %s", joined)
	}
	for _, want := range []string{"CLAUDE_PROFILE=work", "FOO=bar", "CLAUDE_CONFIG_DIR=" + filepath.Join(env.profilesBase(), "work")} {
		if !strings.Contains(joined, want) {
			t.Errorf("env missing %s", want)
		}
	}

	// User-supplied --model wins.
	spec, _ = BuildRunExec(env.load(t), env.profilesBase(), "work", []string{"--model", "opus"})
	if strings.Count(strings.Join(spec.Argv, " "), "--model") != 1 {
		t.Errorf("profile model should be skipped when user passes --model: %v", spec.Argv)
	}

	if _, err := BuildRunExec(env.load(t), env.profilesBase(), "nope", nil); err == nil {
		t.Error("unknown profile should error")
	}
}

func TestUseAutoAndDirenv(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".claude-profile"), "work\n")

	snippet, err := Use(env.load(t), env.profilesBase(), "auto", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snippet, `CLAUDE_PROFILE='work'`) {
		t.Errorf("auto should resolve to work: %q", snippet)
	}
	if _, err := Use(env.load(t), env.profilesBase(), "auto", t.TempDir()); err == nil {
		t.Error("auto without .claude-profile should error")
	}
	if _, err := Use(env.load(t), env.profilesBase(), "nope", dir); err == nil {
		t.Error("unknown profile should error")
	}

	out, err := Direnv(env.load(t), env.profilesBase(), "personal")
	if err != nil || !strings.Contains(out, `CLAUDE_PROFILE='personal'`) {
		t.Errorf("direnv: %v %q", err, out)
	}
}

func TestLinkAndUnlink(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	dir := t.TempDir()
	captureOutput(t)

	if err := Link(env.load(t), dir, "nope"); err == nil {
		t.Error("unknown profile should error")
	}
	if err := Link(env.load(t), dir, "work"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".claude-profile"))
	if strings.TrimSpace(string(data)) != "work" {
		t.Errorf(".claude-profile = %q", data)
	}
	if err := Unlink(dir); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(dir, ".claude-profile")) {
		t.Error("unlink should remove the file")
	}
}

func TestCheckDivergenceDeterministicOrder(t *testing.T) {
	env := newTestEnv(t, `[profiles.b]
[profiles.a]
[profiles.c]
`)
	mustWrite(t, filepath.Join(env.SourceDir, "CLAUDE.md"), "base\n")
	for _, n := range []string{"a", "b", "c"} {
		mustWrite(t, filepath.Join(env.profilesBase(), n, "CLAUDE.md"), "base\nlocal "+n+"\n")
	}
	for i := 0; i < 5; i++ {
		d := CheckDivergence(env.load(t), env.profilesBase())
		if len(d) != 3 || d[0].Profile != "a" || d[1].Profile != "b" || d[2].Profile != "c" {
			t.Fatalf("iteration %d: unexpected order %v", i, d)
		}
	}
}

func TestSortedProfileNames(t *testing.T) {
	cfg := &Config{Profiles: map[string]*Profile{"z": {}, "a": {}, "m": {}}}
	got := strings.Join(SortedProfileNames(cfg), ",")
	if got != "a,m,z" {
		t.Errorf("got %s", got)
	}
}
