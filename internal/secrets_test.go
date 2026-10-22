package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalSecretsRuntimeOnly(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.work.env]\nAPI_TOKEN = 'public-default'\n")
	secretPath := filepath.Join(ProfilesBaseDir(env.ConfigPath), "secrets.toml")
	if err := os.WriteFile(secretPath, []byte("[profiles.work.env]\nAPI_TOKEN = 'private-value'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := env.load(t)
	p := cfg.Profiles["work"]
	if p.Env["API_TOKEN"] != "public-default" || p.LocalEnv["API_TOKEN"] != "private-value" {
		t.Fatal("overlay merged into portable config")
	}
	spec := BuildLaunchSpec("work", "/work", p)
	got := strings.Join(spec.Environ([]string{"API_TOKEN=inherited"}), "\n")
	if !strings.Contains(got, "API_TOKEN=private-value") || strings.Contains(got, "inherited") || strings.Contains(got, "public-default") {
		t.Fatalf("overlay failed: %q", got)
	}
	for _, output := range []string{GenerateWrapper("work", "/work", p), GenerateDirenvSnippet("work", "/work", p)} {
		if strings.Contains(output, "private-value") || !strings.Contains(output, "cpm --config") {
			t.Fatal("generated file embeds secret")
		}
	}
	cfg.Cloud = &CloudConfig{Include: []string{"secrets.toml"}}
	mustWrite(t, filepath.Join(env.SourceDir, "secrets.toml"), "private")
	files, err := GatherSyncFiles(cfg, env.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["secrets.toml"]; ok {
		t.Fatal("local secrets synced")
	}
	os.Chmod(secretPath, 0o644)
	if _, err := LoadConfig(env.ConfigPath); err == nil {
		t.Fatal("insecure secret permissions accepted")
	}
}

func TestInitialCloudCommitBlocksConfigAndMCPSecrets(t *testing.T) {
	requireGit(t)
	env := newCloudEnv(t, "[profiles.work.env]\nAPI_TOKEN = 'private'\n")
	env.activate(t)
	if err := env.App.CloudInit(""); err == nil {
		t.Fatal("initial secret commit accepted")
	}
	if fileExistsAt(CloudRepoDir(env.ConfigPath)) {
		t.Fatal("refused init wrote repo")
	}
	path := filepath.Join(t.TempDir(), "mcp.json")
	mustWrite(t, path, `{"mcpServers":{"service":{"headers":{"Authorization":"Bearer private"}}}}`)
	findings := ScanForSecrets(map[string]string{"mcp.json": path})
	if len(findings) != 1 {
		t.Fatalf("MCP secret missed: %v", findings)
	}
	if err := env.App.CloudInitWithSecrets("", true); err != nil {
		t.Fatal(err)
	}
}
