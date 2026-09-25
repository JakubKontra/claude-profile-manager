package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateUseOutputFish(t *testing.T) {
	out := GenerateUseOutput("work", "/p/work", &Profile{Env: map[string]string{"K": "v"}}, UseOptions{Shell: "fish"})
	for _, want := range []string{
		"set -gx CLAUDE_CONFIG_DIR '/p/work';",
		"set -gx CLAUDE_PROFILE 'work';",
		"set -gx K 'v';",
		"set -gx CPM_MANAGED_VARS CLAUDE_CONFIG_DIR CLAUDE_PROFILE K;",
		"echo 'Switched to profile: work';",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("fish output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "export ") {
		t.Error("fish output must not use export")
	}
	quiet := GenerateUseOutput("work", "/p/work", &Profile{}, UseOptions{Shell: "fish", Quiet: true})
	if strings.Contains(quiet, "echo") {
		t.Error("quiet must not echo")
	}
}

func TestGenerateUseOutputPosixManagedVars(t *testing.T) {
	out := GenerateUseOutput("work", "/p/work", &Profile{Env: map[string]string{"K": "v"}}, UseOptions{Quiet: true})
	if !strings.Contains(out, `export CPM_MANAGED_VARS='CLAUDE_CONFIG_DIR CLAUDE_PROFILE K';`) {
		t.Errorf("managed vars not exported:\n%s", out)
	}
	if strings.Contains(out, "echo") {
		t.Error("quiet must not echo")
	}
	if strings.Contains(out, "grep -E '^(CLAUDE_|ANTHROPIC_)'") {
		t.Error("use must not unset every CLAUDE_/ANTHROPIC_ variable")
	}
}

func TestDetectShell(t *testing.T) {
	if DetectShell(func(string) string { return "/usr/local/bin/fish" }) != "fish" {
		t.Error("fish not detected")
	}
	if DetectShell(func(string) string { return "/bin/zsh" }) != "posix" {
		t.Error("zsh should be posix")
	}
	if DetectShell(func(string) string { return "" }) != "posix" {
		t.Error("empty SHELL should be posix")
	}
}

func TestGenerateShellHookVariants(t *testing.T) {
	bash := GenerateShellHook(HookOptions{})
	if !strings.Contains(bash, "PROMPT_COMMAND") || strings.Contains(bash, "alias cd") {
		t.Error("bash hook should use PROMPT_COMMAND, not alias cd")
	}
	if strings.Contains(bash, "grep -E '^(CLAUDE_|ANTHROPIC_)'") {
		t.Error("hook must only unset managed vars")
	}
	if strings.Contains(bash, "target='personal'") {
		t.Error("no default unless requested")
	}

	withDefault := GenerateShellHook(HookOptions{DefaultProfile: "personal"})
	if !strings.Contains(withDefault, `if [ -z "$target" ]; then target='personal'; fi`) {
		t.Errorf("default profile missing:\n%s", withDefault)
	}

	fish := GenerateShellHook(HookOptions{Shell: "fish", DefaultProfile: "work"})
	for _, want := range []string{"function _cpm_auto_switch --on-variable PWD", "cpm use --shell fish --quiet", "set target 'work'", "set -e CPM_MANAGED_VARS"} {
		if !strings.Contains(fish, want) {
			t.Errorf("fish hook missing %q:\n%s", want, fish)
		}
	}
}

// fakeCPM installs a `cpm` shim whose `use` prints a real use snippet for
// the requested profile, so hooks can be executed in a subshell.
func fakeCPM(t *testing.T, profiles map[string]*Profile) string {
	t.Helper()
	dir := t.TempDir()
	snippets := filepath.Join(dir, "snippets")
	for name, p := range profiles {
		mustWrite(t, filepath.Join(snippets, name), GenerateUseOutput(name, "/p/"+name, p, UseOptions{Quiet: true}))
	}
	script := `#!/usr/bin/env bash
# args: use [--quiet] <name>
name="${@: -1}"
f="` + snippets + `/$name"
[ -f "$f" ] || exit 1
cat "$f"
`
	mustWrite(t, filepath.Join(dir, "cpm"), script)
	os.Chmod(filepath.Join(dir, "cpm"), 0o755)
	return dir
}

func TestHookExecutionUnsetsOnlyManagedVars(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	shim := fakeCPM(t, map[string]*Profile{
		"work":     {Env: map[string]string{"ANTHROPIC_BASE_URL": "https://work"}},
		"personal": {},
	})
	root := t.TempDir()
	project := filepath.Join(root, "project")
	mustWrite(t, filepath.Join(project, ".claude-profile"), "work\n")
	outside := filepath.Join(root, "outside")
	mustMkdir(t, outside)

	hook := filepath.Join(root, "hook.sh")
	mustWrite(t, hook, GenerateShellHook(HookOptions{}))

	script := `
export CLAUDE_MY_OWN=1
export ANTHROPIC_MINE=keep
cd "$PROJECT"
source "$HOOK"
printf 'IN:%s|%s|%s|%s\n' "${CLAUDE_PROFILE:-}" "${ANTHROPIC_BASE_URL:-}" "${CLAUDE_MY_OWN:-}" "${ANTHROPIC_MINE:-}"
cd "$OUTSIDE"; _cpm_prompt_hook
printf 'OUT:%s|%s|%s|%s\n' "${CLAUDE_PROFILE:-}" "${ANTHROPIC_BASE_URL:-}" "${CLAUDE_MY_OWN:-}" "${ANTHROPIC_MINE:-}"
`
	cmd := exec.Command(bash, "-c", script)
	cmd.Env = append(os.Environ(),
		"PATH="+shim+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PROJECT="+project, "OUTSIDE="+outside, "HOOK="+hook,
		"CLAUDE_PROFILE=", "CPM_MANAGED_VARS=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hook script failed: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "[cpm] using profile: work\n") || strings.Count(text, "using profile") != 1 {
		t.Errorf("switch message should appear exactly once:\n%s", text)
	}
	if !strings.Contains(text, "IN:work|https://work|1|keep\n") {
		t.Errorf("inside project: profile vars set, user vars kept:\n%s", text)
	}
	if !strings.Contains(text, "OUT:||1|keep\n") {
		t.Errorf("outside project: only managed vars unset:\n%s", text)
	}
	if !strings.Contains(text, "[cpm] profile unset") {
		t.Errorf("unset message missing:\n%s", text)
	}
}

func TestHookExecutionDefaultProfile(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	shim := fakeCPM(t, map[string]*Profile{"personal": {}})
	root := t.TempDir()
	hook := filepath.Join(root, "hook.sh")
	mustWrite(t, hook, GenerateShellHook(HookOptions{DefaultProfile: "personal"}))

	cmd := exec.Command(bash, "-c", `cd "$ROOT"; source "$HOOK"; printf 'P:%s\n' "${CLAUDE_PROFILE:-}"`)
	cmd.Env = append(os.Environ(), "PATH="+shim+string(os.PathListSeparator)+os.Getenv("PATH"), "ROOT="+root, "HOOK="+hook, "CLAUDE_PROFILE=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(string(out), "P:personal\n") {
		t.Errorf("default profile not applied:\n%s", out)
	}
}

func TestFishHookSyntax(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not available")
	}
	hook := filepath.Join(t.TempDir(), "hook.fish")
	mustWrite(t, hook, GenerateShellHook(HookOptions{Shell: "fish", DefaultProfile: "x"}))
	if out, err := exec.Command(fish, "--no-execute", hook).CombinedOutput(); err != nil {
		t.Fatalf("fish -n failed: %v\n%s", err, out)
	}
	use := filepath.Join(t.TempDir(), "use.fish")
	mustWrite(t, use, GenerateUseOutput("w", "/p", &Profile{Env: map[string]string{"K": "it's"}}, UseOptions{Shell: "fish"}))
	if out, err := exec.Command(fish, "--no-execute", use).CombinedOutput(); err != nil {
		t.Fatalf("fish -n on use output failed: %v\n%s", err, out)
	}
}

func TestSelectProfileInteractive(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	cfg := env.load(t)
	var prompt strings.Builder

	name, err := SelectProfileInteractive(cfg, strings.NewReader("2\n"), &prompt)
	if err != nil || name != "work" {
		t.Errorf("numeric choice: %q %v", name, err)
	}
	if !strings.Contains(prompt.String(), "1) personal") || !strings.Contains(prompt.String(), "2) work") {
		t.Errorf("menu: %q", prompt.String())
	}
	name, err = SelectProfileInteractive(cfg, strings.NewReader("personal\n"), &prompt)
	if err != nil || name != "personal" {
		t.Errorf("name choice: %q %v", name, err)
	}
	for _, bad := range []string{"\n", "9\n", "nope\n", ""} {
		if _, err := SelectProfileInteractive(cfg, strings.NewReader(bad), &prompt); err == nil {
			t.Errorf("input %q should error", bad)
		}
	}
}
