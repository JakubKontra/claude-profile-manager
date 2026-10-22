package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDocsUpToDate fails when the committed docs/cli no longer matches the
// commands. Regenerate with: go run . docs --markdown docs/cli
func TestDocsUpToDate(t *testing.T) {
	root := NewRoot()
	root.InitDefaultCompletionCmd()
	tmp := t.TempDir()
	if err := writeDocs(root, tmp, ".md", renderMarkdown); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Glob(filepath.Join(tmp, "*.md"))
	have, _ := filepath.Glob(filepath.Join("..", "docs", "cli", "*.md"))
	if len(want) != len(have) {
		t.Errorf("docs/cli has %d files, the commands need %d (go run . docs --markdown docs/cli)", len(have), len(want))
	}
	for _, path := range want {
		generated, _ := os.ReadFile(path)
		committed, err := os.ReadFile(filepath.Join("..", "docs", "cli", filepath.Base(path)))
		if err != nil || string(committed) != string(generated) {
			t.Errorf("docs/cli/%s is out of date (go run . docs --markdown docs/cli)", filepath.Base(path))
		}
	}
}

func TestManPages(t *testing.T) {
	root := NewRoot()
	root.InitDefaultCompletionCmd()
	dir := t.TempDir()
	if err := writeDocs(root, dir, ".1", renderMan); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "cpm-cloud-push.1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".TH CPM-CLOUD-PUSH 1", `.B \-\-dry\-run`, "cpm\\-cloud(1)"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("man page missing %q:\n%s", want, page)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "cpm-resolve.1")); err == nil {
		t.Error("hidden commands must not get a man page")
	}
}
