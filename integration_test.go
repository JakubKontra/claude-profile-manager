package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type cliMachine struct {
	root, home, source, config, bin, cpm string
	env                                  []string
}

func newCLIMachine(t *testing.T, binary, bash string) cliMachine {
	t.Helper()
	root := t.TempDir()
	m := cliMachine{root: root, home: filepath.Join(root, "home"), source: filepath.Join(root, "source"), config: filepath.Join(root, "profiles", "config.toml"), bin: filepath.Join(root, "bin"), cpm: binary}
	for _, dir := range []string{m.home, m.source, m.bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cliWrite(t, m.config, fmt.Sprintf("source_dir = %q\nbin_dir = %q\n[profiles.work]\nmodel = 'sonnet'\n", m.source, m.bin), 0o600)
	cliWrite(t, filepath.Join(m.source, "settings.json"), "{}", 0o600)
	cliWrite(t, filepath.Join(m.bin, "claude"), "#!"+bash+"\nfor a in \"$@\"; do printf 'ARG:%s\\n' \"$a\"; done\nprintf 'PROFILE:%s\\nTOKEN:%s\\n' \"${CLAUDE_PROFILE:-}\" \"${API_TOKEN:-}\"\n", 0o755)
	cliWrite(t, filepath.Join(m.bin, "security"), "#!"+bash+"\nexit 44\n", 0o755)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if key == "HOME" || key == "PATH" || key == "SHELL" || strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "CLAUDE_") || strings.HasPrefix(key, "ANTHROPIC_") || key == "API_TOKEN" {
			continue
		}
		m.env = append(m.env, item)
	}
	m.env = append(m.env, "HOME="+m.home, "PATH="+m.bin+string(os.PathListSeparator)+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"), "SHELL="+bash, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=CPM Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=CPM Test", "GIT_COMMITTER_EMAIL=test@example.invalid", "GIT_TERMINAL_PROMPT=0")
	return m
}

func cliWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func (m cliMachine) command(t *testing.T, success bool, binary string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env, cmd.Dir = m.env, m.root
	data, err := cmd.CombinedOutput()
	if (err == nil) != success {
		t.Fatalf("%s %v: success=%v, err=%v\n%s", binary, args, success, err, data)
	}
	return string(data)
}
func (m cliMachine) run(t *testing.T, success bool, args ...string) string {
	t.Helper()
	return m.command(t, success, m.cpm, append([]string{"--config", m.config}, args...)...)
}
func cliSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%v:%d", info.Mode(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += string(data)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += target
		}
		files[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCLIIntegration(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	binary := filepath.Join(t.TempDir(), "cpm")
	if data, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, data)
	}
	t.Run("lifecycle_secrets_preview", func(t *testing.T) {
		m := newCLIMachine(t, binary, bash)
		secretPath := filepath.Join(filepath.Dir(m.config), "secrets.toml")
		cliWrite(t, secretPath, "[profiles.work.env]\nAPI_TOKEN = 'first-private'\n", 0o600)
		before := cliSnapshot(t, m.root)
		preview := m.run(t, true, "install", "--dry-run")
		if strings.Contains(preview, "first-private") || !reflect.DeepEqual(before, cliSnapshot(t, m.root)) {
			t.Fatal("preview leaked or mutated data")
		}
		m.run(t, true, "install")
		wrapper := filepath.Join(m.bin, "claude-work")
		output := m.command(t, true, bash, wrapper, "-p", "hello world")
		if !strings.Contains(output, "ARG:hello world\n") || !strings.Contains(output, "TOKEN:first-private\n") || !strings.Contains(output, "ARG:sonnet\n") {
			t.Fatalf("wrapper: %s", output)
		}
		cliWrite(t, secretPath, "[profiles.work.env]\nAPI_TOKEN = 'rotated-private'\n", 0o600)
		output = m.command(t, true, bash, wrapper, "auth", "status")
		if strings.Contains(output, "ARG:--model") || !strings.Contains(output, "TOKEN:rotated-private") {
			t.Fatalf("runtime rotation/subcommand bypass: %s", output)
		}
		output = m.run(t, true, "exec", "work", "--", "claude", "auth", "status")
		if !strings.Contains(output, "TOKEN:rotated-private") {
			t.Fatal("exec missed overlay")
		}
		output = m.run(t, true, "run", "work", "--", "auth", "status")
		if strings.Contains(output, "ARG:--\n") || strings.Contains(output, "ARG:--model") {
			t.Fatalf("run separator: %s", output)
		}
		m.run(t, true, "add", "private", "--share-projects=false", "--inherit-mcp=false")
		m.run(t, true, "remove", "private")
		doctor := m.run(t, true, "doctor", "--json")
		if !strings.Contains(doctor, "orphan/private") {
			t.Fatal("doctor missed orphan")
		}
		m.run(t, false, "remove", "private", "--purge")
		m.run(t, true, "remove", "private", "--purge", "--yes")
		m.run(t, true, "remove", "work")
		m.run(t, true, "remove", "work", "--purge", "--yes")
		data, _ := os.ReadFile(secretPath)
		if strings.Contains(string(data), "rotated-private") {
			t.Fatal("purge retained local credentials")
		}
	})
	t.Run("foreign_wrapper", func(t *testing.T) {
		m := newCLIMachine(t, binary, bash)
		cliWrite(t, filepath.Join(m.bin, "claude-work"), "foreign", 0o755)
		before := cliSnapshot(t, m.root)
		m.run(t, false, "install")
		if !reflect.DeepEqual(before, cliSnapshot(t, m.root)) {
			t.Fatal("foreign wrapper failure partially installed")
		}
	})
	t.Run("cloud_two_machines", func(t *testing.T) {
		a, b := newCLIMachine(t, binary, bash), newCLIMachine(t, binary, bash)
		remote := filepath.Join(t.TempDir(), "remote.git")
		a.command(t, true, "git", "init", "--bare", remote)
		rel := filepath.Join("skills", "shared.md")
		cliWrite(t, filepath.Join(a.source, rel), "base", 0o600)
		a.run(t, true, "cloud", "init", "--remote", remote)
		a.run(t, true, "cloud", "push")
		b.run(t, true, "cloud", "init", "--remote", remote)
		cliWrite(t, filepath.Join(b.source, rel), "local edit", 0o600)
		cliWrite(t, filepath.Join(b.source, "skills", "local.md"), "local addition", 0o600)
		cliWrite(t, filepath.Join(a.source, rel), "remote edit", 0o600)
		a.run(t, true, "cloud", "push")
		before := cliSnapshot(t, b.source)
		b.run(t, false, "cloud", "pull")
		if !reflect.DeepEqual(before, cliSnapshot(t, b.source)) {
			t.Fatal("conflict changed live files")
		}
		cliWrite(t, filepath.Join(b.source, rel), "base", 0o600)
		b.run(t, true, "cloud", "pull")
		data, _ := os.ReadFile(filepath.Join(b.source, rel))
		if string(data) != "remote edit" {
			t.Fatal("remote change missing")
		}
		if _, err := os.Stat(filepath.Join(b.source, "skills", "local.md")); err != nil {
			t.Fatal("local addition deleted")
		}
		backups, _ := filepath.Glob(filepath.Join(filepath.Dir(b.config), ".cpm", "backups", "*", "skills", "shared.md"))
		if len(backups) != 1 {
			t.Fatal("missing conflict recovery backup")
		}
		cliWrite(t, filepath.Join(a.source, "settings.json"), `{"env":{"API_TOKEN":"blocked-private"}}`, 0o600)
		a.run(t, false, "cloud", "push")
		log := a.command(t, true, "git", "-C", filepath.Join(filepath.Dir(a.config), "cloud"), "log", "-p", "--all")
		if strings.Contains(log, "blocked-private") {
			t.Fatal("blocked secret committed")
		}
	})
	t.Run("initial_secret_refusal", func(t *testing.T) {
		m := newCLIMachine(t, binary, bash)
		cliWrite(t, filepath.Join(m.source, "settings.json"), `{"mcpServers":{"test":{"headers":{"Authorization":"Bearer private"}}}}`, 0o600)
		m.run(t, false, "cloud", "init")
		if _, err := os.Stat(filepath.Join(filepath.Dir(m.config), "cloud")); !os.IsNotExist(err) {
			t.Fatal("refused init created repo")
		}
	})
}
