package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLooksLikeSecret(t *testing.T) {
	yes := [][2]string{
		{"ANTHROPIC_API_KEY", "abc"},
		{"GITHUB_TOKEN", "x"},
		{"DB_PASSWORD", "x"},
		{"MY_SECRET", "x"},
		{"FOO", "sk-ant-api03-xxxx"},
		{"FOO", "ghp_abcdef"},
		{"FOO", "xoxb-123"},
		{"FOO", "AKIAABCDEFGHIJKLMNOP"},
	}
	no := [][2]string{
		{"ANTHROPIC_API_KEY", ""},
		{"CLAUDE_CODE_USE_VERTEX", "1"},
		{"CLOUD_ML_REGION", "europe-west1"},
		{"KEYBOARD", "us"},
		{"PATH", "/usr/bin"},
	}
	for _, kv := range yes {
		if !looksLikeSecret(kv[0], kv[1]) {
			t.Errorf("%s=%s should look like a secret", kv[0], kv[1])
		}
	}
	for _, kv := range no {
		if looksLikeSecret(kv[0], kv[1]) {
			t.Errorf("%s=%s should not look like a secret", kv[0], kv[1])
		}
	}
}

func TestScanForSecretsFindsEnvInSettings(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "settings.json"), `{"env": {"ANTHROPIC_API_KEY": "sk-ant-1", "CLAUDE_CODE_USE_VERTEX": "1"}}`)
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "ANTHROPIC_API_KEY=sk-ant-not-json")
	mustWrite(t, filepath.Join(dir, "commands", "x.md"), "sk-ant-in-a-command")
	files := map[string]string{
		"settings.json": filepath.Join(dir, "settings.json"),
		"CLAUDE.md":     filepath.Join(dir, "CLAUDE.md"),
		"commands/x.md": filepath.Join(dir, "commands", "x.md"),
	}
	findings := ScanForSecrets(files)
	if len(findings) != 1 || findings[0].Key != "ANTHROPIC_API_KEY" || findings[0].File != "settings.json" {
		t.Errorf("unexpected findings: %+v", findings)
	}
}

func TestSyncFilesForIncludeAndSkills(t *testing.T) {
	cfg := &Config{Cloud: &CloudConfig{Include: []string{"settings.local.json", "../etc/passwd", "/abs", "./notes.md"}}}
	got := strings.Join(syncFilesFor(cfg), ",")
	if got != "settings.json,CLAUDE.md,settings.local.json,notes.md" {
		t.Errorf("syncFilesFor = %s", got)
	}
	if strings.Join(syncFilesFor(&Config{}), ",") != "settings.json,CLAUDE.md" {
		t.Error("settings.local.json must not be synced by default")
	}
	found := false
	for _, d := range cloudSyncDirs {
		if d == "skills" {
			found = true
		}
	}
	if !found {
		t.Error("skills/ should be synced")
	}
}

func TestGatherSyncFilesHonoursInclude(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "claude")
	mustWrite(t, filepath.Join(src, "settings.local.json"), `{}`)
	mustWrite(t, filepath.Join(src, "skills", "s", "SKILL.md"), "skill")
	t.Setenv("HOME", root)

	files, _ := GatherSyncFiles(&Config{SourceDir: src}, "")
	if _, ok := files["settings.local.json"]; ok {
		t.Error("settings.local.json gathered without include")
	}
	if _, ok := files[filepath.Join("skills", "s", "SKILL.md")]; !ok {
		t.Error("skills not gathered")
	}
	files, _ = GatherSyncFiles(&Config{SourceDir: src, Cloud: &CloudConfig{Include: []string{"settings.local.json"}}}, "")
	if _, ok := files["settings.local.json"]; !ok {
		t.Error("included file not gathered")
	}
}

func TestCloudPushBlocksOnSecretsAndDryRun(t *testing.T) {
	requireGit(t)
	buf := captureOutput(t)
	env := newCloudEnv(t, "[profiles.a]\n")
	env.activate(t)
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"env": {"ANTHROPIC_API_KEY": "sk-ant-x"}}`)

	// Init commits the initial state; the secret check applies to push only.
	if err := CloudInit(env.ConfigPath, ""); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(env.SourceDir, "CLAUDE.md"), "changed")

	err := CloudPush(env.ConfigPath, PushOptions{})
	if err == nil || !strings.Contains(buf.String(), "ANTHROPIC_API_KEY") {
		t.Fatalf("push should refuse on secrets: err=%v out=%q", err, buf.String())
	}
	if out, _ := exec.Command("git", "-C", CloudRepoDir(env.ConfigPath), "log", "--oneline").Output(); strings.Count(string(out), "\n") != 1 {
		t.Errorf("no commit must be made when refused: %s", out)
	}

	buf.Reset()
	if err := CloudPush(env.ConfigPath, PushOptions{DryRun: true, AllowSecrets: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Dry run") || !strings.Contains(buf.String(), "CLAUDE.md") {
		t.Errorf("dry run output: %q", buf.String())
	}
	if out, _ := exec.Command("git", "-C", CloudRepoDir(env.ConfigPath), "log", "--oneline").Output(); strings.Count(string(out), "\n") != 1 {
		t.Errorf("dry run must not commit: %s", out)
	}

	buf.Reset()
	if err := CloudDiff(env.ConfigPath); err != nil || !strings.Contains(buf.String(), "CLAUDE.md") {
		t.Errorf("diff: %v %q", err, buf.String())
	}

	// allow_secrets in config also unblocks.
	mustWrite(t, env.ConfigPath, "source_dir = \""+env.SourceDir+"\"\n[profiles.a]\n[cloud]\nallow_secrets = true\n")
	if err := CloudPush(env.ConfigPath, PushOptions{}); err != nil {
		t.Fatalf("push with allow_secrets: %v", err)
	}
}

func TestCloudPullResyncsProfiles(t *testing.T) {
	requireGit(t)
	captureOutput(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	exec.Command("git", "init", "--bare", bare).Run()

	a := newCloudEnv(t, "[profiles.work]\n")
	a.activate(t)
	mustWrite(t, filepath.Join(a.SourceDir, "settings.json"), `{"v": 1}`)
	if err := CloudInit(a.ConfigPath, bare); err != nil {
		t.Fatal(err)
	}
	if err := CloudPush(a.ConfigPath, PushOptions{}); err != nil {
		t.Fatal(err)
	}

	b := newCloudEnv(t, "[profiles.work]\n")
	b.activate(t)
	if err := CloudInit(b.ConfigPath, bare); err != nil {
		t.Fatal(err)
	}
	if err := InstallProfiles(b.load(t), b.ConfigPath, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	profileSettings := filepath.Join(b.profilesBase(), "work", "settings.json")
	if data, _ := os.ReadFile(profileSettings); string(data) != `{"v": 1}` {
		t.Fatalf("profile settings = %q", data)
	}

	a.activate(t)
	mustWrite(t, filepath.Join(a.SourceDir, "settings.json"), `{"v": 2}`)
	if err := CloudPush(a.ConfigPath, PushOptions{}); err != nil {
		t.Fatal(err)
	}

	b.activate(t)
	if err := CloudPull(b.ConfigPath, false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(profileSettings); string(data) != `{"v": 2}` {
		t.Errorf("profile copy not re-synced after pull: %q", data)
	}
}

func TestInstallProfilesAutoPullAndPush(t *testing.T) {
	requireGit(t)
	buf := captureOutput(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	exec.Command("git", "init", "--bare", bare).Run()

	a := newCloudEnv(t, "[profiles.work]\n")
	a.activate(t)
	mustWrite(t, filepath.Join(a.SourceDir, "CLAUDE.md"), "from A")
	if err := CloudInit(a.ConfigPath, bare); err != nil {
		t.Fatal(err)
	}
	if err := CloudPush(a.ConfigPath, PushOptions{}); err != nil {
		t.Fatal(err)
	}

	b := newCloudEnv(t, "[profiles.work]\n")
	b.activate(t)
	if err := CloudInit(b.ConfigPath, bare); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, b.ConfigPath, "source_dir = \""+b.SourceDir+"\"\nbin_dir = \""+b.BinDir+"\"\n[profiles.work]\n[cloud]\nremote = \""+bare+"\"\nauto_pull_on_install = true\nauto_push = true\n")

	a.activate(t)
	mustWrite(t, filepath.Join(a.SourceDir, "CLAUDE.md"), "from A v2")
	if err := CloudPush(a.ConfigPath, PushOptions{}); err != nil {
		t.Fatal(err)
	}

	b.activate(t)
	mustWrite(t, filepath.Join(b.SourceDir, "commands", "b.md"), "made on B")
	buf.Reset()
	if err := InstallProfiles(b.load(t), b.ConfigPath, InstallOptions{}); err != nil {
		t.Fatalf("install with auto pull/push: %v\n%s", err, buf.String())
	}
	if data, _ := os.ReadFile(filepath.Join(b.SourceDir, "CLAUDE.md")); string(data) != "from A v2" {
		t.Errorf("auto pull did not fetch A's change: %q", data)
	}
	if !strings.Contains(buf.String(), "Pushed to remote") {
		t.Errorf("auto push did not run: %q", buf.String())
	}

	a.activate(t)
	if err := CloudPull(a.ConfigPath, false); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(a.SourceDir, "commands", "b.md")) {
		t.Error("A did not receive B's auto-pushed command")
	}
}
