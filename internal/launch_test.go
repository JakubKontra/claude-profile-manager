package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":      `'plain'`,
		"":           `''`,
		"it's":       `'it'\''s'`,
		"$HOME":      `'$HOME'`,
		`say "hi"`:   `'say "hi"'`,
		"a b":        `'a b'`,
		"`cmd`":      "'`cmd`'",
		`back\slash`: `'back\slash'`,
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestLaunchSpecClaudeArgs(t *testing.T) {
	spec := BuildLaunchSpec("work", "/p/work", &Profile{
		Model:   "sonnet",
		AddDirs: []string{"/extra"},
	})

	got := strings.Join(spec.ClaudeArgs([]string{"-p", "hi"}), " ")
	if got != "--add-dir /extra --model sonnet" {
		t.Errorf("args = %q", got)
	}
	if args := spec.ClaudeArgs([]string{"--model=opus"}); strings.Contains(strings.Join(args, " "), "sonnet") {
		t.Errorf("profile model must not override user --model=: %v", args)
	}
	if args := spec.ClaudeArgs([]string{"auth", "login"}); args != nil {
		t.Errorf("bypass subcommand must get no flags: %v", args)
	}
	if args := spec.ClaudeArgs([]string{"mcp"}); args != nil {
		t.Errorf("mcp must be bypassed: %v", args)
	}
}

func TestLaunchSpecEnviron(t *testing.T) {
	spec := BuildLaunchSpec("work", "/p/work", &Profile{Env: map[string]string{"ZED": "1", "ALPHA": "2"}})
	base := []string{"PATH=/bin", "CLAUDE_PROFILE=old", "ANTHROPIC_API_KEY=x", "HOME=/h"}
	got := spec.Environ(base)
	want := []string{"PATH=/bin", "HOME=/h", "CLAUDE_CONFIG_DIR=/p/work", "CLAUDE_PROFILE=work", "ALPHA=2", "ZED=1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Environ = %v, want %v", got, want)
	}
}

func TestGenerateWrapperBashSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	profile := &Profile{
		Model:   "sonnet",
		AddDirs: []string{"/tmp/with space"},
		Env:     map[string]string{"WEIRD": `a"b$c'd`},
	}
	script := filepath.Join(t.TempDir(), "claude-test")
	mustWrite(t, script, GenerateWrapper("test", "/p/test", profile))
	if out, err := exec.Command(bash, "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, out)
	}
}

// fakeClaude installs a fake `claude` that prints its argv and selected env.
func fakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/usr/bin/env bash
for a in "$@"; do printf 'ARG:%s\n' "$a"; done
printf 'ENV:CLAUDE_PROFILE=%s\n' "${CLAUDE_PROFILE:-}"
printf 'ENV:CLAUDE_CONFIG_DIR=%s\n' "${CLAUDE_CONFIG_DIR:-}"
printf 'ENV:WEIRD=%s\n' "${WEIRD:-}"
printf 'ENV:ANTHROPIC_API_KEY=%s\n' "${ANTHROPIC_API_KEY:-}"
`
	mustWrite(t, filepath.Join(dir, "claude"), script)
	os.Chmod(filepath.Join(dir, "claude"), 0o755)
	return dir
}

func runWrapper(t *testing.T, profile *Profile, args ...string) string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	fakeDir := fakeClaude(t)
	script := filepath.Join(t.TempDir(), "claude-test")
	mustWrite(t, script, GenerateWrapper("test", "/p/test", profile))

	cmd := exec.Command(bash, append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"), "ANTHROPIC_API_KEY=leak")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed: %v\n%s", err, out)
	}
	return string(out)
}

func TestGenerateWrapperExecutesWithQuotedValues(t *testing.T) {
	profile := &Profile{
		Model:   "sonnet",
		AddDirs: []string{"/tmp/with space"},
		Env:     map[string]string{"WEIRD": `a"b$c'd`},
	}
	out := runWrapper(t, profile, "-p", "hello world")

	wantLines := []string{
		"ARG:--add-dir", "ARG:/tmp/with space", "ARG:--model", "ARG:sonnet", "ARG:-p", "ARG:hello world",
		"ENV:CLAUDE_PROFILE=test", "ENV:CLAUDE_CONFIG_DIR=/p/test", `ENV:WEIRD=a"b$c'd`, "ENV:ANTHROPIC_API_KEY=",
	}
	for _, w := range wantLines {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("output missing line %q:\n%s", w, out)
		}
	}
}

func TestGenerateWrapperExecutesBypassAndNoExtras(t *testing.T) {
	// No add_dirs, no model: the empty array must not trip `set -u`.
	out := runWrapper(t, &Profile{}, "-p", "x")
	if strings.Contains(out, "ARG:--model") || !strings.Contains(out, "ARG:-p\n") {
		t.Errorf("unexpected args:\n%s", out)
	}

	// Bypass subcommand gets no flags even with model/add_dirs.
	out = runWrapper(t, &Profile{Model: "sonnet", AddDirs: []string{"/x"}}, "auth", "status")
	if strings.Contains(out, "ARG:--model") || strings.Contains(out, "ARG:--add-dir") {
		t.Errorf("bypass subcommand received profile flags:\n%s", out)
	}
	if !strings.Contains(out, "ARG:auth\nARG:status\n") {
		t.Errorf("subcommand args lost:\n%s", out)
	}

	// User --model wins.
	out = runWrapper(t, &Profile{Model: "sonnet"}, "--model", "opus")
	if strings.Contains(out, "ARG:sonnet") || !strings.Contains(out, "ARG:opus") {
		t.Errorf("user model should win:\n%s", out)
	}
}

func TestGenerateUseOutputEvaluates(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	snippet := GenerateUseOutput("work", "/p/work", &Profile{Env: map[string]string{"WEIRD": `it's "x" $y`}})
	script := snippet + "\nprintf '%s|%s|%s' \"$CLAUDE_PROFILE\" \"$CLAUDE_CONFIG_DIR\" \"$WEIRD\"\n"
	cmd := exec.Command(bash, "-c", script)
	cmd.Env = append(os.Environ(), "CLAUDE_PROFILE=old")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval failed: %v\n%s", err, out)
	}
	if !strings.HasSuffix(string(out), `work|/p/work|it's "x" $y`) {
		t.Errorf("unexpected eval result: %q", out)
	}
}

func TestBuildExecExec(t *testing.T) {
	env := newTestEnv(t, `[profiles.work]
env = { FOO = "bar" }
`)
	spec, err := BuildExecExec(env.load(t), env.profilesBase(), "work", []string{"env"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(spec.Path) != "env" || spec.Argv[0] != "env" {
		t.Errorf("unexpected spec: %+v", spec)
	}
	joined := strings.Join(spec.Env, "\n")
	if !strings.Contains(joined, "FOO=bar") || !strings.Contains(joined, "CLAUDE_PROFILE=work") {
		t.Errorf("env missing profile vars: %s", joined)
	}
	if _, err := BuildExecExec(env.load(t), env.profilesBase(), "work", nil); err == nil {
		t.Error("empty command should error")
	}
	if _, err := BuildExecExec(env.load(t), env.profilesBase(), "work", []string{"definitely-not-a-binary-xyz"}); err == nil {
		t.Error("missing binary should error")
	}
}
