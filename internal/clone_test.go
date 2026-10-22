package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloneProfile(t *testing.T) {
	env := newTestEnv(t, "[profiles.original]\ndescription = \"Original\"\nmodel = \"sonnet\"\n")
	srcProfile := filepath.Join(env.profilesBase(), "original")
	mustWrite(t, filepath.Join(srcProfile, "settings.json"), `{"test": true}`)
	mustWrite(t, filepath.Join(srcProfile, "CLAUDE.md"), "# Claude")
	for _, dir := range []string{"skills", "plugins"} {
		mustMkdir(t, filepath.Join(env.SourceDir, dir))
	}
	app, _ := testAppAt(t, env.ConfigPath)

	if err := app.CloneProfile("original", "cloned", env.load(t)); err != nil {
		t.Fatalf("CloneProfile failed: %v", err)
	}

	clonedDir := filepath.Join(env.profilesBase(), "cloned")
	data, err := os.ReadFile(filepath.Join(clonedDir, "settings.json"))
	if err != nil || string(data) != `{"test": true}` {
		t.Errorf("cloned settings.json = %q (%v)", data, err)
	}
	for _, dir := range []string{"skills", "plugins"} {
		target, err := os.Readlink(filepath.Join(clonedDir, dir))
		if err != nil || target != filepath.Join(env.SourceDir, dir) {
			t.Errorf("%s symlink = %q (%v)", dir, target, err)
		}
	}

	// The clone is registered in config.toml with the source's settings.
	cfg := env.load(t)
	cloned := cfg.Profiles["cloned"]
	if cloned == nil || cloned.Model != "sonnet" || !strings.Contains(cloned.Description, "Original") {
		t.Errorf("clone not registered in config: %+v", cloned)
	}
}

func TestCloneProfileErrors(t *testing.T) {
	env := newTestEnv(t, "[profiles.source]\n[profiles.taken]\n")
	cfg := env.load(t)
	app, _ := testAppAt(t, env.ConfigPath)

	if err := app.CloneProfile("missing", "new", cfg); err == nil {
		t.Error("expected error for unknown source profile")
	}
	if err := app.CloneProfile("source", "new", cfg); err == nil {
		t.Error("expected error for uninstalled source profile")
	}
	mustMkdir(t, filepath.Join(env.profilesBase(), "source"))
	if err := app.CloneProfile("source", "taken", cfg); err == nil {
		t.Error("expected error when target exists in config")
	}
	if err := app.CloneProfile("source", "bad name", cfg); err == nil {
		t.Error("expected error for invalid target name")
	}
	mustMkdir(t, filepath.Join(env.profilesBase(), "dirtaken"))
	if err := app.CloneProfile("source", "dirtaken", cfg); err == nil {
		t.Error("expected error when target directory exists")
	}
}

func TestCloneProfileDoesNotCopyCredentials(t *testing.T) {
	env := newTestEnv(t, "[profiles.original]\n")
	srcProfile := filepath.Join(env.profilesBase(), "original")
	mustWrite(t, filepath.Join(srcProfile, ".credentials.json"), `{"token": "secret"}`)
	mustWrite(t, filepath.Join(srcProfile, "settings.json"), `{}`)
	app, _ := testAppAt(t, env.ConfigPath)

	if err := app.CloneProfile("original", "cloned", env.load(t)); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(env.profilesBase(), "cloned", ".credentials.json")) {
		t.Error("credentials should NOT be cloned")
	}
}
