package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// Fuzz targets for the parsers that read text cpm does not control: the
// user's config.toml, .git/config, settings.json and remote URLs. PRs run
// only the seeds; the weekly fuzz workflow explores further.

func FuzzRenameProfileTable(f *testing.F) {
	for _, seed := range []string{
		"[profiles.work]\nmodel = \"opus\"\n",
		"[profiles.\"work\"]\n[profiles.work.env]\nA = \"1\"\n",
		"# c\n[ profiles . 'work' ] # note\n[profiles.workshop]\n",
		"profiles.work.model = \"x\"\n",
		"[profiles.work]\n[profiles.job]\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		var before Config
		if _, err := toml.Decode(data, &before); err != nil {
			return
		}
		out, _, err := RenameProfileTable([]byte(data), "work", "job")
		if err != nil {
			return
		}
		var after Config
		if _, err := toml.Decode(string(out), &after); err != nil {
			t.Fatalf("output does not parse: %v\n%s", err, out)
		}
		if after.Profiles["work"] != nil {
			t.Fatalf("old profile still present:\n%s", out)
		}
		if before.Profiles["work"] != nil && after.Profiles["job"] == nil {
			t.Fatalf("profile lost:\n%s", out)
		}
		for name := range before.Profiles {
			if name != "work" && after.Profiles[name] == nil {
				t.Fatalf("unrelated profile %q lost:\n%s", name, out)
			}
		}
	})
}

func FuzzAppendRemoveProfile(f *testing.F) {
	for _, seed := range []string{"", "source_dir = \"~/.claude\"\n", "[profiles.a]\n# keep\n[cloud]\nremote = \"x\"\n", "[[match]]\nprofile = \"a\"\n"} {
		f.Add(seed, "desc")
	}
	f.Fuzz(func(t *testing.T, data, description string) {
		var before Config
		if _, err := toml.Decode(data, &before); err != nil || before.Profiles["fuzz"] != nil {
			return
		}
		added, err := AppendProfileTable([]byte(data), "fuzz", &Profile{Description: description})
		if !utf8.ValidString(description) {
			if err == nil {
				t.Fatalf("invalid UTF-8 description accepted:\n%q", added)
			}
			return
		}
		if err != nil {
			t.Fatalf("append failed on a valid config: %v", err)
		}
		var mid Config
		if _, err := toml.Decode(string(added), &mid); err != nil || mid.Profiles["fuzz"] == nil || mid.Profiles["fuzz"].Description != description {
			t.Fatalf("appended config: %v\n%s", err, added)
		}
		removed, ok, err := RemoveProfileTable(added, "fuzz")
		if err != nil || !ok {
			t.Fatalf("remove: %v %v\n%s", ok, err, added)
		}
		var after Config
		if _, err := toml.Decode(string(removed), &after); err != nil || after.Profiles["fuzz"] != nil {
			t.Fatalf("after remove: %v\n%s", err, removed)
		}
		if len(after.Profiles) != len(before.Profiles) {
			t.Fatalf("remove changed other profiles:\n%s", removed)
		}
	})
}

func FuzzSetRootKey(f *testing.F) {
	for _, seed := range []string{"", "a = 1\n", "[t]\nx = 1\n", "# c\n\n[[match]]\nprofile = \"w\"\n", "a = 1"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		var before map[string]any
		md, err := toml.Decode(data, &before)
		if err != nil || md.IsDefined("indicator") {
			return
		}
		out := SetRootKey([]byte(data), "indicator", "true")
		var after struct {
			Indicator bool `toml:"indicator"`
		}
		if _, err := toml.Decode(string(out), &after); err != nil || !after.Indicator {
			t.Fatalf("SetRootKey(%q) = %q: %v", data, out, err)
		}
	})
}

func FuzzGlobMatch(f *testing.F) {
	for _, seed := range [][2]string{{"~/Work/**", "/home/me/Work/a"}, {"github.com/*/x", "github.com/me/x"}, {"a?c", "abc"}, {"[", "["}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, pattern, name string) {
		globMatch(pattern, name) // must not panic
		if !utf8.ValidString(name) {
			return // paths and remotes are UTF-8; regexp rejects anything else
		}
		if !strings.ContainsAny(name, "*?") && !globMatch(name, name) {
			t.Fatalf("literal %q does not match itself", name)
		}
		if !globMatch("**", name) {
			t.Fatalf("** does not match %q", name)
		}
		if !strings.Contains(name, "/") && !globMatch("*", name) {
			t.Fatalf("* does not match %q", name)
		}
	})
}

func FuzzNormalizeRemote(f *testing.F) {
	for _, seed := range [][2]string{{"github.com", "me/repo"}, {"GitLab.Example.com", "group/sub/proj"}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, host, path string) {
		NormalizeRemote(host + ":" + path) // arbitrary input must not panic
		if host == "" || path == "" || strings.ContainsAny(host, ":/@") || strings.Contains(path, ":") ||
			strings.ContainsFunc(host+path, unicode.IsSpace) ||
			strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.HasSuffix(path, ".git") || strings.Contains(path, "//") {
			return
		}
		// The three ways to write the same remote agree.
		scp := NormalizeRemote("git@" + host + ":" + path + ".git")
		https := NormalizeRemote("https://" + host + "/" + path)
		ssh := NormalizeRemote("ssh://git@" + host + ":22/" + path + ".git")
		if scp != https || https != ssh {
			t.Fatalf("forms differ: scp %q, https %q, ssh %q", scp, https, ssh)
		}
		if want := strings.ToLower(host) + "/" + path; https != want {
			t.Fatalf("NormalizeRemote = %q, want %q", https, want)
		}
	})
}

func FuzzGitRemoteURLs(f *testing.F) {
	f.Add("[remote \"origin\"]\n\turl = git@github.com:me/x.git\n")
	f.Add("[core]\nbare = false\n[remote \"a\"]\nurl=\"https://x/y\"\n")
	f.Fuzz(func(t *testing.T, config string) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		gitRemoteURLs(dir) // must not panic
	})
}

func FuzzApplyIndicator(f *testing.F) {
	for _, seed := range []string{`{}`, `{"model":"opus"}`, `{"statusLine":{"type":"command","command":"mine"}}`, ``, `[]`, `{oops`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		on := applyIndicator([]byte(data), "work", true)
		if again := applyIndicator(on, "work", true); string(again) != string(on) {
			t.Fatalf("on is not idempotent: %q -> %q -> %q", data, on, again)
		}
		var settings map[string]any
		if json.Unmarshal([]byte(data), &settings) != nil || settings == nil {
			return
		}
		if _, has := settings["statusLine"]; has {
			if string(on) != data {
				t.Fatalf("existing statusLine changed: %q -> %q", data, on)
			}
			return
		}
		if off := applyIndicator(on, "work", false); !settingsEqual(off, []byte(data), "work") {
			t.Fatalf("off after on changed settings: %q -> %q", data, off)
		}
	})
}
