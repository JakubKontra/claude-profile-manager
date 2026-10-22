package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRemote(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"git@github.com:company/app.git":             "github.com/company/app",
		"git@GitHub.com:Company/app":                 "github.com/Company/app",
		"https://github.com/company/app.git":         "github.com/company/app",
		"https://user:token@github.com/company/app/": "github.com/company/app",
		"ssh://git@github.com:22/company/app.git":    "github.com/company/app",
		"ssh://git@gitlab.example.com/group/sub/app": "gitlab.example.com/group/sub/app",
		"github.com/company/*":                       "github.com/company/*",
		"git@github.com:company/*":                   "github.com/company/*",
		"/srv/git/app.git":                           "/srv/git/app",
	} {
		if got := NormalizeRemote(in); got != want {
			t.Errorf("NormalizeRemote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"github.com/company/*", "github.com/company/app", true},
		{"github.com/company/*", "github.com/company/group/app", false},
		{"github.com/company/**", "github.com/company/group/app", true},
		{"github.com/company/*", "github.com/company-other/app", false},
		{"github.com/*/app", "github.com/anyone/app", true},
		{"/home/me/Work/client/**", "/home/me/Work/client", true},
		{"/home/me/Work/client/**", "/home/me/Work/client/a/b", true},
		{"/home/me/Work/client/**", "/home/me/Work/clientele", false},
		{"/home/me/Work/*", "/home/me/Work/a/b", false},
		{"/home/me/Work/repo-?", "/home/me/Work/repo-1", true},
		{"/home/me/Work/a.b", "/home/me/Work/axb", false},
	}
	for _, c := range cases {
		if got := globMatch(c.pattern, c.name); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

// writeRepo creates dir/.git/config with the given remotes.
func writeRepo(t *testing.T, dir string, urls ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("[core]\n\tbare = false\n")
	for i, url := range urls {
		name := "origin"
		if i > 0 {
			name = "fork" + string(rune('a'+i))
		}
		b.WriteString("[remote \"" + name + "\"]\n\turl = " + url + "\n\tfetch = +refs/heads/*:refs/remotes/" + name + "/*\n")
	}
	mustWrite(t, filepath.Join(dir, ".git", "config"), b.String())
}

func TestGitRemoteURLs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	writeRepo(t, repo, "git@github.com:company/app.git", "https://github.com/me/app")
	sub := filepath.Join(repo, "src", "pkg")
	mustMkdir(t, sub)

	got := gitRemoteURLs(sub)
	if strings.Join(got, " ") != "git@github.com:company/app.git https://github.com/me/app" {
		t.Errorf("remotes from a subdirectory = %v", got)
	}
	if got := gitRemoteURLs(filepath.Join(root)); got != nil {
		t.Errorf("outside a repo: %v", got)
	}

	// A linked worktree has a .git file pointing at .git/worktrees/<name>,
	// whose commondir leads back to the main repo's config.
	wtGitDir := filepath.Join(repo, ".git", "worktrees", "feature")
	mustWrite(t, filepath.Join(wtGitDir, "commondir"), "../..\n")
	worktree := filepath.Join(root, "feature")
	mustWrite(t, filepath.Join(worktree, ".git"), "gitdir: "+wtGitDir+"\n")
	if got := gitRemoteURLs(worktree); len(got) != 2 {
		t.Errorf("worktree should share the main repo's remotes: %v", got)
	}

	// A submodule's .git file points at its own git dir, relative.
	subGitDir := filepath.Join(repo, ".git", "modules", "lib")
	mustWrite(t, filepath.Join(subGitDir, "config"), "[remote \"origin\"]\n\turl = \"git@github.com:vendor/lib.git\"\n")
	module := filepath.Join(repo, "lib")
	mustWrite(t, filepath.Join(module, ".git"), "gitdir: ../.git/modules/lib\n")
	if got := gitRemoteURLs(module); len(got) != 1 || got[0] != "git@github.com:vendor/lib.git" {
		t.Errorf("submodule remotes = %v", got)
	}
}

func TestGitRemoteURLsMatchesGit(t *testing.T) {
	t.Parallel()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "remote", "add", "origin", "git@github.com:company/app.git"},
		{"-C", repo, "remote", "add", "upstream", "https://github.com/upstream/app"},
	} {
		if out, err := gitCommand(t, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", git, args, err, out)
		}
	}
	got := gitRemoteURLs(repo)
	if strings.Join(got, " ") != "git@github.com:company/app.git https://github.com/upstream/app" {
		t.Errorf("remotes written by git = %v", got)
	}
}

func TestResolveProfileOrder(t *testing.T) {
	t.Parallel()
	app, _ := testApp(t)
	work := filepath.Join(app.Home, "Work")
	repo := filepath.Join(work, "client", "app")
	writeRepo(t, repo, "git@github.com:company/app.git")
	other := filepath.Join(app.Home, "src", "oss")
	writeRepo(t, other, "https://github.com/someone/oss")

	cfg := &Config{Match: []MatchRule{
		{Remote: "github.com/company/*", Path: "~/Elsewhere/**", Profile: "never"},
		{Remote: "github.com/company/*", Profile: "work"},
		{Path: "~/Work/client/**", Profile: "client"},
		{Path: "~/src/**", Profile: "oss"},
	}}

	res, ok := app.ResolveProfile(cfg, filepath.Join(repo, "src"))
	if !ok || res.Profile != "work" || res.Source != "match" || res.Rule != "remote github.com/company/*" {
		t.Errorf("first matching rule should win, and remote and path must both match: %+v", res)
	}

	res, ok = app.ResolveProfile(cfg, filepath.Join(work, "client"))
	if !ok || res.Profile != "client" || res.Rule != "path ~/Work/client/**" {
		t.Errorf("path rule: %+v", res)
	}

	mustWrite(t, filepath.Join(repo, ".claude-profile"), "pinned\n")
	if res, _ := app.ResolveProfile(cfg, repo); res.Profile != "pinned" || res.Source != "file" {
		t.Errorf(".claude-profile must beat [[match]]: %+v", res)
	}

	if res, ok := app.ResolveProfile(cfg, other); !ok || res.Profile != "oss" {
		t.Errorf("path rule for a repo without matching remote: %+v", res)
	}
	if _, ok := app.ResolveProfile(cfg, t.TempDir()); ok {
		t.Error("nothing should match an unrelated directory")
	}
	if _, ok := app.ResolveProfile(&Config{}, repo+"/.."); ok {
		t.Error("without rules or files nothing resolves")
	}
}

func TestMatchRulesValidated(t *testing.T) {
	t.Parallel()
	for body, want := range map[string]string{
		"[[match]]\nremote = 'github.com/x/*'\n": "profile is required",
		"[[match]]\nprofile = 'work'\n":          "set remote, path or both",
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		mustWrite(t, path, "[profiles.work]\n"+body)
		if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("config %q: err = %v, want %q", body, err, want)
		}
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	mustWrite(t, path, "[profiles.work]\n[[match]]\npath = '~/Work/**'\nprofile = 'work'\n")
	cfg, err := LoadConfig(path)
	if err != nil || len(cfg.Match) != 1 || len(cfg.Undecoded) != 0 {
		t.Errorf("valid rule: cfg=%+v err=%v", cfg, err)
	}
}

func TestUseAutoAndWhichWithMatch(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, twoProfiles+"\n[[match]]\nremote = 'github.com/company/*'\nprofile = 'work'\n\n[[match]]\npath = '~/gone/**'\nprofile = 'missing'\n")
	app, buf := env.App, env.Out
	repo := filepath.Join(env.Root, "app")
	writeRepo(t, repo, "git@github.com:company/app.git")
	cfg := env.load(t)

	snippet, err := app.Use(cfg, "auto", repo, UseOptions{})
	if err != nil || !strings.Contains(snippet, "CLAUDE_PROFILE='work'") {
		t.Fatalf("use auto via [[match]]: %v\n%s", err, snippet)
	}

	r := app.Which(cfg, repo)
	if r.Active || r.Profile != "work" || r.Source != "match" || r.Rule != "remote github.com/company/*" {
		t.Errorf("which: %+v", r)
	}
	app.RenderWhich(r)
	if !strings.Contains(buf.String(), "Source:     [[match]] remote github.com/company/*") {
		t.Errorf("render: %q", buf.String())
	}

	gone := filepath.Join(env.Root, "gone", "x")
	mustMkdir(t, gone)
	if _, err := app.Use(cfg, "auto", gone, UseOptions{}); err == nil || !strings.Contains(err.Error(), `profile "missing" from [[match]] path ~/gone/**`) {
		t.Errorf("rule with an unknown profile: %v", err)
	}
	if _, err := app.Use(cfg, "auto", t.TempDir(), UseOptions{}); err == nil || !strings.Contains(err.Error(), "no [[match]] rule applies") {
		t.Errorf("nothing applies: %v", err)
	}

	report := app.DoctorReportFor(cfg, DoctorOptions{LookPath: noLookPath})
	if c, _ := findCheck(report.Checks, "match #2"); c.Status != "error" || !strings.Contains(c.Detail, `"missing"`) {
		t.Errorf("doctor should flag the rule with an unknown profile: %+v", c)
	}
	if _, found := findCheck(report.Checks, "match #1"); found {
		t.Error("a valid rule needs no check")
	}
}

func TestHookWithResolveAndConfig(t *testing.T) {
	t.Parallel()
	opts := HookOptions{Resolve: true, ConfigPath: "/cfg/it's.toml"}
	posix := GenerateShellHook(opts)
	if !strings.Contains(posix, `target="$(cpm --config '/cfg/it'\''s.toml' resolve "$PWD" 2>/dev/null)"`) ||
		!strings.Contains(posix, `snippet="$(cpm --config '/cfg/it'\''s.toml' use --quiet "$target" 2>/dev/null)"`) ||
		strings.Contains(posix, ".claude-profile\" ]") {
		t.Errorf("resolve hook should delegate to cpm with --config:\n%s", posix)
	}
	opts.Shell = "fish"
	if fish := GenerateShellHook(opts); !strings.Contains(fish, "set -l target (cpm --config '/cfg/it'\\''s.toml' resolve \"$PWD\" 2>/dev/null)") {
		t.Errorf("fish resolve hook:\n%s", fish)
	}
	if plain := GenerateShellHook(HookOptions{}); strings.Contains(plain, "resolve") || strings.Contains(plain, "--config") {
		t.Errorf("default hook must stay pure shell:\n%s", plain)
	}
}

func TestHookExecutionWithResolve(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	shim := fakeCPM(t, map[string]*Profile{"work": {}})
	// The fake cpm answers `resolve` from a file the test controls.
	root := t.TempDir()
	answer := filepath.Join(root, "answer")
	data, _ := os.ReadFile(filepath.Join(shim, "cpm"))
	mustWrite(t, filepath.Join(shim, "cpm"), strings.Replace(string(data), "\n", "\nif [ \"$1\" = resolve ]; then cat \""+answer+"\" 2>/dev/null; exit 0; fi\n", 1))
	os.Chmod(filepath.Join(shim, "cpm"), 0o755)
	hook := filepath.Join(root, "hook.sh")
	mustWrite(t, hook, GenerateShellHook(HookOptions{Resolve: true}))
	mustWrite(t, answer, "work\n")

	script := `source "$HOOK"; printf 'A:%s\n' "${CLAUDE_PROFILE:-}"; rm "$ANSWER"; cd /; _cpm_prompt_hook; printf 'B:%s\n' "${CLAUDE_PROFILE:-}"`
	cmd := exec.Command(bash, "-c", script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+shim+string(os.PathListSeparator)+os.Getenv("PATH"), "HOOK="+hook, "ANSWER="+answer, "CLAUDE_PROFILE=", "CPM_MANAGED_VARS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "A:work\n") || !strings.Contains(text, "B:\n") || !strings.Contains(text, "no .claude-profile or [[match]] rule") {
		t.Errorf("hook should follow cpm resolve:\n%s", text)
	}
}
