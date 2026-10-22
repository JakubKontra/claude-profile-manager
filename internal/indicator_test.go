package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIndicatorEnabled(t *testing.T) {
	t.Parallel()
	on, off := true, false
	cases := []struct {
		global  bool
		profile *bool
		want    bool
	}{
		{false, nil, false},
		{true, nil, true},
		{true, &off, false},
		{false, &on, true},
	}
	for _, c := range cases {
		if got := IndicatorEnabled(&Config{Indicator: c.global}, &Profile{Indicator: c.profile}); got != c.want {
			t.Errorf("global=%v profile=%v: got %v, want %v", c.global, c.profile, got, c.want)
		}
	}
}

func TestApplyIndicator(t *testing.T) {
	t.Parallel()
	ours := map[string]any{"type": "command", "command": "echo 'cpm: work'"}
	cases := []struct {
		name string
		in   string
		on   bool
		want map[string]any // nil: input returned unchanged
	}{
		{"adds when missing", `{"model":"opus"}`, true, map[string]any{"model": "opus", "statusLine": ours}},
		{"adds to empty file", ``, true, map[string]any{"statusLine": ours}},
		{"keeps foreign statusLine", `{"statusLine":{"type":"command","command":"mine"}}`, true, nil},
		{"keeps foreign statusLine when off", `{"statusLine":{"type":"command","command":"mine"}}`, false, nil},
		{"removes own when off", `{"model":"opus","statusLine":{"type":"command","command":"echo 'cpm: work'"}}`, false, map[string]any{"model": "opus"}},
		{"keeps other profile's", `{"statusLine":{"type":"command","command":"echo 'cpm: home'"}}`, false, nil},
		{"off without statusLine", `{"model":"opus"}`, false, nil},
		{"unparseable", `{oops`, true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := applyIndicator([]byte(c.in), "work", c.on)
			if c.want == nil {
				if string(got) != c.in {
					t.Errorf("want input unchanged, got %s", got)
				}
				return
			}
			if !reflect.DeepEqual(decodeJSON(t, got), c.want) {
				t.Errorf("got %s", got)
			}
		})
	}
}

func TestInstallIndicator(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "indicator = true\n[profiles.work]\n[profiles.home]\nindicator = false\n")
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"model": "opus"}`)
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	work := readJSON(t, filepath.Join(env.profilesBase(), "work", "settings.json"))
	if !isIndicator(work["statusLine"], "work") || work["model"] != "opus" {
		t.Errorf("work settings: %v", work)
	}
	if home := readJSON(t, filepath.Join(env.profilesBase(), "home", "settings.json")); home["statusLine"] != nil {
		t.Errorf("home opted out but got %v", home["statusLine"])
	}
	// Injected by cpm, so not a local edit; --sync goes through.
	if d := CheckDivergence(env.load(t), env.profilesBase()); len(d) != 0 {
		t.Errorf("indicator reported as divergence: %v", d)
	}
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{Sync: true}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Turning it off removes it and still is no divergence.
	writeTestConfig(t, env, "[profiles.work]\n[profiles.home]\n")
	if d := CheckDivergence(env.load(t), env.profilesBase()); len(d) != 0 {
		t.Errorf("disabled indicator reported as divergence: %v", d)
	}
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if work := readJSON(t, filepath.Join(env.profilesBase(), "work", "settings.json")); work["statusLine"] != nil {
		t.Errorf("indicator not removed: %v", work)
	}
}

func TestInstallIndicatorNeverOverwrites(t *testing.T) {
	t.Parallel()
	mine := map[string]any{"type": "command", "command": "mine"}
	t.Run("source", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "indicator = true\n[profiles.work]\n")
		mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"statusLine":{"type":"command","command":"mine"}}`)
		if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := readJSON(t, filepath.Join(env.profilesBase(), "work", "settings.json")); !reflect.DeepEqual(got["statusLine"], mine) {
			t.Errorf("statusLine overwritten: %v", got["statusLine"])
		}
	})
	t.Run("profile settings", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "indicator = true\n[profiles.work.settings.statusLine]\ntype = \"command\"\ncommand = \"mine\"\n")
		mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
		if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := readJSON(t, filepath.Join(env.profilesBase(), "work", "settings.json")); !reflect.DeepEqual(got["statusLine"], mine) {
			t.Errorf("statusLine overwritten: %v", got["statusLine"])
		}
	})
	t.Run("local edit", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "[profiles.work]\n")
		mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{}`)
		if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(env.profilesBase(), "work", "settings.json")
		mustWrite(t, path, `{"statusLine":{"type":"command","command":"mine"}}`)
		writeTestConfig(t, env, "indicator = true\n[profiles.work]\n")
		if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := readJSON(t, path); !reflect.DeepEqual(got["statusLine"], mine) {
			t.Errorf("statusLine overwritten: %v", got["statusLine"])
		}
	})
}

func TestInstallIndicatorCreatesSettings(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "[profiles.work]\nindicator = true\n")
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	got := readJSON(t, filepath.Join(env.profilesBase(), "work", "settings.json"))
	if !isIndicator(got["statusLine"], "work") {
		t.Errorf("settings.json: %v", got)
	}
	if fileExists(baselinePath(filepath.Join(env.profilesBase(), "work"), "settings.json")) {
		t.Error("no source settings.json, so no baseline expected")
	}
}

func TestInstallIndicatorDryRun(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "indicator = true\n[profiles.work]\n")
	mustWrite(t, filepath.Join(env.SourceDir, "settings.json"), `{"model": "opus"}`)
	if err := env.App.InstallProfiles(env.load(t), InstallOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.Out.String(), `+  "statusLine": {`) {
		t.Errorf("dry run does not show the statusLine:\n%s", env.Out.String())
	}
	if _, err := os.Stat(filepath.Join(env.profilesBase(), "work", "settings.json")); !os.IsNotExist(err) {
		t.Error("dry run wrote settings.json")
	}
}

func decodeJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	return m
}
