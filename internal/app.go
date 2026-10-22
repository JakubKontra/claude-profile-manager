package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// App carries what commands need from their environment: the config
// location, the streams they read from and write to, and the external
// programs and stores they talk to. Tests build one around a temp dir, a
// buffer and fakes instead of swapping globals.
type App struct {
	// ConfigPath is the config.toml path as given (may start with ~).
	ConfigPath string
	// BaseDir is the directory holding config.toml and the profiles.
	BaseDir string

	In  io.Reader
	Out io.Writer
	Err io.Writer

	// Run executes external programs (git).
	Run Runner
	// Keychain is the OS credential store; a no-op outside macOS.
	Keychain KeychainStore

	// Home is the user's home directory; ~ in paths expands to it.
	Home string
	// Env is the environment as KEY=value pairs, read through Getenv.
	Env []string
}

// NewApp returns an App for configPath wired to the process's stdio.
func NewApp(configPath string) *App {
	home, _ := os.UserHomeDir()
	return &App{
		ConfigPath: configPath,
		BaseDir:    filepath.Dir(expandHome(home, configPath)),
		In:         os.Stdin,
		Out:        os.Stdout,
		Err:        os.Stderr,
		Run:        ExecRunner{},
		Keychain:   platformKeychain(),
		Home:       home,
		Env:        os.Environ(),
	}
}

// LoadConfig reads and validates the app's config; at least one profile is required.
func (a *App) LoadConfig() (*Config, error) { return loadConfig(a.Home, a.ConfigPath, true) }

// LoadCloudConfig loads the app's config without requiring profiles.
func (a *App) LoadCloudConfig() (*Config, error) { return loadConfig(a.Home, a.ConfigPath, false) }

// Getenv returns the value of key in a.Env, or "" when it is unset. A later
// entry wins, as with exec.Cmd.Env.
func (a *App) Getenv(key string) string {
	prefix := key + "="
	for i := len(a.Env) - 1; i >= 0; i-- {
		if strings.HasPrefix(a.Env[i], prefix) {
			return a.Env[i][len(prefix):]
		}
	}
	return ""
}

// expand resolves a leading ~ against a.Home.
func (a *App) expand(p string) string { return expandHome(a.Home, p) }

// lookPath finds an executable like exec.LookPath, but searches the PATH in
// a.Env instead of the process's.
func (a *App) lookPath(name string) (string, error) {
	if strings.Contains(name, "/") {
		if isExecutable(name) {
			return name, nil
		}
		return "", fmt.Errorf("%s: not an executable file", name)
	}
	for _, dir := range filepath.SplitList(a.Getenv("PATH")) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue // relative PATH entries are ignored, as exec.LookPath does
		}
		if path := filepath.Join(dir, name); isExecutable(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s: executable file not found in $PATH", name)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

// PrintJSON writes v as indented JSON to the app's output.
func (a *App) PrintJSON(v any) error {
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (a *App) outf(format string, args ...any) { fmt.Fprintf(a.Out, format, args...) }
func (a *App) outln(args ...any)               { fmt.Fprintln(a.Out, args...) }
func (a *App) out(args ...any)                 { fmt.Fprint(a.Out, args...) }
