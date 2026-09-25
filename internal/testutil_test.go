package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// captureOutput redirects command output into a buffer for the test.
func captureOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := SetOutput(&buf)
	t.Cleanup(func() { SetOutput(prev) })
	return &buf
}

// testEnv is a self-contained cpm layout inside a temp dir.
type testEnv struct {
	Root       string
	SourceDir  string
	BinDir     string
	ConfigPath string
}

// newTestEnv creates source/bin dirs and writes config.toml with the given
// body appended after source_dir/bin_dir.
func newTestEnv(t *testing.T, profilesTOML string) testEnv {
	t.Helper()
	root := t.TempDir()
	env := testEnv{
		Root:       root,
		SourceDir:  filepath.Join(root, "claude"),
		BinDir:     filepath.Join(root, "bin"),
		ConfigPath: filepath.Join(root, "profiles", "config.toml"),
	}
	mustMkdir(t, env.SourceDir)
	mustMkdir(t, env.BinDir)
	mustMkdir(t, filepath.Dir(env.ConfigPath))
	writeTestConfig(t, env, profilesTOML)
	return env
}

func writeTestConfig(t *testing.T, env testEnv, profilesTOML string) {
	t.Helper()
	content := "source_dir = \"" + env.SourceDir + "\"\nbin_dir = \"" + env.BinDir + "\"\n\n" + profilesTOML
	if err := os.WriteFile(env.ConfigPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e testEnv) load(t *testing.T) *Config {
	t.Helper()
	cfg, err := LoadConfig(e.ConfigPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

func (e testEnv) profilesBase() string { return filepath.Dir(e.ConfigPath) }

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
