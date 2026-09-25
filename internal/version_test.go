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
	captureOutput(t)

	if err := Upgrade(); err != nil {
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
	captureOutput(t)

	err := Upgrade()
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
	buf := captureOutput(t)

	if err := Upgrade(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Already at latest") {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestUpgradeRefusesHomebrewInstall(t *testing.T) {
	stubExecutable(t, "/opt/homebrew/Cellar/cpm/0.2.1/bin/cpm")
	err := Upgrade()
	if err == nil || !strings.Contains(err.Error(), "brew upgrade cpm") {
		t.Fatalf("expected Homebrew hint, got %v", err)
	}
}

func TestIsHomebrewPath(t *testing.T) {
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
	got, err := CheckLatestVersion()
	if err != nil || got != "v1.2.3" {
		t.Errorf("got %q, %v", got, err)
	}
}
