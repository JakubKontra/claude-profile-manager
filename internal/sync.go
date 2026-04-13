package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SyncMCPServers(profileDir string) error {
	home, _ := os.UserHomeDir()
	sourcePath := filepath.Join(home, ".claude.json")

	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil // No source file, skip silently
	}

	var source map[string]any
	if err := json.Unmarshal(sourceData, &source); err != nil {
		return nil
	}

	servers, ok := source["mcpServers"]
	if !ok {
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

	// Check if already in sync
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

	if err := os.WriteFile(profilePath, append(out, '\n'), 0o644); err != nil {
		return err
	}

	count := 0
	if m, ok := servers.(map[string]any); ok {
		count = len(m)
	}
	fmt.Printf("  synced mcpServers (%d server%s)\n", count, pluralS(count))
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

	fmt.Println("  patched attribution in settings.json")
	return nil
}

type DivergedFile struct {
	Profile  string
	Filename string
	Details  string
}

func CheckDivergence(cfg *Config, profilesBase string) []DivergedFile {
	var diverged []DivergedFile

	for name, profile := range cfg.Profiles {
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
