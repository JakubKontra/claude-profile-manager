package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// userConfig mirrors the shape of a real config copied from
// config.example.toml: header comments, commented-out example tables and a
// commented-out [cloud] section.
const userConfig = `# Claude Code Multi-Account Profile Configuration
# Copy to ~/.claude-profiles/config.toml and edit

source_dir = "~/.claude"
bin_dir = "~/.local/bin"

[profiles.personal]
description = "Personal Anthropic account"

[profiles.work]
description = "Company team subscription"
add_dirs = ["~/Work/cartop"]

# [profiles.work.attribution]
# commit = "Co-Authored-By: Claude <noreply@anthropic.com>"

# [profiles.work-vertex]
# description = "Company via Google Cloud Vertex AI"

# Cloud sync — sync settings across devices via git
# [cloud]
# remote = "git@github.com:your-user/claude-settings.git"
`

func TestTableHeader(t *testing.T) {
	cases := map[string]struct {
		name string
		ok   bool
	}{
		"[cloud]":                    {"cloud", true},
		"  [profiles.work]  ":        {"profiles.work", true},
		`[profiles."work-vertex"]`:   {"profiles.work-vertex", true},
		"[profiles.work] # trailing": {"profiles.work", true},
		"# [cloud]":                  {"", false},
		"#[cloud]":                   {"", false},
		"[[arr]]":                    {"", false},
		"remote = \"[cloud]\"":       {"", false},
		"[profiles.'a.b']":           {"profiles.a.b", true},
		"[]":                         {"", false},
		"[profiles.work] extra":      {"", false},
	}
	for line, want := range cases {
		name, ok := tableHeader(line)
		if ok != want.ok || name != want.name {
			t.Errorf("tableHeader(%q) = (%q,%v), want (%q,%v)", line, name, ok, want.name, want.ok)
		}
	}
}

func TestFindTableIncludesSubtables(t *testing.T) {
	lines := strings.Split(`[profiles.a]
x = 1
[profiles.a.env]
K = "v"
# comment
[profiles.b]
y = 2
`, "\n")
	tbl, ok := findTable(lines, "profiles.a")
	if !ok || tbl.Start != 0 || tbl.End != 5 {
		t.Errorf("unexpected range: %+v ok=%v", tbl, ok)
	}
	tbl, ok = findTable(lines, "profiles.b")
	if !ok || tbl.Start != 5 || tbl.End != len(lines) {
		t.Errorf("unexpected range for b: %+v ok=%v", tbl, ok)
	}
	if _, ok := findTable(lines, "profiles"); ok {
		t.Error("parent table must not match")
	}
}

func TestQuoteTOMLString(t *testing.T) {
	cases := map[string]string{
		"plain":      `"plain"`,
		`q"uote`:     `"q\"uote"`,
		`back\slash`: `"back\\slash"`,
		"tab\tnl\n":  `"tab\tnl\n"`,
		"ctl\x01":    `"ctl\u0001"`,
		"ünï":        `"ünï"`,
	}
	for in, want := range cases {
		if got := quoteTOMLString(in); got != want {
			t.Errorf("quoteTOMLString(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestAppendProfileTableRendersAllFieldsAndRoundTrips(t *testing.T) {
	p := &Profile{
		Description: `Client "X"`,
		Model:       "opus",
		AddDirs:     []string{"~/Work/x", "/tmp/y z"},
		Env:         map[string]string{"B": "2", "A": "it's"},
		Attribution: &Attribution{Commit: "Co-Authored-By: X"},
	}
	out, err := AppendProfileTable([]byte(userConfig), "client-x", p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.HasPrefix(text, userConfig) {
		t.Error("existing content (including comments) must be preserved verbatim")
	}
	if strings.Contains(text, `model = ""`) || strings.Contains(text, "[profiles.client-x.env]\n\n") {
		t.Errorf("empty fields must be omitted:\n%s", text)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, out, 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("round trip failed: %v\n%s", err, text)
	}
	got := cfg.Profiles["client-x"]
	if got == nil || got.Description != p.Description || got.Model != "opus" || len(got.AddDirs) != 2 ||
		got.Env["A"] != "it's" || got.Env["B"] != "2" || got.Attribution == nil || got.Attribution.Commit != "Co-Authored-By: X" {
		t.Errorf("round trip lost data: %+v", got)
	}
	if len(cfg.Profiles) != 3 {
		t.Errorf("expected 3 profiles, got %d", len(cfg.Profiles))
	}

	if _, err := AppendProfileTable(out, "work", &Profile{}); err == nil {
		t.Error("duplicate profile must be rejected")
	}
	if _, err := AppendProfileTable(out, "bad name", &Profile{}); err == nil {
		t.Error("invalid name must be rejected")
	}
}

func TestAppendProfileTableToEmptyConfig(t *testing.T) {
	out, err := AppendProfileTable(nil, "solo", &Profile{Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "[profiles.solo]\n") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestRemoveProfileTableWithSubtablesAndComments(t *testing.T) {
	cfgText := userConfig + `
[profiles.work.env]
FOO = "bar"
`
	out, removed, err := RemoveProfileTable([]byte(cfgText), "work")
	if err != nil || !removed {
		t.Fatalf("remove: %v removed=%v", err, removed)
	}
	text := string(out)
	if strings.Contains(text, "[profiles.work]") || strings.Contains(text, "FOO") || strings.Contains(text, "add_dirs") {
		t.Errorf("work table not fully removed:\n%s", text)
	}
	// Comments that belonged to the block go with it; unrelated ones stay.
	if strings.Contains(text, "# [profiles.work.attribution]") {
		t.Errorf("comments inside the removed block should be removed:\n%s", text)
	}
	if !strings.Contains(text, "[profiles.personal]") || !strings.Contains(text, "# Copy to ~/.claude-profiles") {
		t.Errorf("unrelated content lost:\n%s", text)
	}
	if strings.Contains(text, "\n\n\n") {
		t.Errorf("blank lines not collapsed:\n%q", text)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, out, 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, still := cfg.Profiles["work"]; still || len(cfg.Profiles) != 1 {
		t.Errorf("profiles after removal: %v", cfg.Profiles)
	}
}

func TestRemoveProfileTableMissing(t *testing.T) {
	out, removed, err := RemoveProfileTable([]byte(userConfig), "nope")
	if err != nil || removed || string(out) != userConfig {
		t.Errorf("missing profile should be a no-op: %v %v", err, removed)
	}
}

func TestRemoveProfileTableUnsupportedFormErrors(t *testing.T) {
	inline := "[profiles]\nwork = { description = \"x\" }\n"
	_, _, err := RemoveProfileTable([]byte(inline), "work")
	if err == nil || !strings.Contains(err.Error(), "manually") {
		t.Errorf("inline table should produce a manual-edit error, got %v", err)
	}
}

func TestSetTableKeyIgnoresCommentedHeader(t *testing.T) {
	// The A13 case: "# [cloud]" must not count as the [cloud] table.
	out, err := SetTableKey([]byte(userConfig), "cloud", "remote", quoteTOMLString("git@github.com:me/x.git"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, out, 0o644)
	cfg, err := LoadCloudConfig(path)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if cfg.Cloud == nil || cfg.Cloud.Remote != "git@github.com:me/x.git" {
		t.Errorf("remote not saved:\n%s", out)
	}
	if !strings.Contains(string(out), "# [cloud]") {
		t.Error("commented example must be preserved")
	}
}

func TestSetTableKeyReplacesAndInserts(t *testing.T) {
	base := "[cloud]\nremote = \"old\"\nexclude = []\n\n[profiles.a]\n"
	out, _ := SetTableKey([]byte(base), "cloud", "remote", `"new"`)
	if strings.Count(string(out), "remote =") != 1 || !strings.Contains(string(out), `remote = "new"`) {
		t.Errorf("replace failed:\n%s", out)
	}

	out, _ = SetTableKey([]byte(base), "cloud", "auto_push", "true")
	text := string(out)
	if !strings.Contains(text, "[cloud]\nauto_push = true\nremote") {
		t.Errorf("insert after header failed:\n%s", text)
	}
	if !strings.HasSuffix(text, "[profiles.a]\n") {
		t.Errorf("other tables disturbed:\n%s", text)
	}

	// A key in a subtable with the same name must not be touched.
	sub := "[cloud]\nx = 1\n[cloud.extra]\nremote = \"sub\"\n"
	out, _ = SetTableKey([]byte(sub), "cloud", "remote", `"top"`)
	if !strings.Contains(string(out), "[cloud]\nremote = \"top\"\nx = 1\n[cloud.extra]\nremote = \"sub\"") {
		t.Errorf("subtable key was touched:\n%s", out)
	}
}

func TestWriteConfigAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := writeConfigAtomic(path, []byte("[profiles.a]\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigAtomic(path, []byte("not = = toml")); err == nil {
		t.Error("invalid TOML must be rejected")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "[profiles.a]\n" {
		t.Errorf("file changed by rejected write: %q", data)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left: %v", leftovers)
	}
}
