package internal

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// testApp returns an App with no config, for helpers that only print.
func testApp(t *testing.T) (*App, *bytes.Buffer) {
	t.Helper()
	return testAppAt(t, "")
}

// testAppAt returns an App for configPath whose output goes to the returned
// buffer. Errors and prompts go to a separate buffer so tests see only stdout.
func testAppAt(t *testing.T, configPath string) (*App, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	return &App{
		ConfigPath: configPath,
		BaseDir:    ProfilesBaseDir(configPath),
		In:         strings.NewReader(""),
		Out:        &out,
		Err:        &bytes.Buffer{},
		Run:        ExecRunner{},
		Keychain:   newFakeKeychain(),
	}, &out
}

// fakeKeychain is an in-memory KeychainStore keyed by service name, so tests
// never reach the real macOS Keychain.
type fakeKeychain struct {
	mu      sync.Mutex
	entries map[string][]byte
	deleted []string
	// readErr, when set, is returned by Read.
	readErr error
}

func newFakeKeychain() *fakeKeychain {
	return &fakeKeychain{entries: map[string][]byte{}}
}

// login stores secret as the Keychain login of the profile at profileDir.
func (k *fakeKeychain) login(profileDir, secret string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.entries[KeychainServiceName(profileDir)] = []byte(secret)
}

func (k *fakeKeychain) Exists(service, _ string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	_, ok := k.entries[service]
	return ok
}

func (k *fakeKeychain) Read(service, _ string) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.readErr != nil {
		return nil, k.readErr
	}
	data, ok := k.entries[service]
	if !ok {
		return nil, fmt.Errorf("no keychain entry for %s", service)
	}
	return data, nil
}

func (k *fakeKeychain) Delete(service, _ string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.deleted = append(k.deleted, service)
	delete(k.entries, service)
	return nil
}

// testEnv is a self-contained cpm layout inside a temp dir.
type testEnv struct {
	Root       string
	SourceDir  string
	BinDir     string
	ConfigPath string
	// App runs commands against this layout; its output lands in Out.
	App *App
	Out *bytes.Buffer
	// Keychain is the App's in-memory Keychain.
	Keychain *fakeKeychain
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
	env.App, env.Out = testAppAt(t, env.ConfigPath)
	env.Keychain = env.App.Keychain.(*fakeKeychain)
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
