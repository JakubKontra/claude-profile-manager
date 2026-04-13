package internal

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Attribution struct {
	Commit string `toml:"commit"`
	PR     string `toml:"pr"`
}

type Profile struct {
	Description string            `toml:"description"`
	Model       string            `toml:"model"`
	AddDirs     []string          `toml:"add_dirs"`
	Env         map[string]string `toml:"env"`
	Attribution *Attribution      `toml:"attribution"`
}

type CloudConfig struct {
	Remote   string   `toml:"remote"`
	AutoPush bool     `toml:"auto_push"`
	Exclude  []string `toml:"exclude"`
}

type Config struct {
	SourceDir string              `toml:"source_dir"`
	BinDir    string              `toml:"bin_dir"`
	Profiles  map[string]*Profile `toml:"profiles"`
	Cloud     *CloudConfig        `toml:"cloud"`
}

func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude-profiles", "config.toml")
}

func LoadConfig(path string) (*Config, error) {
	path = ExpandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config: %w", err)
	}

	if cfg.SourceDir == "" {
		cfg.SourceDir = "~/.claude"
	}
	if cfg.BinDir == "" {
		cfg.BinDir = "~/.local/bin"
	}
	cfg.SourceDir = ExpandPath(cfg.SourceDir)
	cfg.BinDir = ExpandPath(cfg.BinDir)

	if len(cfg.Profiles) == 0 {
		return nil, fmt.Errorf("no profiles defined in %s", path)
	}

	return &cfg, nil
}

func ExpandPath(p string) string {
	if len(p) == 0 {
		return p
	}
	if p[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}

// LoadCloudConfig loads config without requiring profiles to be defined.
func LoadCloudConfig(path string) (*Config, error) {
	path = ExpandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		// Return a default config if no config file exists
		return &Config{
			SourceDir: ExpandPath("~/.claude"),
			BinDir:    ExpandPath("~/.local/bin"),
		}, nil
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config: %w", err)
	}

	if cfg.SourceDir == "" {
		cfg.SourceDir = "~/.claude"
	}
	if cfg.BinDir == "" {
		cfg.BinDir = "~/.local/bin"
	}
	cfg.SourceDir = ExpandPath(cfg.SourceDir)
	cfg.BinDir = ExpandPath(cfg.BinDir)

	return &cfg, nil
}

func ProfilesBaseDir(configPath string) string {
	return filepath.Dir(ExpandPath(configPath))
}
