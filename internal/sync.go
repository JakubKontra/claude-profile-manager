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

func PatchAttribution(profileDir string, attr *Attribution) error {
	if attr == nil {
		return nil
	}

	settingsPath := filepath.Join(profileDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}

	attrMap := map[string]string{}
	if attr.Commit != "" {
		attrMap["commit"] = attr.Commit
	}
	if attr.PR != "" {
		attrMap["pr"] = attr.PR
	}

	// Check if already matches
	existingJSON, _ := json.Marshal(settings["attribution"])
	newJSON, _ := json.Marshal(attrMap)
	if string(existingJSON) == string(newJSON) {
		return nil
	}

	settings["attribution"] = attrMap

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return err
	}

	outln("  patched attribution in settings.json")
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

			// For settings.json, apply attribution patch before comparing
			expectedData := srcData
			if filename == "settings.json" && profile.Attribution != nil {
				expectedData = applyAttributionToSource(srcData, profile.Attribution)
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

func applyAttributionToSource(data []byte, attr *Attribution) []byte {
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return data
	}

	attrMap := map[string]string{}
	if attr.Commit != "" {
		attrMap["commit"] = attr.Commit
	}
	if attr.PR != "" {
		attrMap["pr"] = attr.PR
	}
	settings["attribution"] = attrMap

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return data
	}
	return append(out, '\n')
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
