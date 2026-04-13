package internal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Files copied from source_dir into the cloud repo.
var cloudSyncFiles = []string{
	"settings.json",
	"settings.local.json",
	"CLAUDE.md",
}

// Directories copied recursively from source_dir into the cloud repo.
var cloudSyncDirs = []string{
	"commands",
	"agents",
}

// Files from other locations (relative to $HOME).
type externalSyncFile struct {
	HomePath string // relative to $HOME
	RepoPath string // path inside the cloud repo
}

var cloudSyncExternal = []externalSyncFile{
	{".claude/plugins/installed_plugins.json", "plugins/installed_plugins.json"},
	{".claude/plugins/known_marketplaces.json", "plugins/known_marketplaces.json"},
	{".agents/.skill-lock.json", "skills/skill-lock.json"},
}

const cloudGitignore = `# CPM Cloud Sync — auto-generated
.credentials.json
projects/
sessions/
statsig/
todos/
*.log
.DS_Store
`

// CloudRepoDir returns the path to the cloud sync git repo.
func CloudRepoDir(configPath string) string {
	return filepath.Join(ProfilesBaseDir(configPath), "cloud")
}

// CloudInit initializes or clones the cloud sync repo.
func CloudInit(configPath string, remote string) error {
	repoDir := CloudRepoDir(configPath)

	// Check if already initialized
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil {
		return fmt.Errorf("cloud repo already initialized at %s\nUse 'cpm cloud push' to sync, or remove the directory to reinitialize", repoDir)
	}

	// Check git is available
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH — install git first")
	}

	// If remote is provided, try cloning first
	if remote != "" {
		hasRefs, _ := checkRemoteHasRefs(remote)
		if hasRefs {
			fmt.Printf("Cloning from %s...\n", remote)
			if _, err := gitExecDir("", "clone", remote, repoDir); err != nil {
				return fmt.Errorf("cannot clone remote: %w", err)
			}
			// Distribute files from cloned repo
			cfg, err := LoadCloudConfig(configPath)
			if err != nil {
				return err
			}
			if err := DistributeSyncFiles(repoDir, cfg); err != nil {
				return fmt.Errorf("distributing files: %w", err)
			}
			// Save remote to config
			if err := saveCloudRemote(configPath, remote); err != nil {
				return fmt.Errorf("saving remote to config: %w", err)
			}
			fmt.Println("\nCloud repo cloned and files distributed.")
			fmt.Println("Run 'cpm cloud status' to verify.")
			return nil
		}
	}

	// Fresh init
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return fmt.Errorf("cannot create cloud dir: %w", err)
	}

	if _, err := gitExec(repoDir, "init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	// Write .gitignore
	if err := os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(cloudGitignore), 0o644); err != nil {
		return fmt.Errorf("cannot write .gitignore: %w", err)
	}

	// Gather and copy syncable files
	cfg, err := LoadCloudConfig(configPath)
	if err != nil {
		return err
	}
	files, err := GatherSyncFiles(cfg, configPath)
	if err != nil {
		return err
	}

	copied := 0
	for repoPath, srcPath := range files {
		dst := filepath.Join(repoDir, repoPath)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := cloudCopyFile(srcPath, dst); err != nil {
			fmt.Printf("  skipped %s (%v)\n", repoPath, err)
			continue
		}
		fmt.Printf("  added %s\n", repoPath)
		copied++
	}

	if copied == 0 {
		fmt.Println("\nNo syncable files found. The repo is empty.")
		return nil
	}

	// Initial commit
	if _, err := gitExec(repoDir, "add", "-A"); err != nil {
		return err
	}
	if _, err := gitExec(repoDir, "commit", "-m", "Initial cloud sync"); err != nil {
		return err
	}

	// Add remote if provided
	if remote != "" {
		if _, err := gitExec(repoDir, "remote", "add", "origin", remote); err != nil {
			return fmt.Errorf("cannot add remote: %w", err)
		}
		if err := saveCloudRemote(configPath, remote); err != nil {
			return fmt.Errorf("saving remote to config: %w", err)
		}
		fmt.Printf("\nRemote set to: %s\n", remote)
	}

	fmt.Printf("\nCloud repo initialized at %s (%d files)\n", repoDir, copied)
	if remote == "" {
		fmt.Println("\nTo add a remote: cpm cloud remote <url>")
	} else {
		fmt.Println("Push with: cpm cloud push")
	}

	return nil
}

// CloudPush gathers syncable files, commits, and pushes to remote.
func CloudPush(configPath string, message string) error {
	repoDir := CloudRepoDir(configPath)
	if err := ensureCloudRepo(repoDir); err != nil {
		return err
	}

	cfg, err := LoadCloudConfig(configPath)
	if err != nil {
		return err
	}

	// Gather and copy files
	files, err := GatherSyncFiles(cfg, configPath)
	if err != nil {
		return err
	}

	for repoPath, srcPath := range files {
		dst := filepath.Join(repoDir, repoPath)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := cloudCopyFile(srcPath, dst); err != nil {
			continue // skip missing files silently
		}
	}

	// Also sync directories — remove files from repo dirs that no longer exist in source
	for _, dirname := range cloudSyncDirs {
		if isExcluded(dirname+"/", cfg) {
			continue
		}
		repoSubDir := filepath.Join(repoDir, dirname)
		if _, err := os.Stat(repoSubDir); err == nil {
			// Clean files in repo dir that no longer exist in source
			srcDir := filepath.Join(cfg.SourceDir, dirname)
			cleanDeletedFiles(repoSubDir, srcDir)
		}
	}

	// Stage all changes
	if _, err := gitExec(repoDir, "add", "-A"); err != nil {
		return err
	}

	// Check if there are changes
	status, err := gitExec(repoDir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		fmt.Println("Already up to date — no changes to push.")
		return nil
	}

	// Commit
	if message == "" {
		hostname, _ := os.Hostname()
		message = fmt.Sprintf("cpm sync: %s at %s", hostname, time.Now().Format(time.RFC3339))
	}
	if _, err := gitExec(repoDir, "commit", "-m", message); err != nil {
		return err
	}

	// Show what changed
	diff, _ := gitExec(repoDir, "diff", "--stat", "HEAD~1..HEAD")
	if diff != "" {
		fmt.Println(strings.TrimSpace(diff))
	}

	// Push if remote configured
	hasRemote := hasOriginRemote(repoDir)
	if hasRemote {
		// Ensure main branch exists on remote
		if _, err := gitExec(repoDir, "push", "-u", "origin", "HEAD"); err != nil {
			return fmt.Errorf("push failed: %w\nResolve manually in %s", err, repoDir)
		}
		fmt.Println("\nPushed to remote.")
	} else {
		fmt.Println("\nCommitted locally. Add a remote with: cpm cloud remote <url>")
	}

	return nil
}

// CloudPull pulls from remote and distributes files to their live locations.
func CloudPull(configPath string, dryRun bool) error {
	repoDir := CloudRepoDir(configPath)
	if err := ensureCloudRepo(repoDir); err != nil {
		return err
	}

	if !hasOriginRemote(repoDir) {
		return fmt.Errorf("no remote configured — add one with: cpm cloud remote <url>")
	}

	// Pull with --ff-only
	output, err := gitExec(repoDir, "pull", "--ff-only", "origin", "HEAD")
	if err != nil {
		return fmt.Errorf("pull failed (possible divergence): %w\nResolve manually in %s\nor force overwrite with: cd %s && git reset --hard origin/main", err, repoDir, repoDir)
	}

	if strings.Contains(output, "Already up to date") {
		fmt.Println("Already up to date.")
		if !dryRun {
			return nil
		}
	}

	cfg, err := LoadCloudConfig(configPath)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Println("\nDry run — files that would be updated:")
		return showPullDiff(repoDir, cfg, configPath)
	}

	if err := DistributeSyncFiles(repoDir, cfg); err != nil {
		return err
	}

	// Also distribute cpm config.toml with merge
	repoConfigPath := filepath.Join(repoDir, "cpm", "config.toml")
	if _, err := os.Stat(repoConfigPath); err == nil {
		if err := mergeConfigTOML(repoConfigPath, configPath); err != nil {
			fmt.Printf("  warning: could not merge config.toml: %v\n", err)
		}
	}

	fmt.Println("\nFiles distributed from cloud repo.")
	return nil
}

// CloudStatus shows the state of the cloud sync repo.
func CloudStatus(configPath string) error {
	repoDir := CloudRepoDir(configPath)

	// Check if initialized
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		fmt.Println("Cloud sync not initialized.")
		fmt.Println("Run 'cpm cloud init' to get started.")
		return nil
	}

	fmt.Printf("Cloud repo: %s\n", repoDir)

	// Remote
	if hasOriginRemote(repoDir) {
		remote, _ := gitExec(repoDir, "remote", "get-url", "origin")
		fmt.Printf("Remote:     %s", remote)
	} else {
		fmt.Println("Remote:     (none)")
	}

	// Last commit
	log, _ := gitExec(repoDir, "log", "--oneline", "-5")
	if log != "" {
		fmt.Printf("\nRecent syncs:\n")
		for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	// Local changes
	cfg, err := LoadCloudConfig(configPath)
	if err != nil {
		return err
	}

	fmt.Println("\nLocal changes (not yet pushed):")
	hasChanges := false
	files, _ := GatherSyncFiles(cfg, configPath)
	for repoPath, srcPath := range files {
		dstPath := filepath.Join(repoDir, repoPath)
		if filesAreDifferent(srcPath, dstPath) {
			fmt.Printf("  modified: %s\n", repoPath)
			hasChanges = true
		}
	}
	if !hasChanges {
		fmt.Println("  (no changes)")
	}

	return nil
}

// CloudRemote sets or updates the remote URL.
func CloudRemote(configPath string, url string) error {
	repoDir := CloudRepoDir(configPath)
	if err := ensureCloudRepo(repoDir); err != nil {
		return err
	}

	if hasOriginRemote(repoDir) {
		if _, err := gitExec(repoDir, "remote", "set-url", "origin", url); err != nil {
			return err
		}
	} else {
		if _, err := gitExec(repoDir, "remote", "add", "origin", url); err != nil {
			return err
		}
	}

	if err := saveCloudRemote(configPath, url); err != nil {
		return fmt.Errorf("saving remote to config: %w", err)
	}

	fmt.Printf("Remote set to: %s\n", url)
	return nil
}

// GatherSyncFiles returns a map of repoPath -> absoluteSourcePath for all syncable files.
func GatherSyncFiles(cfg *Config, configPath string) (map[string]string, error) {
	home, _ := os.UserHomeDir()
	files := make(map[string]string)

	// Files from source_dir
	for _, name := range cloudSyncFiles {
		if isExcluded(name, cfg) {
			continue
		}
		src := filepath.Join(cfg.SourceDir, name)
		if _, err := os.Stat(src); err == nil {
			files[name] = src
		}
	}

	// Directories from source_dir
	for _, dirname := range cloudSyncDirs {
		if isExcluded(dirname+"/", cfg) {
			continue
		}
		srcDir := filepath.Join(cfg.SourceDir, dirname)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			continue
		}
		_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(cfg.SourceDir, path)
			files[rel] = path
			return nil
		})
	}

	// External files
	for _, ext := range cloudSyncExternal {
		if isExcluded(ext.RepoPath, cfg) {
			continue
		}
		src := filepath.Join(home, ext.HomePath)
		if _, err := os.Stat(src); err == nil {
			files[ext.RepoPath] = src
		}
	}

	// CPM config.toml
	if !isExcluded("cpm/config.toml", cfg) {
		cfgPath := ExpandPath(configPath)
		if _, err := os.Stat(cfgPath); err == nil {
			files["cpm/config.toml"] = cfgPath
		}
	}

	return files, nil
}

// DistributeSyncFiles copies files from the cloud repo back to their live locations.
func DistributeSyncFiles(repoDir string, cfg *Config) error {
	home, _ := os.UserHomeDir()

	// Files to source_dir
	for _, name := range cloudSyncFiles {
		src := filepath.Join(repoDir, name)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		dst := filepath.Join(cfg.SourceDir, name)
		if err := cloudCopyFile(src, dst); err != nil {
			fmt.Printf("  warning: cannot write %s: %v\n", name, err)
			continue
		}
		fmt.Printf("  restored %s\n", name)
	}

	// Directories to source_dir
	for _, dirname := range cloudSyncDirs {
		srcDir := filepath.Join(repoDir, dirname)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			continue
		}
		dstDir := filepath.Join(cfg.SourceDir, dirname)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			continue
		}
		_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(srcDir, path)
			dst := filepath.Join(dstDir, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil
			}
			if err := cloudCopyFile(path, dst); err != nil {
				return nil
			}
			fmt.Printf("  restored %s/%s\n", dirname, rel)
			return nil
		})
	}

	// External files
	for _, ext := range cloudSyncExternal {
		src := filepath.Join(repoDir, ext.RepoPath)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		dst := filepath.Join(home, ext.HomePath)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		if err := cloudCopyFile(src, dst); err != nil {
			fmt.Printf("  warning: cannot write %s: %v\n", ext.HomePath, err)
			continue
		}
		fmt.Printf("  restored %s\n", ext.RepoPath)
	}

	return nil
}

// --- helpers ---

func gitExec(repoDir string, args ...string) (string, error) {
	return gitExecDir(repoDir, args...)
}

func gitExecDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func ensureCloudRepo(repoDir string) error {
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		return fmt.Errorf("cloud repo not initialized — run 'cpm cloud init' first")
	}
	return nil
}

func hasOriginRemote(repoDir string) bool {
	_, err := gitExec(repoDir, "remote", "get-url", "origin")
	return err == nil
}

func checkRemoteHasRefs(remote string) (bool, error) {
	out, err := gitExecDir("", "ls-remote", "--heads", remote)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func cloudCopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func cloudCopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		return cloudCopyFile(path, target)
	})
}

func cleanDeletedFiles(repoSubDir, srcDir string) {
	_ = filepath.Walk(repoSubDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(repoSubDir, path)
		srcFile := filepath.Join(srcDir, rel)
		if _, err := os.Stat(srcFile); os.IsNotExist(err) {
			os.Remove(path)
		}
		return nil
	})
}

func filesAreDifferent(a, b string) bool {
	dataA, errA := os.ReadFile(a)
	dataB, errB := os.ReadFile(b)
	if errA != nil || errB != nil {
		return errA != errB // one exists, other doesn't
	}
	return string(dataA) != string(dataB)
}

func isExcluded(path string, cfg *Config) bool {
	if cfg.Cloud == nil {
		return false
	}
	for _, exc := range cfg.Cloud.Exclude {
		if exc == path || exc == strings.TrimSuffix(path, "/") {
			return true
		}
	}
	return false
}

func showPullDiff(repoDir string, cfg *Config, configPath string) error {
	home, _ := os.UserHomeDir()

	for _, name := range cloudSyncFiles {
		src := filepath.Join(repoDir, name)
		dst := filepath.Join(cfg.SourceDir, name)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		if filesAreDifferent(src, dst) {
			fmt.Printf("  would update: %s\n", name)
		}
	}

	for _, ext := range cloudSyncExternal {
		src := filepath.Join(repoDir, ext.RepoPath)
		dst := filepath.Join(home, ext.HomePath)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		if filesAreDifferent(src, dst) {
			fmt.Printf("  would update: %s\n", ext.RepoPath)
		}
	}

	return nil
}

func saveCloudRemote(configPath string, remote string) error {
	cfgPath := ExpandPath(configPath)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		// Config doesn't exist, create minimal one with cloud section
		content := fmt.Sprintf("[cloud]\nremote = %q\n", remote)
		return os.WriteFile(cfgPath, []byte(content), 0o644)
	}

	content := string(data)
	if strings.Contains(content, "[cloud]") {
		// Update existing cloud section's remote
		lines := strings.Split(content, "\n")
		found := false
		inCloud := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "[cloud]" {
				inCloud = true
				continue
			}
			if inCloud && strings.HasPrefix(trimmed, "[") {
				// Hit next section without finding remote
				break
			}
			if inCloud && strings.HasPrefix(trimmed, "remote") {
				lines[i] = fmt.Sprintf("remote = %q", remote)
				found = true
				break
			}
		}
		if !found {
			// Add remote under [cloud]
			for i, line := range lines {
				if strings.TrimSpace(line) == "[cloud]" {
					lines = append(lines[:i+1], append([]string{fmt.Sprintf("remote = %q", remote)}, lines[i+1:]...)...)
					break
				}
			}
		}
		return os.WriteFile(cfgPath, []byte(strings.Join(lines, "\n")), 0o644)
	}

	// Append cloud section
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += fmt.Sprintf("\n[cloud]\nremote = %q\n", remote)
	return os.WriteFile(cfgPath, []byte(content), 0o644)
}

func mergeConfigTOML(pulledPath, localConfigPath string) error {
	localPath := ExpandPath(localConfigPath)

	pulledData, err := os.ReadFile(pulledPath)
	if err != nil {
		return err
	}

	var pulled Config
	if err := toml.Unmarshal(pulledData, &pulled); err != nil {
		return err
	}

	localData, err := os.ReadFile(localPath)
	if err != nil {
		// No local config, just copy
		return cloudCopyFile(pulledPath, localPath)
	}

	var local Config
	if err := toml.Unmarshal(localData, &local); err != nil {
		return err
	}

	// Merge: add missing profiles from pulled, preserve local source_dir/bin_dir
	if local.Profiles == nil {
		local.Profiles = make(map[string]*Profile)
	}
	merged := false
	for name, profile := range pulled.Profiles {
		if _, exists := local.Profiles[name]; !exists {
			local.Profiles[name] = profile
			fmt.Printf("  added profile from cloud: %s\n", name)
			merged = true
		}
	}

	// Merge cloud config
	if pulled.Cloud != nil && local.Cloud == nil {
		local.Cloud = pulled.Cloud
		merged = true
	}

	if merged {
		// Write back — we preserve the original file and just append new profiles
		// to avoid losing formatting. Use simple append approach.
		content := string(localData)
		for name, profile := range pulled.Profiles {
			if _, exists := local.Profiles[name]; exists {
				// Skip already existing (handled above via the merged map check,
				// but we re-check against the original local data)
				continue
			}
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += fmt.Sprintf("\n[profiles.%s]\n", name)
			if profile.Description != "" {
				content += fmt.Sprintf("description = %q\n", profile.Description)
			}
			if profile.Model != "" {
				content += fmt.Sprintf("model = %q\n", profile.Model)
			}
		}
		return os.WriteFile(localPath, []byte(content), 0o644)
	}

	return nil
}
