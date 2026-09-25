package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sourceClaudeJSON = `{
  "hasCompletedOnboarding": true,
  "lastOnboardingVersion": "2.1.0",
  "theme": "dark",
  "installMethod": "native",
  "oauthAccount": {"emailAddress": "me@example.com"},
  "userID": "u-123",
  "machineID": "m-1",
  "projects": {"/x": {}},
  "cachedStatsigGates": {"a": true},
  "numStartups": 42,
  "mcpServers": {
    "github": {"type": "http", "url": "https://api.githubcopilot.com/mcp/"},
    "notion": {"command": "npx", "args": ["notion-mcp"]}
  }
}`

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSeedClaudeJSONWhitelist(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	mustWrite(t, src, sourceClaudeJSON)
	profileDir := filepath.Join(dir, "profile")
	mustMkdir(t, profileDir)
	captureOutput(t)

	wrote, err := SeedClaudeJSON(profileDir, src)
	if err != nil || !wrote {
		t.Fatalf("seed: %v wrote=%v", err, wrote)
	}
	got := readJSON(t, filepath.Join(profileDir, ".claude.json"))
	for _, want := range []string{"hasCompletedOnboarding", "lastOnboardingVersion", "theme", "installMethod"} {
		if _, ok := got[want]; !ok {
			t.Errorf("seed missing %s", want)
		}
	}
	for _, forbidden := range []string{"oauthAccount", "userID", "machineID", "projects", "cachedStatsigGates", "numStartups", "mcpServers"} {
		if _, ok := got[forbidden]; ok {
			t.Errorf("seed must not copy %s", forbidden)
		}
	}
	if st, _ := os.Stat(filepath.Join(profileDir, ".claude.json")); st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", st.Mode().Perm())
	}

	// Existing file is never touched.
	wrote, err = SeedClaudeJSON(profileDir, src)
	if err != nil || wrote {
		t.Errorf("second seed must be a no-op: %v %v", err, wrote)
	}
}

func TestSeedClaudeJSONNoSourceOrNothingToSeed(t *testing.T) {
	dir := t.TempDir()
	if wrote, err := SeedClaudeJSON(dir, filepath.Join(dir, "missing")); err != nil || wrote {
		t.Errorf("missing source: %v %v", err, wrote)
	}
	src := filepath.Join(dir, "empty.json")
	mustWrite(t, src, `{"userID": "x"}`)
	if wrote, err := SeedClaudeJSON(dir, src); err != nil || wrote {
		t.Errorf("nothing whitelisted: %v %v", err, wrote)
	}
	if fileExists(filepath.Join(dir, ".claude.json")) {
		t.Error("no file should be written when nothing to seed")
	}
}

func TestMergeMCPServers(t *testing.T) {
	source := map[string]any{"github": map[string]any{"url": "g"}, "notion": map[string]any{"cmd": "n"}}
	p := &Profile{
		MCPExclude: []string{"notion"},
		MCPServers: map[string]map[string]any{
			"jira":   {"command": "jira-mcp"},
			"github": {"url": "override"},
		},
	}
	merged := MergeMCPServers(source, p)
	if _, ok := merged["notion"]; ok {
		t.Error("excluded server present")
	}
	if merged["jira"] == nil {
		t.Error("profile server missing")
	}
	if g, _ := merged["github"].(map[string]any); g["url"] != "override" {
		t.Errorf("profile server should win over global: %v", merged["github"])
	}
	if len(source) != 2 {
		t.Error("source must not be mutated")
	}
	if got := MergeMCPServers(source, nil); len(got) != 2 {
		t.Errorf("nil profile keeps all: %v", got)
	}
}

func TestSyncMCPServersMergeExcludeOverrideIdempotent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	mustWrite(t, src, sourceClaudeJSON)
	profileDir := filepath.Join(dir, "profile")
	mustWrite(t, filepath.Join(profileDir, ".claude.json"), `{"theme": "light", "mcpServers": {"old": {}}}`)
	buf := captureOutput(t)

	p := &Profile{
		MCPExclude: []string{"notion"},
		MCPServers: map[string]map[string]any{"jira": {"command": "jira-mcp", "args": []any{"--x"}}},
	}
	if err := SyncMCPServers(profileDir, p, src); err != nil {
		t.Fatal(err)
	}
	got := readJSON(t, filepath.Join(profileDir, ".claude.json"))
	servers := got["mcpServers"].(map[string]any)
	if _, ok := servers["notion"]; ok {
		t.Error("excluded server synced")
	}
	if _, ok := servers["old"]; ok {
		t.Error("stale profile server should be replaced")
	}
	if servers["github"] == nil || servers["jira"] == nil {
		t.Errorf("expected github + jira: %v", servers)
	}
	if got["theme"] != "light" {
		t.Error("other keys must be preserved")
	}
	if !strings.Contains(buf.String(), "synced mcpServers (2 servers)") {
		t.Errorf("unexpected output: %q", buf.String())
	}

	buf.Reset()
	if err := SyncMCPServers(profileDir, p, src); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "" {
		t.Errorf("second sync should be silent: %q", buf.String())
	}
}

func TestSyncMCPServersProfileOnlyServers(t *testing.T) {
	dir := t.TempDir()
	profileDir := filepath.Join(dir, "profile")
	mustWrite(t, filepath.Join(profileDir, ".claude.json"), `{}`)
	captureOutput(t)

	p := &Profile{MCPServers: map[string]map[string]any{"jira": {"command": "x"}}}
	if err := SyncMCPServers(profileDir, p, filepath.Join(dir, "missing")); err != nil {
		t.Fatal(err)
	}
	got := readJSON(t, filepath.Join(profileDir, ".claude.json"))
	if got["mcpServers"].(map[string]any)["jira"] == nil {
		t.Error("profile-only server not written without a global source")
	}
}

func TestInstallProfilesSeedsThenSyncsMCP(t *testing.T) {
	env := newTestEnv(t, `[profiles.work]
mcp_exclude = ["notion"]
[profiles.work.mcp_servers.jira]
command = "jira-mcp"
`)
	src := filepath.Join(env.Root, "claude.json")
	mustWrite(t, src, sourceClaudeJSON)
	// Point the install pipeline at our fake ~/.claude.json via HOME.
	t.Setenv("HOME", env.Root)
	os.Rename(src, filepath.Join(env.Root, ".claude.json"))
	captureOutput(t)

	if err := InstallProfiles(env.load(t), env.ConfigPath, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	got := readJSON(t, filepath.Join(env.profilesBase(), "work", ".claude.json"))
	if got["hasCompletedOnboarding"] != true {
		t.Error("onboarding flag not seeded")
	}
	servers, _ := got["mcpServers"].(map[string]any)
	if servers["github"] == nil || servers["jira"] == nil || servers["notion"] != nil {
		t.Errorf("mcp servers not merged on first install: %v", servers)
	}
	if _, ok := got["oauthAccount"]; ok {
		t.Error("account must not be seeded")
	}
}
