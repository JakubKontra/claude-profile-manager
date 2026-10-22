package internal

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolationPromptDefaultsAndFlags(t *testing.T) {
	t.Parallel()
	app, _ := testApp(t)
	opts := AddOptions{}
	if err := app.ChooseIsolation(bufio.NewReader(strings.NewReader("\n\n")), &opts); err != nil {
		t.Fatal(err)
	}
	if *opts.ShareProjects || *opts.InheritMCP {
		t.Fatal("interactive defaults must isolate")
	}
	yes := true
	opts = AddOptions{ShareProjects: &yes}
	if err := app.ChooseIsolation(bufio.NewReader(strings.NewReader("invalid\nyes\n")), &opts); err != nil {
		t.Fatal(err)
	}
	if !*opts.ShareProjects || !*opts.InheritMCP {
		t.Fatal("explicit choices lost")
	}
}

func TestMCPIsolationPreservesOwnServersAndCompatibility(t *testing.T) {
	t.Parallel()
	global := map[string]any{"global": map[string]any{"command": "global"}}
	if len(MergeMCPServers(global, &Profile{})) != 1 {
		t.Fatal("legacy inheritance changed")
	}
	no := false
	p := &Profile{InheritMCP: &no, MCPServers: map[string]map[string]any{"own": {"command": "own"}}}
	servers := MergeMCPServers(global, p)
	if len(servers) != 1 || servers["own"] == nil {
		t.Fatal("wrong isolated MCP set")
	}
}

func TestAddIsolatedProfileAndClearInheritedServers(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.old]\n")
	app := env.App
	mustMkdir(t, filepath.Join(env.SourceDir, "projects"))
	mustWrite(t, filepath.Join(env.Root, ".claude.json"), `{"mcpServers":{"global":{"command":"global"}}}`)
	no := false
	if err := app.AddProfile("private", AddOptions{ShareProjects: &no, InheritMCP: &no}); err != nil {
		t.Fatal(err)
	}
	cfg := env.load(t)
	p := cfg.Profiles["private"]
	if inheritsMCP(p) || len(p.Isolate) != 1 {
		t.Fatal("choices not persisted")
	}
	path := filepath.Join(env.profilesBase(), "private", ".claude.json")
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "global") {
		t.Fatal("global MCP inherited")
	}
	if _, err := os.Readlink(filepath.Join(env.profilesBase(), "private", "projects")); err == nil {
		t.Fatal("history shared")
	}
	// Disabling inheritance clears stale servers even without global settings.
	os.Remove(filepath.Join(env.Root, ".claude.json"))
	mustWrite(t, path, `{"mcpServers":{"stale":{"command":"old"}}}`)
	if err := app.InstallProfiles(cfg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "stale") {
		t.Fatal("stale MCP retained")
	}
}
