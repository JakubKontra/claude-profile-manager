package internal

import (
	"os"
	"path/filepath"
	"testing"
)

// writeGitConfig makes a repository whose .git/config holds cfg.
func writeGitConfig(t *testing.T, cfg string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// git config accepts these forms and reads the same URL from each; the hook
// reads the file itself, so it has to agree with git.
func TestGitRemoteURLsReadsWhatGitReads(t *testing.T) {
	t.Parallel()
	const want = "git@github.com:work/app.git"
	for name, cfg := range map[string]string{
		"plain":               "[remote \"origin\"]\n\turl = git@github.com:work/app.git\n",
		"quoted":              "[remote \"origin\"]\n\turl = \"git@github.com:work/app.git\"\n",
		"hash comment":        "[remote \"origin\"]\n\turl = git@github.com:work/app.git # work account\n",
		"semicolon comment":   "[remote \"origin\"]\n\turl = git@github.com:work/app.git ; work account\n",
		"quoted then comment": "[remote \"origin\"]\n\turl = \"git@github.com:work/app.git\" # work\n",
		"on the section line": "[remote \"origin\"] url = git@github.com:work/app.git\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := gitRemoteURLs(writeGitConfig(t, cfg))
			if len(got) != 1 || got[0] != want {
				t.Errorf("gitRemoteURLs = %q, want [%q]", got, want)
			}
		})
	}
}

func TestGitRemoteURLsKeepsACommentCharacterInsideQuotes(t *testing.T) {
	t.Parallel()
	got := gitRemoteURLs(writeGitConfig(t, "[remote \"origin\"]\n\turl = \"https://example.com/a#b/app.git\"\n"))
	if len(got) != 1 || got[0] != "https://example.com/a#b/app.git" {
		t.Errorf("gitRemoteURLs = %q", got)
	}
}

// A glob such as github.com/work/* hides the problem (its * takes the comment
// too); a rule that names the repository does not.
func TestAGitCommentedRemoteStillMatchesARuleThatNamesTheRepo(t *testing.T) {
	t.Parallel()
	dir := writeGitConfig(t, "[remote \"origin\"]\n\turl = git@github.com:work/app.git # work account\n")
	if !remoteMatches("github.com/work/app", gitRemoteURLs(dir)) {
		t.Error("a remote rule for github.com/work/app did not match a repo whose url line has a trailing comment")
	}
}
