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
	"strconv"
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
	TagName    string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// fetchLatestRelease returns the newest release. GitHub's /releases/latest
// never returns a prerelease; with prerelease the newest of the recent
// releases is picked instead, prereleases included.
func fetchLatestRelease(prerelease bool) (githubRelease, error) {
	if !prerelease {
		var release githubRelease
		err := getJSON(fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIBase, repoOwner, repoName), &release)
		return release, err
	}
	var releases []githubRelease
	if err := getJSON(fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30", githubAPIBase, repoOwner, repoName), &releases); err != nil {
		return githubRelease{}, err
	}
	var newest githubRelease
	for _, r := range releases {
		if !r.Draft && (newest.TagName == "" || compareVersions(r.TagName, newest.TagName) > 0) {
			newest = r
		}
	}
	if newest.TagName == "" {
		return githubRelease{}, fmt.Errorf("no releases found")
	}
	return newest, nil
}

func getJSON(url string, v any) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("cannot fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("cannot parse release info: %w", err)
	}
	return nil
}

// CheckLatestVersion returns the newest release tag; see fetchLatestRelease.
func CheckLatestVersion(prerelease bool) (string, error) {
	release, err := fetchLatestRelease(prerelease)
	if err != nil {
		return "", err
	}
	return release.TagName, nil
}

// wantsPrerelease reports whether update checks include prereleases: when
// asked to, or when this build is itself a prerelease.
func wantsPrerelease(flag bool) bool {
	_, pre, ok := parseVersion(Version)
	return flag || (ok && pre != "")
}

// isNewer reports whether tag is a newer release than this build. A build
// whose version does not parse (such as "dev") takes any other tag.
func isNewer(tag string) bool {
	if _, _, ok := parseVersion(Version); !ok {
		return strings.TrimPrefix(tag, "v") != strings.TrimPrefix(Version, "v")
	}
	return compareVersions(tag, Version) > 0
}

// parseVersion splits "v1.2.3-rc.1" into [1 2 3] and "rc.1". Build metadata
// after "+" is ignored.
func parseVersion(v string) (core [3]int, pre string, ok bool) {
	v, _, _ = strings.Cut(strings.TrimPrefix(v, "v"), "+")
	v, pre, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return core, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return core, "", false
		}
		core[i] = n
	}
	return core, pre, true
}

// compareVersions orders two versions by semver precedence: -1, 0 or 1.
// Versions that do not parse sort before those that do.
func compareVersions(a, b string) int {
	ca, pa, oka := parseVersion(a)
	cb, pb, okb := parseVersion(b)
	switch {
	case !oka || !okb:
		return cmpBool(oka, okb)
	case ca != cb:
		for i := range ca {
			if ca[i] != cb[i] {
				return cmpInt(ca[i], cb[i])
			}
		}
	case pa == pb:
		return 0
	case pa == "" || pb == "":
		// A release sorts after its prereleases.
		return cmpBool(pa == "", pb == "")
	}
	return comparePrerelease(pa, pb)
}

// comparePrerelease compares dot-separated identifiers: numeric ones
// numerically and before alphanumeric ones, and a longer list wins a tie.
func comparePrerelease(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		na, errA := strconv.Atoi(as[i])
		nb, errB := strconv.Atoi(bs[i])
		switch {
		case errA == nil && errB == nil:
			if na != nb {
				return cmpInt(na, nb)
			}
		case errA == nil || errB == nil:
			return cmpBool(errA != nil, errB != nil)
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(as), len(bs))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// cmpBool orders false before true.
func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case b:
		return -1
	}
	return 1
}

// isHomebrewPath reports whether a binary lives inside a Homebrew prefix, in
// which case Homebrew should manage upgrades.
func isHomebrewPath(path string) bool {
	return strings.Contains(path, "/Cellar/") || strings.Contains(path, "/homebrew/") || strings.Contains(path, "/linuxbrew/")
}

// Upgrade replaces the running cpm binary with the latest GitHub release,
// after verifying its SHA-256 against the published checksums.txt. It never
// installs an older version. Prereleases are considered with prerelease or
// when the running build is one.
func (a *App) Upgrade(prerelease bool) error {
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

	release, err := fetchLatestRelease(wantsPrerelease(prerelease))
	if err != nil {
		return err
	}

	if !isNewer(release.TagName) {
		a.outf("Already at latest version: %s\n", Version)
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

	a.outf("Downloading %s...\n", release.TagName)

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

	a.outf("Updated %s to %s\n", exe, release.TagName)
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
