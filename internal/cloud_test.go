package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatherSyncFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0o755)

	// Create some test files
	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{"test": true}`), 0o644)
	os.WriteFile(filepath.Join(sourceDir, "CLAUDE.md"), []byte("# Test"), 0o644)

	// Create commands dir
	cmdDir := filepath.Join(sourceDir, "commands")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "test.md"), []byte("test command"), 0o644)

	cfg := &Config{SourceDir: sourceDir}

	// Use a config path that doesn't exist (no cpm/config.toml)
	files, err := GatherSyncFiles(cfg, filepath.Join(tmpDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := files["settings.json"]; !ok {
		t.Error("expected settings.json in gathered files")
	}
	if _, ok := files["CLAUDE.md"]; !ok {
		t.Error("expected CLAUDE.md in gathered files")
	}
	if _, ok := files["commands/test.md"]; !ok {
		t.Error("expected commands/test.md in gathered files")
	}
	// settings.local.json should not be present (doesn't exist)
	if _, ok := files["settings.local.json"]; ok {
		t.Error("did not expect settings.local.json (file doesn't exist)")
	}
}

func TestGatherSyncFilesWithExclude(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0o755)

	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(sourceDir, "CLAUDE.md"), []byte("# Test"), 0o644)

	cfg := &Config{
		SourceDir: sourceDir,
		Cloud: &CloudConfig{
			Exclude: []string{"CLAUDE.md"},
		},
	}

	files, err := GatherSyncFiles(cfg, filepath.Join(tmpDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := files["CLAUDE.md"]; ok {
		t.Error("CLAUDE.md should be excluded")
	}
	if _, ok := files["settings.json"]; !ok {
		t.Error("settings.json should still be included")
	}
}

func TestGatherSyncFilesDirExclude(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	cmdDir := filepath.Join(sourceDir, "commands")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "test.md"), []byte("test"), 0o644)

	cfg := &Config{
		SourceDir: sourceDir,
		Cloud: &CloudConfig{
			Exclude: []string{"commands/"},
		},
	}

	files, err := GatherSyncFiles(cfg, filepath.Join(tmpDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := files["commands/test.md"]; ok {
		t.Error("commands/ directory should be excluded")
	}
}

func TestDistributeSyncFilesRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	repoDir := filepath.Join(tmpDir, "repo")
	restoreDir := filepath.Join(tmpDir, "restore")

	os.MkdirAll(sourceDir, 0o755)
	os.MkdirAll(repoDir, 0o755)
	os.MkdirAll(restoreDir, 0o755)

	// Create source files
	os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(`{"key": "value"}`), 0o644)

	cmdDir := filepath.Join(sourceDir, "commands")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "hello.md"), []byte("hello world"), 0o644)

	// Gather files
	cfg := &Config{SourceDir: sourceDir}
	files, _ := GatherSyncFiles(cfg, filepath.Join(tmpDir, "config.toml"))

	// Copy to repo
	for repoPath, srcPath := range files {
		dst := filepath.Join(repoDir, repoPath)
		os.MkdirAll(filepath.Dir(dst), 0o755)
		copyFile(srcPath, dst)
	}

	// Distribute from repo to restore dir
	restoreCfg := &Config{SourceDir: restoreDir}
	DistributeSyncFiles(repoDir, restoreCfg)

	// Check files were restored
	data, err := os.ReadFile(filepath.Join(restoreDir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not restored: %v", err)
	}
	if string(data) != `{"key": "value"}` {
		t.Errorf("settings.json content mismatch: %s", data)
	}

	data, err = os.ReadFile(filepath.Join(restoreDir, "commands", "hello.md"))
	if err != nil {
		t.Fatalf("commands/hello.md not restored: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("commands/hello.md content mismatch: %s", data)
	}
}

func TestFilesAreDifferent(t *testing.T) {
	tmpDir := t.TempDir()

	fileA := filepath.Join(tmpDir, "a.txt")
	fileB := filepath.Join(tmpDir, "b.txt")
	fileC := filepath.Join(tmpDir, "c.txt")

	os.WriteFile(fileA, []byte("same"), 0o644)
	os.WriteFile(fileB, []byte("same"), 0o644)
	os.WriteFile(fileC, []byte("different"), 0o644)

	if filesAreDifferent(fileA, fileB) {
		t.Error("identical files reported as different")
	}
	if !filesAreDifferent(fileA, fileC) {
		t.Error("different files reported as same")
	}
	if !filesAreDifferent(fileA, filepath.Join(tmpDir, "nonexistent")) {
		t.Error("existing vs nonexistent should be different")
	}
}

func TestIsExcluded(t *testing.T) {
	cfg := &Config{
		Cloud: &CloudConfig{
			Exclude: []string{"CLAUDE.md", "commands/"},
		},
	}

	if !isExcluded("CLAUDE.md", cfg) {
		t.Error("CLAUDE.md should be excluded")
	}
	if !isExcluded("commands/", cfg) {
		t.Error("commands/ should be excluded")
	}
	if isExcluded("settings.json", cfg) {
		t.Error("settings.json should not be excluded")
	}

	// nil cloud config
	nilCfg := &Config{}
	if isExcluded("CLAUDE.md", nilCfg) {
		t.Error("nothing should be excluded with nil cloud config")
	}
}

func TestCleanDeletedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo", "commands")
	srcDir := filepath.Join(tmpDir, "src", "commands")

	os.MkdirAll(repoDir, 0o755)
	os.MkdirAll(srcDir, 0o755)

	// File exists in both
	os.WriteFile(filepath.Join(repoDir, "keep.md"), []byte("keep"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "keep.md"), []byte("keep"), 0o644)

	// File exists only in repo (was deleted from source)
	os.WriteFile(filepath.Join(repoDir, "deleted.md"), []byte("deleted"), 0o644)

	cleanDeletedFiles(repoDir, srcDir)

	if _, err := os.Stat(filepath.Join(repoDir, "keep.md")); os.IsNotExist(err) {
		t.Error("keep.md should still exist in repo")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "deleted.md")); !os.IsNotExist(err) {
		t.Error("deleted.md should have been removed from repo")
	}
}

func TestSaveCloudRemoteWithCommentedHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte(userConfig), 0o644)

	if err := saveCloudRemote(path, "git@github.com:me/settings.git"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadCloudConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cloud == nil || cfg.Cloud.Remote != "git@github.com:me/settings.git" {
		data, _ := os.ReadFile(path)
		t.Errorf("remote not saved:\n%s", data)
	}

	// Update in place, no duplicate key.
	if err := saveCloudRemote(path, "git@github.com:me/other.git"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "\nremote = ") != 1 {
		t.Errorf("remote duplicated:\n%s", data)
	}
	cfg, _ = LoadCloudConfig(path)
	if cfg.Cloud.Remote != "git@github.com:me/other.git" {
		t.Errorf("remote not updated: %s", cfg.Cloud.Remote)
	}
}

func TestSaveCloudRemoteCreatesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := saveCloudRemote(path, "url"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadCloudConfig(path)
	if err != nil || cfg.Cloud == nil || cfg.Cloud.Remote != "url" {
		t.Errorf("config not created: %v %+v", err, cfg)
	}
}

func TestMergeConfigTOMLAddsNewProfiles(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "config.toml")
	pulled := filepath.Join(dir, "pulled.toml")
	os.WriteFile(local, []byte(userConfig), 0o644)
	os.WriteFile(pulled, []byte(`source_dir = "/elsewhere"
[profiles.work]
description = "changed remotely"
[profiles.client]
description = "Client"
model = "opus"
add_dirs = ["~/c"]
[profiles.client.env]
K = "v"
[cloud]
remote = "git@example:x.git"
`), 0o644)
	captureOutput(t)

	if err := mergeConfigTOML(pulled, local); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(local)
	if err != nil {
		t.Fatal(err)
	}
	client := cfg.Profiles["client"]
	if client == nil || client.Model != "opus" || client.Env["K"] != "v" || len(client.AddDirs) != 1 {
		t.Errorf("client profile not merged fully: %+v", client)
	}
	if cfg.Profiles["work"].Description != "Company team subscription" {
		t.Error("existing profile must not be overwritten")
	}
	if cfg.SourceDir != ExpandPath("~/.claude") {
		t.Errorf("local source_dir must be preserved, got %s", cfg.SourceDir)
	}
	if cfg.Cloud == nil || cfg.Cloud.Remote != "git@example:x.git" {
		t.Errorf("cloud section not merged: %+v", cfg.Cloud)
	}

	// Second merge is a no-op.
	before, _ := os.ReadFile(local)
	if err := mergeConfigTOML(pulled, local); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(local)
	if string(before) != string(after) {
		t.Error("merge must be idempotent")
	}
}

func TestPlanDistributeKindsExcludeAndDeletes(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	live := filepath.Join(root, "live")
	t.Setenv("HOME", filepath.Join(root, "home"))

	mustWrite(t, filepath.Join(repo, "settings.json"), `{"a":1}`)
	mustWrite(t, filepath.Join(repo, "CLAUDE.md"), "new")
	mustWrite(t, filepath.Join(repo, "commands", "keep.md"), "same")
	mustWrite(t, filepath.Join(repo, "commands", "changed.md"), "v2")
	mustWrite(t, filepath.Join(repo, "agents", "a.md"), "agent")
	mustWrite(t, filepath.Join(repo, "cpm", "config.toml"), "[profiles.x]\n")

	mustWrite(t, filepath.Join(live, "CLAUDE.md"), "old")
	mustWrite(t, filepath.Join(live, "commands", "keep.md"), "same")
	mustWrite(t, filepath.Join(live, "commands", "changed.md"), "v1")
	mustWrite(t, filepath.Join(live, "commands", "gone.md"), "deleted remotely")
	mustWrite(t, filepath.Join(live, "agents", "local-only.md"), "excluded dir, must survive")
	mustWrite(t, filepath.Join(live, "orphan.json"), "top-level, never deleted")

	cfg := &Config{SourceDir: live, Cloud: &CloudConfig{Exclude: []string{"CLAUDE.md", "agents/"}}}
	plan := PlanDistribute(repo, cfg, filepath.Join(root, "config.toml"))

	kinds := map[string]string{}
	for _, a := range plan {
		kinds[a.RepoPath] = a.Kind
	}
	want := map[string]string{
		"settings.json":       "create",
		"CLAUDE.md":           "excluded",
		"commands/keep.md":    "unchanged",
		"commands/changed.md": "update",
		"commands/gone.md":    "delete",
		"agents/a.md":         "excluded",
		"cpm/config.toml":     "merge",
	}
	for path, kind := range want {
		if kinds[path] != kind {
			t.Errorf("%s: kind = %q, want %q", path, kinds[path], kind)
		}
	}
	if _, planned := kinds["agents/local-only.md"]; planned {
		t.Error("excluded directory must not get deletions")
	}
	if _, planned := kinds["orphan.json"]; planned {
		t.Error("top-level files are never deleted")
	}

	captureOutput(t)
	if err := ApplyDistribute(plan); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(live, "CLAUDE.md")); string(data) != "old" {
		t.Error("excluded file was overwritten")
	}
	if data, _ := os.ReadFile(filepath.Join(live, "commands", "changed.md")); string(data) != "v2" {
		t.Error("updated file not written")
	}
	if fileExists(filepath.Join(live, "commands", "gone.md")) {
		t.Error("remotely deleted file not removed")
	}
	if !fileExists(filepath.Join(live, "agents", "local-only.md")) || !fileExists(filepath.Join(live, "orphan.json")) {
		t.Error("protected files were deleted")
	}
	if !fileExists(filepath.Join(live, "settings.json")) {
		t.Error("created file missing")
	}
	cfgOut, err := LoadConfig(filepath.Join(root, "config.toml"))
	if err != nil || cfgOut.Profiles["x"] == nil {
		t.Errorf("config merge not applied: %v", err)
	}
}

func TestPlanDistributeSkipsMissingRepoDirs(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "live")
	mustWrite(t, filepath.Join(live, "commands", "mine.md"), "x")
	t.Setenv("HOME", root)
	plan := PlanDistribute(filepath.Join(root, "empty-repo"), &Config{SourceDir: live}, "")
	if len(plan) != 0 {
		t.Errorf("a repo without commands/ must not delete local commands: %v", plan)
	}
}

// cloudEnv is one "machine" for cloud round-trip tests.
type cloudEnv struct {
	testEnv
	Home string
}

func newCloudEnv(t *testing.T, profiles string) cloudEnv {
	t.Helper()
	env := newTestEnv(t, profiles)
	home := filepath.Join(env.Root, "home")
	mustMkdir(t, home)
	return cloudEnv{testEnv: env, Home: home}
}

func (c cloudEnv) activate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", c.Home)
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("GIT_AUTHOR_NAME", "cpm test")
	t.Setenv("GIT_AUTHOR_EMAIL", "cpm@test")
	t.Setenv("GIT_COMMITTER_NAME", "cpm test")
	t.Setenv("GIT_COMMITTER_EMAIL", "cpm@test")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func TestCloudPushPullRoundTrip(t *testing.T) {
	requireGit(t)
	captureOutput(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	// Machine A: fresh init against an empty remote, then push.
	a := newCloudEnv(t, "[profiles.work]\ndescription = \"Work\"\n")
	a.activate(t)
	mustWrite(t, filepath.Join(a.SourceDir, "settings.json"), `{"from": "A"}`)
	mustWrite(t, filepath.Join(a.SourceDir, "commands", "one.md"), "one")
	mustWrite(t, filepath.Join(a.SourceDir, "commands", "two.md"), "two")
	if err := CloudInit(a.ConfigPath, bare); err != nil {
		t.Fatalf("init A: %v", err)
	}
	if !hasOriginRemote(CloudRepoDir(a.ConfigPath)) {
		t.Fatal("remote must be set on a fresh init")
	}
	if err := CloudPush(a.ConfigPath, "first"); err != nil {
		t.Fatalf("push A: %v", err)
	}

	// Machine B: clone, files are distributed and the profile merged.
	b := newCloudEnv(t, "[profiles.personal]\n")
	b.activate(t)
	if err := CloudInit(b.ConfigPath, bare); err != nil {
		t.Fatalf("init B: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(b.SourceDir, "settings.json")); string(data) != `{"from": "A"}` {
		t.Errorf("B did not receive settings.json: %q", data)
	}
	if !fileExists(filepath.Join(b.SourceDir, "commands", "two.md")) {
		t.Error("B did not receive commands/two.md")
	}
	cfgB := b.load(t)
	if cfgB.Profiles["work"] == nil || cfgB.Profiles["personal"] == nil {
		t.Errorf("B config should have both profiles: %v", cfgB.Profiles)
	}
	if cfgB.Cloud == nil || cfgB.Cloud.Remote != bare {
		t.Errorf("B remote not saved: %+v", cfgB.Cloud)
	}

	// A changes and deletes; B pulls.
	a.activate(t)
	mustWrite(t, filepath.Join(a.SourceDir, "settings.json"), `{"from": "A2"}`)
	os.Remove(filepath.Join(a.SourceDir, "commands", "two.md"))
	if err := CloudPush(a.ConfigPath, ""); err != nil {
		t.Fatalf("push A2: %v", err)
	}
	if err := CloudPush(a.ConfigPath, ""); err != nil {
		t.Fatalf("no-op push: %v", err)
	}

	b.activate(t)
	if err := CloudPull(b.ConfigPath, true); err != nil {
		t.Fatalf("dry-run pull B: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(b.SourceDir, "settings.json")); string(data) != `{"from": "A"}` {
		t.Error("dry run must not change files")
	}
	if err := CloudPull(b.ConfigPath, false); err != nil {
		t.Fatalf("pull B: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(b.SourceDir, "settings.json")); string(data) != `{"from": "A2"}` {
		t.Errorf("B not updated: %q", data)
	}
	if fileExists(filepath.Join(b.SourceDir, "commands", "two.md")) {
		t.Error("file deleted on A still present on B")
	}
	if err := CloudStatus(b.ConfigPath); err != nil {
		t.Errorf("status: %v", err)
	}
}

func TestCloudInitRemoteUnreachableErrors(t *testing.T) {
	requireGit(t)
	captureOutput(t)
	env := newCloudEnv(t, "[profiles.a]\n")
	env.activate(t)
	err := CloudInit(env.ConfigPath, filepath.Join(env.Root, "does-not-exist.git"))
	if err == nil || !strings.Contains(err.Error(), "cannot reach remote") {
		t.Fatalf("expected unreachable remote error, got %v", err)
	}
	if fileExists(filepath.Join(CloudRepoDir(env.ConfigPath), ".git")) {
		t.Error("no repo should be created when the remote is unreachable")
	}
}
