package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func loadLocalSecrets(cfg *Config, configPath string) error {
	path := filepath.Join(ProfilesBaseDir(configPath), "secrets.toml")
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 {
		return fmt.Errorf("%s must be a regular file with permissions 0600 (chmod 600)", path)
	}
	var secrets struct {
		Profiles map[string]struct {
			Env map[string]string `toml:"env"`
		} `toml:"profiles"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	md, err := toml.Decode(string(data), &secrets)
	if err != nil {
		return fmt.Errorf("cannot parse secrets.toml (check TOML syntax)")
	}
	if len(md.Undecoded()) != 0 {
		return fmt.Errorf("secrets.toml supports only profiles.<name>.env")
	}
	for name, secret := range secrets.Profiles {
		if err := ValidateProfileName(name); err != nil {
			return err
		}
		for key := range secret.Env {
			if !isValidEnvName(key) || key == "CLAUDE_CONFIG_DIR" || key == "CLAUDE_PROFILE" {
				return fmt.Errorf("invalid or reserved variable %q in secrets.toml", key)
			}
		}
		if p := cfg.Profiles[name]; p != nil {
			p.LocalEnv = secret.Env
		}
	}
	return nil
}

func privateSyncFile(path, configPath string) bool {
	base := filepath.Base(path)
	if base == "secrets.toml" || base == ".credentials.json" {
		return true
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	secret := filepath.Join(ProfilesBaseDir(configPath), "secrets.toml")
	secret, _ = filepath.Abs(secret)
	resolved, _ = filepath.Abs(resolved)
	return resolved == secret || filepath.Base(resolved) == ".credentials.json"
}

func checkSyncSecrets(files map[string]string, allowed bool) error {
	if allowed {
		return nil
	}
	findings := ScanForSecrets(files)
	if len(findings) == 0 {
		return nil
	}
	outln("Refusing to sync: secrets or unreadable structured files detected:")
	for _, f := range findings {
		outf("  %s: %s\n", f.File, f.Key)
	}
	return fmt.Errorf("move credentials to local secrets.toml, exclude the file, or explicitly pass --allow-secrets")
}

func scanSecretValues(file, prefix string, value any, findings *[]SecretFinding) {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range sortedAnyKeys(v) {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			if text, ok := v[key].(string); ok && (looksLikeSecret(key, text) || ((strings.EqualFold(key, "authorization") || strings.EqualFold(key, "token") || strings.EqualFold(key, "secret") || strings.EqualFold(key, "apikey")) && text != "")) {
				*findings = append(*findings, SecretFinding{File: file, Key: path})
			} else {
				scanSecretValues(file, path, v[key], findings)
			}
		}
	case []any:
		for i, item := range v {
			scanSecretValues(file, fmt.Sprintf("%s[%d]", prefix, i), item, findings)
		}
	case string:
		if secretValuePattern.MatchString(v) {
			*findings = append(*findings, SecretFinding{File: file, Key: prefix})
		}
	}
}
