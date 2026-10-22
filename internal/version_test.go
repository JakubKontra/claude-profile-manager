package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease serves a GitHub-like release with one binary and checksums.
func fakeRelease(t *testing.T, tag string, binary []byte, checksum string) *httptest.Server {
	t.Helper()
	assetName := fmt.Sprintf("cpm_%s_%s", runtime.GOOS, runtime.GOARCH)
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/jakubkontra/claude-profile-manager/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":%q,"browser_download_url":%q},{"name":"checksums.txt","browser_download_url":%q}]}`,
			tag, assetName, srv.URL+"/dl/"+assetName, srv.URL+"/dl/checksums.txt")
	})
	mux.HandleFunc("/dl/"+assetName, func(w http.ResponseWriter, r *http.Request) { w.Write(binary) })
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n%s  cpm_other_arch\n", checksum, assetName, strings.Repeat("0", 64))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prevBase := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = prevBase })
	return srv
}

func stubExecutable(t *testing.T, path string) {
	t.Helper()
	prev := executablePath
	executablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { executablePath = prev })
}

func TestUpgradeVerifiesChecksumAndReplacesRunningBinary(t *testing.T) {
	binary := []byte("#!/bin/sh\necho new\n")
	sum := sha256.Sum256(binary)
	fakeRelease(t, "v9.9.9", binary, hex.EncodeToString(sum[:]))

	exe := filepath.Join(t.TempDir(), "cpm")
	mustWrite(t, exe, "old")
	stubExecutable(t, exe)
	app, _ := testApp(t)

	if err := app.Upgrade(false); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	data, _ := os.ReadFile(exe)
	if string(data) != string(binary) {
		t.Errorf("binary not replaced: %q", data)
	}
	if st, _ := os.Stat(exe); st.Mode()&0o111 == 0 {
		t.Error("replaced binary is not executable")
	}
}

func TestUpgradeRejectsBadChecksum(t *testing.T) {
	binary := []byte("evil")
	fakeRelease(t, "v9.9.9", binary, strings.Repeat("ab", 32))

	exe := filepath.Join(t.TempDir(), "cpm")
	mustWrite(t, exe, "old")
	stubExecutable(t, exe)
	app, _ := testApp(t)

	err := app.Upgrade(false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum error, got %v", err)
	}
	data, _ := os.ReadFile(exe)
	if string(data) != "old" {
		t.Error("binary must be untouched after a failed verification")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "cpm-update-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestUpgradeAlreadyLatest(t *testing.T) {
	fakeRelease(t, "v"+Version, []byte("x"), strings.Repeat("0", 64))
	exe := filepath.Join(t.TempDir(), "cpm")
	mustWrite(t, exe, "old")
	stubExecutable(t, exe)
	app, buf := testApp(t)

	if err := app.Upgrade(false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Already at latest") {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestUpgradeRefusesHomebrewInstall(t *testing.T) {
	stubExecutable(t, "/opt/homebrew/Cellar/cpm/0.2.1/bin/cpm")
	app, _ := testApp(t)
	err := app.Upgrade(false)
	if err == nil || !strings.Contains(err.Error(), "brew upgrade cpm") {
		t.Fatalf("expected Homebrew hint, got %v", err)
	}
}

func TestIsHomebrewPath(t *testing.T) {
	t.Parallel()
	yes := []string{"/opt/homebrew/bin/cpm", "/usr/local/Cellar/cpm/1/bin/cpm", "/home/linuxbrew/.linuxbrew/bin/cpm"}
	no := []string{"/Users/me/.local/bin/cpm", "/usr/local/bin/cpm"}
	for _, p := range yes {
		if !isHomebrewPath(p) {
			t.Errorf("%s should be Homebrew", p)
		}
	}
	for _, p := range no {
		if isHomebrewPath(p) {
			t.Errorf("%s should not be Homebrew", p)
		}
	}
}

func TestCheckLatestVersion(t *testing.T) {
	fakeRelease(t, "v1.2.3", nil, "")
	got, err := CheckLatestVersion(false)
	if err != nil || got != "v1.2.3" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()
	ordered := []string{"dev", "v0.5.0", "v0.6.0-rc.1", "0.6.0-rc.2", "v0.6.0-rc.10", "v0.6.0", "v0.10.0", "v1.0.0-alpha", "v1.0.0-alpha.1", "v1.0.0-beta", "v1.0.0-rc.1", "v1.0.0"}
	for i := range ordered {
		for j := range ordered {
			want := cmpInt(i, j)
			if got := compareVersions(ordered[i], ordered[j]); got != want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	if compareVersions("v1.2.3+build", "1.2.3") != 0 {
		t.Error("build metadata must be ignored")
	}
}

// fakeReleaseList serves /releases/latest as stable and /releases as tags,
// newest first like GitHub. Tags with a "-" are prereleases.
func fakeReleaseList(t *testing.T, stable string, tags ...string) {
	t.Helper()
	mux := http.NewServeMux()
	release := func(tag string) string {
		return fmt.Sprintf(`{"tag_name":%q,"prerelease":%t,"assets":[]}`, tag, strings.Contains(tag, "-"))
	}
	mux.HandleFunc("/repos/jakubkontra/claude-profile-manager/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, release(stable))
	})
	mux.HandleFunc("/repos/jakubkontra/claude-profile-manager/releases", func(w http.ResponseWriter, r *http.Request) {
		var items []string
		for _, tag := range tags {
			items = append(items, release(tag))
		}
		items = append(items, `{"tag_name":"v9.0.0","draft":true}`)
		fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	prevBase := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = prevBase })
}

func stubVersion(t *testing.T, v string) {
	t.Helper()
	prev := Version
	Version = v
	t.Cleanup(func() { Version = prev })
}

func TestUpgradeChannels(t *testing.T) {
	cases := []struct {
		name, current string
		flag          bool
		stable        string
		tags          []string
		wantNew       string // "" means no update offered
	}{
		{"stable user ignores rc", "0.8.0", false, "v0.8.0", []string{"v1.0.0-rc.1", "v0.8.0"}, ""},
		{"stable user opts in", "0.8.0", true, "v0.8.0", []string{"v1.0.0-rc.1", "v0.8.0"}, "v1.0.0-rc.1"},
		{"rc user is not downgraded", "1.0.0-rc.1", false, "v0.8.0", []string{"v1.0.0-rc.1", "v0.8.0"}, ""},
		{"rc user gets next rc", "1.0.0-rc.1", false, "v0.8.0", []string{"v1.0.0-rc.2", "v1.0.0-rc.1", "v0.8.0"}, "v1.0.0-rc.2"},
		{"rc user gets final", "1.0.0-rc.2", false, "v1.0.0", []string{"v1.0.0", "v1.0.0-rc.2"}, "v1.0.0"},
		{"newer local build", "0.9.0", false, "v0.8.0", nil, ""},
		{"dev build", "dev", false, "v0.8.0", nil, "v0.8.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stubVersion(t, c.current)
			fakeReleaseList(t, c.stable, c.tags...)
			app, buf := testApp(t)
			app.PrintVersion(c.flag)
			got := ""
			if _, after, ok := strings.Cut(buf.String(), "New version available: "); ok {
				got, _, _ = strings.Cut(after, " ")
			}
			if got != c.wantNew {
				t.Errorf("offered %q, want %q\n%s", got, c.wantNew, buf.String())
			}

			// Upgrade agrees: it either stops as up to date or tries the
			// offered release (which has no assets here).
			stubExecutable(t, filepath.Join(t.TempDir(), "cpm"))
			buf.Reset()
			err := app.Upgrade(c.flag)
			if c.wantNew == "" {
				if err != nil || !strings.Contains(buf.String(), "Already at latest") {
					t.Errorf("Upgrade: %v %q", err, buf.String())
				}
			} else if err == nil || !strings.Contains(err.Error(), "in release "+c.wantNew) {
				t.Errorf("Upgrade should target %s, got %v", c.wantNew, err)
			}
		})
	}
}
