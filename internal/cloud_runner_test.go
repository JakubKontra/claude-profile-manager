package internal

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// newFakeGitEnv returns an env with an initialized-looking cloud repo whose
// git calls are answered by a fake runner.
func newFakeGitEnv(t *testing.T, replies map[string]fakeReply) (testEnv, *fakeRunner, string) {
	t.Helper()
	env := newTestEnv(t, "[profiles.work]\n")
	repoDir := CloudRepoDir(env.ConfigPath)
	mustMkdir(t, filepath.Join(repoDir, ".git"))
	run := newFakeRunner(replies)
	env.App.Run = run
	return env, run, repoDir
}

func TestCloudPushReportsPushFailure(t *testing.T) {
	t.Parallel()
	env, run, repoDir := newFakeGitEnv(t, map[string]fakeReply{
		"git status --porcelain":         {out: " M config.toml\n"},
		"git push -u origin HEAD":        {out: "! [rejected]", err: errors.New("exit status 1")},
		"git remote get-url origin":      {out: "git@example.com:me/cpm-sync.git\n"},
		"git show --stat --format= HEAD": {out: " config.toml | 1 +\n"},
	})

	err := env.App.CloudPush(PushOptions{Message: "sync"})
	if err == nil || !strings.Contains(err.Error(), "push failed") || !strings.Contains(err.Error(), repoDir) {
		t.Fatalf("push failure not reported with the repo path: %v", err)
	}
	calls := strings.Join(run.calls, "\n")
	if !strings.Contains(calls, "git commit -m sync") {
		t.Errorf("changes should be committed before pushing:\n%s", calls)
	}
	if fileExists(cloudStatePath(repoDir)) {
		t.Error("a failed push must not record a synced state")
	}
}

func TestCloudPullFailureSuggestsReset(t *testing.T) {
	t.Parallel()
	env, _, _ := newFakeGitEnv(t, map[string]fakeReply{
		"git pull --ff-only origin HEAD":  {err: errors.New("exit status 128")},
		"git rev-parse --abbrev-ref HEAD": {out: "trunk\n"},
	})

	err := env.App.CloudPull(false)
	if err == nil || !strings.Contains(err.Error(), "git reset --hard origin/trunk") {
		t.Fatalf("pull failure should suggest resetting to the current branch: %v", err)
	}
}

func TestCheckRemoteHasRefs(t *testing.T) {
	t.Parallel()
	app, _ := testApp(t)
	app.Run = newFakeRunner(map[string]fakeReply{
		"git ls-remote --heads empty":   {},
		"git ls-remote --heads full":    {out: "abc123\trefs/heads/main\n"},
		"git ls-remote --heads offline": {out: "fatal: could not read from remote\n", err: errors.New("exit status 128")},
	})

	if has, err := app.checkRemoteHasRefs("empty"); has || err != nil {
		t.Errorf("empty remote: has=%v err=%v", has, err)
	}
	if has, err := app.checkRemoteHasRefs("full"); !has || err != nil {
		t.Errorf("remote with branches: has=%v err=%v", has, err)
	}
	if _, err := app.checkRemoteHasRefs("offline"); err == nil || err.Error() != "fatal: could not read from remote" {
		t.Errorf("unreachable remote should return git's message: %v", err)
	}
}
