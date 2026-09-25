package internal

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Set via ldflags at build time
var (
	Version = "dev"
	Commit  = "unknown"
)

// httpClient bounds every network call so 'cpm version' cannot hang offline.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// githubAPIBase is overridden in tests to point at a local server.
var githubAPIBase = "https://api.github.com"

// executablePath is overridden in tests.
var executablePath = os.Executable

const repoOwner = "jakubkontra"
const repoName = "claude-profile-manager"

const checksumsAsset = "checksums.txt"

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func fetchLatestRelease() (githubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIBase, repoOwner, repoName)

	resp, err := httpClient.Get(url)
	if err != nil {
		return githubRelease{}, fmt.Errorf("cannot fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("cannot parse release info: %w", err)
	}
	return release, nil
}

func CheckLatestVersion() (string, error) {
	release, err := fetchLatestRelease()
	if err != nil {
		return "", err
	}
	return release.TagName, nil
}

// isHomebrewPath reports whether a binary lives inside a Homebrew prefix, in
// which case Homebrew should manage upgrades.
func isHomebrewPath(path string) bool {
	return strings.Contains(path, "/Cellar/") || strings.Contains(path, "/homebrew/") || strings.Contains(path, "/linuxbrew/")
}

// Upgrade replaces the running cpm binary with the latest GitHub release,
// after verifying its SHA-256 against the published checksums.txt.
func Upgrade() error {
	exe, err := executablePath()
	if err != nil {
		return fmt.Errorf("cannot locate running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if isHomebrewPath(exe) {
		return fmt.Errorf("cpm is managed by Homebrew (%s)\nRun: brew upgrade cpm", exe)
	}

	release, err := fetchLatestRelease()
	if err != nil {
		return err
	}

	if release.TagName == "v"+Version || release.TagName == Version {
		outf("Already at latest version: %s\n", Version)
		return nil
	}

	assetName := fmt.Sprintf("cpm_%s_%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL, checksumsURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case checksumsAsset:
			checksumsURL = asset.BrowserDownloadURL
		case assetName:
			downloadURL = asset.BrowserDownloadURL
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}
	if checksumsURL == "" {
		return fmt.Errorf("release %s has no %s; refusing to install an unverified binary", release.TagName, checksumsAsset)
	}

	checksums, err := fetchChecksums(checksumsURL)
	if err != nil {
		return err
	}
	want, ok := checksums[assetName]
	if !ok {
		return fmt.Errorf("%s has no entry for %s", checksumsAsset, assetName)
	}

	outf("Downloading %s...\n", release.TagName)

	tmpPath, err := downloadToTemp(downloadURL, filepath.Dir(exe))
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath) // no-op after a successful rename

	if err := verifySHA256(tmpPath, want); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		return fmt.Errorf("cannot replace binary: %w", err)
	}

	outf("Updated %s to %s\n", exe, release.TagName)
	return nil
}

func downloadToTemp(url, dir string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("cannot download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(dir, "cpm-update-*")
	if err != nil {
		return "", fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("cannot write binary: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

// fetchChecksums parses goreleaser's checksums.txt ("<sha256>  <name>" lines).
func fetchChecksums(url string) (map[string]string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("cannot download %s: %w", checksumsAsset, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s download returned %d", checksumsAsset, resp.StatusCode)
	}

	sums := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		sums[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return sums, scanner.Err()
}

func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(want) {
		return fmt.Errorf("checksum mismatch: got %s, want %s — refusing to install", got, want)
	}
	return nil
}
