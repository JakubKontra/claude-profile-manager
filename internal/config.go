package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"
)

type Attribution struct {
	Commit string `toml:"commit,omitempty"`
	PR     string `toml:"pr,omitempty"`
}

type Profile struct {
	Description string            `toml:"description,omitempty"`
	Model       string            `toml:"model,omitempty"`
	AddDirs     []string          `toml:"add_dirs,omitempty"`
	Env         map[string]string `toml:"env,omitempty"`
	Attribution *Attribution      `toml:"attribution,omitempty"`

	// Share replaces the list of source directories symlinked into this
	// profile; nil inherits the global default. Isolate removes entries
	// from that list (e.g. a private "projects/" session history).
	Share   []string `toml:"share,omitempty"`
	Isolate []string `toml:"isolate,omitempty"`

	// MCPExclude lists servers from ~/.claude.json this profile must not get.
	MCPExclude []string `toml:"mcp_exclude,omitempty"`
	// MCPServers are profile-specific servers ([profiles.x.mcp_servers.<name>]);
	// they win over a same-named global server.
	MCPServers map[string]map[string]any `toml:"mcp_servers,omitempty"`

	// Settings are deep-merged into the profile's settings.json
	// ([profiles.x.settings]); maps merge, everything else is replaced.
	Settings map[string]any `toml:"settings,omitempty"`
}

type CloudConfig struct {
	Remote   string   `toml:"remote"`
	AutoPush bool     `toml:"auto_push"`
	Exclude  []string `toml:"exclude"`
}

type Config struct {
	SourceDir string `toml:"source_dir"`
	BinDir    string `toml:"bin_dir"`
	// Share is the default list of source directories symlinked into every
	// profile; nil means defaultShareDirs.
	Share    []string            `toml:"share,omitempty"`
	Profiles map[string]*Profile `toml:"profiles"`
	Cloud    *CloudConfig        `toml:"cloud"`

	// Undecoded lists keys present in the file that no field consumed —
	// usually typos. Filled by the loader, reported by doctor.
	Undecoded []string `toml:"-"`
}

// profileNamePattern is what a profile name may look like: it becomes a
// directory name, a shell script name and a TOML table key.
var profileNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// envNamePattern matches a POSIX environment variable name.
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const maxProfileNameLen = 64

// ValidateProfileName rejects names that would break paths, wrappers or TOML.
func ValidateProfileName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("profile name must not be empty")
	case len(name) > maxProfileNameLen:
		return fmt.Errorf("profile name %q is too long (max %d characters)", name, maxProfileNameLen)
	case !profileNamePattern.MatchString(name):
		return fmt.Errorf("invalid profile name %q: use only letters, digits, '-' and '_'", name)
	case name == "auto":
		return fmt.Errorf("profile name %q is reserved", name)
	}
	return nil
}

func isValidEnvName(name string) bool {
	return envNamePattern.MatchString(name)
}

func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude-profiles", "config.toml")
}

// LoadConfig reads and validates the config; at least one profile is required.
func LoadConfig(path string) (*Config, error) {
	return loadConfig(path, true)
}

// LoadCloudConfig loads config without requiring profiles to be defined and
// falls back to defaults when the file does not exist yet.
func LoadCloudConfig(path string) (*Config, error) {
	return loadConfig(path, false)
}

func loadConfig(path string, requireProfiles bool) (*Config, error) {
	path = ExpandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if !requireProfiles && os.IsNotExist(err) {
			cfg := &Config{}
			applyConfigDefaults(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	var cfg Config
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot parse config: %w", err)
	}
	for _, key := range md.Undecoded() {
		cfg.Undecoded = append(cfg.Undecoded, key.String())
	}
	sort.Strings(cfg.Undecoded)

	applyConfigDefaults(&cfg)

	if requireProfiles && len(cfg.Profiles) == 0 {
		return nil, fmt.Errorf("no profiles defined in %s", path)
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}

	return &cfg, nil
}

func applyConfigDefaults(cfg *Config) {
	if cfg.SourceDir == "" {
		cfg.SourceDir = "~/.claude"
	}
	if cfg.BinDir == "" {
		cfg.BinDir = "~/.local/bin"
	}
	cfg.SourceDir = ExpandPath(cfg.SourceDir)
	cfg.BinDir = ExpandPath(cfg.BinDir)
}

// shareEntryPattern restricts share/isolate entries to plain directory names.
var shareEntryPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func validateShareEntries(context string, entries []string) error {
	for _, e := range entries {
		if e == "." || e == ".." || !shareEntryPattern.MatchString(e) {
			return fmt.Errorf("%s: invalid directory name %q (plain names only)", context, e)
		}
	}
	return nil
}

func validateConfig(cfg *Config) error {
	if err := validateShareEntries("share", cfg.Share); err != nil {
		return err
	}
	for _, name := range SortedProfileNames(cfg) {
		if err := ValidateProfileName(name); err != nil {
			return err
		}
		profile := cfg.Profiles[name]
		if profile == nil {
			cfg.Profiles[name] = &Profile{}
			continue
		}
		for _, k := range sortedEnvKeys(profile.Env) {
			if !isValidEnvName(k) {
				return fmt.Errorf("profile %q: invalid environment variable name %q", name, k)
			}
		}
		if err := validateShareEntries("profile "+name+" share", profile.Share); err != nil {
			return err
		}
		if err := validateShareEntries("profile "+name+" isolate", profile.Isolate); err != nil {
			return err
		}
	}
	return nil
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

func ProfilesBaseDir(configPath string) string {
	return filepath.Dir(ExpandPath(configPath))
}
