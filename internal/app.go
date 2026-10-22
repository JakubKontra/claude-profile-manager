package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
}

// NewApp returns an App for configPath wired to the process's stdio.
func NewApp(configPath string) *App {
	return &App{
		ConfigPath: configPath,
		BaseDir:    ProfilesBaseDir(configPath),
		In:         os.Stdin,
		Out:        os.Stdout,
		Err:        os.Stderr,
		Run:        ExecRunner{},
		Keychain:   platformKeychain(),
	}
}

// LoadConfig reads and validates the app's config; at least one profile is required.
func (a *App) LoadConfig() (*Config, error) { return LoadConfig(a.ConfigPath) }

// LoadCloudConfig loads the app's config without requiring profiles.
func (a *App) LoadCloudConfig() (*Config, error) { return LoadCloudConfig(a.ConfigPath) }

// PrintJSON writes v as indented JSON to the app's output.
func (a *App) PrintJSON(v any) error {
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (a *App) outf(format string, args ...any) { fmt.Fprintf(a.Out, format, args...) }
func (a *App) outln(args ...any)               { fmt.Fprintln(a.Out, args...) }
func (a *App) out(args ...any)                 { fmt.Fprint(a.Out, args...) }
