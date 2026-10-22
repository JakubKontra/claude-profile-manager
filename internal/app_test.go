package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppLookPath(t *testing.T) {
	t.Parallel()
	bin, other := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(bin, "tool"), "#!/bin/sh\n")
	os.Chmod(filepath.Join(bin, "tool"), 0o755)
	mustWrite(t, filepath.Join(other, "plain"), "not executable")

	app, _ := testApp(t)
	app.Env = []string{"PATH=relative" + string(os.PathListSeparator) + other + string(os.PathListSeparator) + bin}
	if got, err := app.lookPath("tool"); err != nil || got != filepath.Join(bin, "tool") {
		t.Errorf("lookPath(tool) = %q, %v", got, err)
	}
	if _, err := app.lookPath("plain"); err == nil {
		t.Error("a file without the executable bit must not be found")
	}
	if got, err := app.lookPath(filepath.Join(bin, "tool")); err != nil || got != filepath.Join(bin, "tool") {
		t.Errorf("an absolute path should be returned as is: %q, %v", got, err)
	}

	app.Env = nil
	if _, err := app.lookPath("tool"); err == nil {
		t.Error("an empty PATH finds nothing")
	}
}
