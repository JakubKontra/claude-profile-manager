package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultClaudeJSONPath is where Claude Code keeps the global ~/.claude.json.
func DefaultClaudeJSONPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

// claudeJSONSeedKeys are the keys copied from ~/.claude.json into a new
// profile so it starts without the onboarding wizard. Account, identity,
// project state and caches are deliberately left out.
var claudeJSONSeedKeys = []string{
	"hasCompletedOnboarding",
	"lastOnboardingVersion",
	"theme",
	"preferredNotifChannel",
	"editorMode",
	"autoUpdates",
	"autoUpdatesProtectedForNative",
	"installMethod",
}

// SeedClaudeJSON creates <profileDir>/.claude.json from the whitelisted keys
// of sourcePath when the profile has none yet. It reports whether it wrote.
func SeedClaudeJSON(profileDir, sourcePath string) (bool, error) {
	profilePath := filepath.Join(profileDir, ".claude.json")
	if _, err := os.Stat(profilePath); err == nil {
		return false, nil
	}

	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		return false, nil // nothing to seed from
	}
	var source map[string]any
	if err := json.Unmarshal(sourceData, &source); err != nil {
		return false, nil
	}

	seed := make(map[string]any)
	for _, key := range claudeJSONSeedKeys {
		if v, ok := source[key]; ok {
			seed[key] = v
		}
	}
	if len(seed) == 0 {
		return false, nil
	}

	out, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(profilePath, append(out, '\n'), 0o600); err != nil {
		return false, err
	}
	outln("  seeded .claude.json (onboarding skipped)")
	return true, nil
}

// MergeMCPServers computes a profile's servers: the global ones minus
// mcp_exclude, overlaid with the profile's own mcp_servers.
func MergeMCPServers(source map[string]any, p *Profile) map[string]any {
	merged := make(map[string]any, len(source))
	for name, server := range source {
		merged[name] = server
	}
	if p != nil {
		for _, name := range p.MCPExclude {
			delete(merged, name)
		}
		for name, server := range p.MCPServers {
			merged[name] = server
		}
	}
	return merged
}

// SyncMCPServers writes the merged MCP server set into the profile's
// .claude.json. It is a no-op until the profile file exists.
func SyncMCPServers(profileDir string, p *Profile, sourcePath string) error {
	var source map[string]any
	if sourceData, err := os.ReadFile(sourcePath); err == nil {
		_ = json.Unmarshal(sourceData, &source)
	}
	var globalServers map[string]any
	if s, ok := source["mcpServers"].(map[string]any); ok {
		globalServers = s
	}
	if globalServers == nil && (p == nil || len(p.MCPServers) == 0) {
		return nil
	}

	profilePath := filepath.Join(profileDir, ".claude.json")
	profileData, err := os.ReadFile(profilePath)
	if err != nil {
		return nil // Profile .claude.json doesn't exist yet (first run)
	}
	var profile map[string]any
	if err := json.Unmarshal(profileData, &profile); err != nil {
		return nil
	}

	servers := MergeMCPServers(globalServers, p)

	existingJSON, _ := json.Marshal(profile["mcpServers"])
	newJSON, _ := json.Marshal(servers)
	if string(existingJSON) == string(newJSON) {
		return nil
	}

	profile["mcpServers"] = servers

	out, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(profilePath, append(out, '\n'), 0o600); err != nil {
		return err
	}

	outf("  synced mcpServers (%d server%s)\n", len(servers), pluralS(len(servers)))
	return nil
}

// attributionMap renders the attribution block for settings.json.
func attributionMap(attr *Attribution) map[string]any {
	m := map[string]any{}
	if attr.Commit != "" {
		m["commit"] = attr.Commit
	}
	if attr.PR != "" {
		m["pr"] = attr.PR
	}
	return m
}

// EffectiveSettingsOverrides is what a profile layers onto settings.json:
// its [profiles.x.settings] plus attribution, which wins on conflict.
func EffectiveSettingsOverrides(p *Profile) map[string]any {
	if p == nil {
		return nil
	}
	overrides := deepCopyMap(p.Settings)
	if p.Attribution != nil {
		if overrides == nil {
			overrides = map[string]any{}
		}
		overrides["attribution"] = attributionMap(p.Attribution)
	}
	return overrides
}

// deepMerge merges src into dst: nested maps merge recursively, every other
// value (including arrays) replaces the destination. dst may be nil.
func deepMerge(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range src {
		srcMap, srcIsMap := v.(map[string]any)
		dstMap, dstIsMap := dst[k].(map[string]any)
		if srcIsMap && dstIsMap {
			dst[k] = deepMerge(dstMap, srcMap)
			continue
		}
		dst[k] = v
	}
	return dst
}

func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			out[k] = deepCopyMap(sub)
			continue
		}
		out[k] = v
	}
	return out
}

// PatchAttribution keeps the old entry point; attribution is one override.
func PatchAttribution(profileDir string, attr *Attribution) error {
	if attr == nil {
		return nil
	}
	return PatchSettings(profileDir, map[string]any{"attribution": attributionMap(attr)})
}

// PatchSettings deep-merges overrides into the profile's settings.json.
func PatchSettings(profileDir string, overrides map[string]any) error {
	if len(overrides) == 0 {
		return nil
	}

	settingsPath := filepath.Join(profileDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	merged, changed := applySettingsToSource(data, overrides)
	if !changed {
		return nil
	}
	if err := os.WriteFile(settingsPath, merged, 0o644); err != nil {
		return err
	}

	outln("  patched settings.json")
	return nil
}

type DivergedFile struct {
	Profile  string
	Filename string
	Details  string
}

func CheckDivergence(cfg *Config, profilesBase string) []DivergedFile {
	var diverged []DivergedFile

	for _, name := range SortedProfileNames(cfg) {
		profile := cfg.Profiles[name]
		profileDir := filepath.Join(profilesBase, name)

		for _, filename := range copyFiles {
			src := filepath.Join(cfg.SourceDir, filename)
			dst := filepath.Join(profileDir, filename)

			srcData, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			dstData, err := os.ReadFile(dst)
			if err != nil {
				continue
			}

			// For settings.json, apply the profile's overrides before comparing
			expectedData := srcData
			if filename == "settings.json" {
				if overrides := EffectiveSettingsOverrides(profile); len(overrides) > 0 {
					expectedData, _ = applySettingsToSource(srcData, overrides)
				}
			}

			if string(expectedData) != string(dstData) {
				// Check if profile has additions not in source
				srcLines := strings.Split(string(expectedData), "\n")
				dstLines := strings.Split(string(dstData), "\n")

				hasAdditions := false
				for _, dl := range dstLines {
					found := false
					for _, sl := range srcLines {
						if dl == sl {
							found = true
							break
						}
					}
					if !found && strings.TrimSpace(dl) != "" {
						hasAdditions = true
						break
					}
				}

				if hasAdditions {
					diverged = append(diverged, DivergedFile{
						Profile:  name,
						Filename: filename,
						Details:  fmt.Sprintf("Profile '%s' — %s has local changes", name, filename),
					})
				}
			}
		}
	}

	return diverged
}

// applySettingsToSource returns data with overrides merged in and whether
// that changed anything. Unparseable input is returned unchanged.
func applySettingsToSource(data []byte, overrides map[string]any) ([]byte, bool) {
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return data, false
	}
	before, _ := json.Marshal(settings)

	merged := deepMerge(settings, deepCopyMap(overrides))
	after, _ := json.Marshal(merged)
	if string(before) == string(after) {
		return data, false
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return data, false
	}
	return append(out, '\n'), true
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
