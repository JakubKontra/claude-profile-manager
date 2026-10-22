package internal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MatchRule picks a profile for directories without a .claude-profile, by
// git remote, by path, or by both ([[match]] in config.toml).
type MatchRule struct {
	// Remote is a glob over the normalized remote URL (host/owner/repo).
	Remote string `toml:"remote,omitempty"`
	// Path is a glob over the directory; a leading ~ is the home directory.
	Path    string `toml:"path,omitempty"`
	Profile string `toml:"profile"`
}

func (r MatchRule) describe() string {
	var parts []string
	if r.Remote != "" {
		parts = append(parts, "remote "+r.Remote)
	}
	if r.Path != "" {
		parts = append(parts, "path "+r.Path)
	}
	return strings.Join(parts, ", ")
}

func validateMatchRules(rules []MatchRule) error {
	for i, r := range rules {
		switch {
		case r.Profile == "":
			return fmt.Errorf("match #%d: profile is required", i+1)
		case r.Remote == "" && r.Path == "":
			return fmt.Errorf("match #%d: set remote, path or both", i+1)
		}
	}
	return nil
}

// Resolution is the profile that applies to a directory and why.
type Resolution struct {
	Profile string
	// Source is "file" for a .claude-profile or "match" for a [[match]] rule.
	Source string
	// Rule is the matching rule, described as "remote github.com/x/*".
	Rule string
}

// origin describes where the resolution came from, for messages.
func (r Resolution) origin() string {
	if r.Source == "match" {
		return "[[match]] " + r.Rule
	}
	return ".claude-profile"
}

// ResolveProfile finds the profile for dir: the nearest .claude-profile
// wins, then the first [[match]] rule in file order. ok is false when
// neither applies.
func (a *App) ResolveProfile(cfg *Config, dir string) (res Resolution, ok bool) {
	if name, err := DetectProfileFile(dir); err == nil {
		return Resolution{Profile: name, Source: "file"}, true
	}
	if len(cfg.Match) == 0 {
		return Resolution{}, false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Resolution{}, false
	}
	var remotes []string
	remotesRead := false
	for _, r := range cfg.Match {
		if r.Path != "" && !globMatch(filepath.Clean(a.expand(r.Path)), abs) {
			continue
		}
		if r.Remote != "" {
			if !remotesRead {
				remotes, remotesRead = gitRemoteURLs(abs), true
			}
			if !remoteMatches(r.Remote, remotes) {
				continue
			}
		}
		return Resolution{Profile: r.Profile, Source: "match", Rule: r.describe()}, true
	}
	return Resolution{}, false
}

func remoteMatches(pattern string, remotes []string) bool {
	pattern = NormalizeRemote(pattern)
	for _, url := range remotes {
		if globMatch(pattern, NormalizeRemote(url)) {
			return true
		}
	}
	return false
}

// globMatch reports whether name matches pattern, where * matches within one
// path segment, ** matches across segments and a trailing /** also matches
// the directory itself.
func globMatch(pattern, name string) bool {
	var b strings.Builder
	b.WriteString("(?s)^") // ** spans any byte, newlines included
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case strings.HasPrefix(pattern[i:], "/**") && i+3 == len(pattern):
			b.WriteString("(/.*)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			// One byte at a time keeps multi-byte characters intact;
			// string(c) would turn each byte into its own rune.
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return err == nil && re.MatchString(name)
}

// NormalizeRemote reduces the forms of a git remote URL to host/path, so
// git@github.com:me/repo.git, ssh://git@github.com/me/repo and
// https://github.com/me/repo all become github.com/me/repo.
func NormalizeRemote(url string) string {
	u := strings.TrimSpace(url)
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
		if j := strings.IndexByte(u, '/'); j >= 0 {
			host := u[:j]
			if at := strings.LastIndexByte(host, '@'); at >= 0 {
				host = host[at+1:]
			}
			if colon := strings.IndexByte(host, ':'); colon >= 0 {
				host = host[:colon] // port
			}
			u = host + u[j:]
		}
	} else if colon := strings.IndexByte(u, ':'); colon >= 0 && !strings.Contains(u[:colon], "/") {
		// scp-like syntax: [user@]host:path
		host := u[:colon]
		if at := strings.LastIndexByte(host, '@'); at >= 0 {
			host = host[at+1:]
		}
		u = host + "/" + strings.TrimPrefix(u[colon+1:], "/")
	}
	u = strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
	if j := strings.IndexByte(u, '/'); j >= 0 {
		return strings.ToLower(u[:j]) + u[j:]
	}
	return strings.ToLower(u)
}

// gitRemoteURLs returns the remote URLs of the repository containing dir by
// reading its config file directly, so the shell hook never starts git.
func gitRemoteURLs(dir string) []string {
	configPath := findGitConfig(dir)
	if configPath == "" {
		return nil
	}
	f, err := os.Open(configPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var urls []string
	inRemote := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inRemote = strings.HasPrefix(line, "[remote ")
			continue
		}
		if !inRemote {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found && strings.EqualFold(strings.TrimSpace(key), "url") {
			urls = append(urls, strings.Trim(strings.TrimSpace(value), `"`))
		}
	}
	return urls
}

// findGitConfig walks up from dir to the nearest .git and returns the
// repository's config file. Worktrees and submodules use a .git file that
// points at the real git dir; linked worktrees share the main repo's config.
func findGitConfig(dir string) string {
	for {
		dotGit := filepath.Join(dir, ".git")
		if st, err := os.Stat(dotGit); err == nil {
			gitDir := dotGit
			if !st.IsDir() {
				gitDir = readGitDirFile(dotGit)
				if gitDir == "" {
					return ""
				}
			}
			if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
				common := strings.TrimSpace(string(data))
				if !filepath.IsAbs(common) {
					common = filepath.Join(gitDir, common)
				}
				gitDir = common
			}
			return filepath.Join(gitDir, "config")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func readGitDirFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return ""
	}
	gitDir = strings.TrimSpace(gitDir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(filepath.Dir(path), gitDir)
	}
	return gitDir
}
