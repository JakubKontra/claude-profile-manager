package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

	cfg, err := LoadCloudConfig(configPath)
	if err != nil {
		return err
	}

	// If remote is provided, try cloning first
	if remote != "" {
		hasRefs, err := checkRemoteHasRefs(remote)
		if err != nil {
			return fmt.Errorf("cannot reach remote %s: %w", remote, err)
		}
		if hasRefs {
			outf("Cloning from %s...\n", remote)
			if _, err := gitExecDir("", "clone", remote, repoDir); err != nil {
				return fmt.Errorf("cannot clone remote: %w", err)
			}
			plan := PlanDistribute(repoDir, cfg, configPath)
			if err := ApplyDistribute(plan); err != nil {
				return fmt.Errorf("distributing files: %w", err)
			}
			if err := saveCloudRemote(configPath, remote); err != nil {
				return fmt.Errorf("saving remote to config: %w", err)
			}
			outln("\nCloud repo cloned and files distributed.")
			outln("Run 'cpm cloud status' to verify.")
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

	files, err := GatherSyncFiles(cfg, configPath)
	if err != nil {
		return err
	}

	copied := 0
	for _, repoPath := range sortedKeys(files) {
		srcPath := files[repoPath]
		dst := filepath.Join(repoDir, repoPath)
		if err := copyFile(srcPath, dst); err != nil {
			outf("  skipped %s (%v)\n", repoPath, err)
			continue
		}
		outf("  added %s\n", repoPath)
		copied++
	}

	if copied > 0 {
		if _, err := gitExec(repoDir, "add", "-A"); err != nil {
			return err
		}
		if _, err := gitExec(repoDir, "commit", "-m", "Initial cloud sync"); err != nil {
			return err
		}
	} else {
		outln("\nNo syncable files found. The repo is empty.")
	}

	// Add remote if provided
	if remote != "" {
		if _, err := gitExec(repoDir, "remote", "add", "origin", remote); err != nil {
			return fmt.Errorf("cannot add remote: %w", err)
		}
		if err := saveCloudRemote(configPath, remote); err != nil {
			return fmt.Errorf("saving remote to config: %w", err)
		}
		outf("\nRemote set to: %s\n", remote)
	}

	outf("\nCloud repo initialized at %s (%d files)\n", repoDir, copied)
	if remote == "" {
		outln("\nTo add a remote: cpm cloud remote <url>")
	} else {
		outln("Push with: cpm cloud push")
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
		if err := copyFile(srcPath, dst); err != nil {
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
		outln("Already up to date — no changes to push.")
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
	diff, _ := gitExec(repoDir, "show", "--stat", "--format=", "HEAD")
	if diff != "" {
		outln(strings.TrimSpace(diff))
	}

	// Push if remote configured
	hasRemote := hasOriginRemote(repoDir)
	if hasRemote {
		// Ensure main branch exists on remote
		if _, err := gitExec(repoDir, "push", "-u", "origin", "HEAD"); err != nil {
			return fmt.Errorf("push failed: %w\nResolve manually in %s", err, repoDir)
		}
		outln("\nPushed to remote.")
	} else {
		outln("\nCommitted locally. Add a remote with: cpm cloud remote <url>")
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

	before, _ := gitExec(repoDir, "rev-parse", "HEAD")
	if _, err := gitExec(repoDir, "pull", "--ff-only", "origin", "HEAD"); err != nil {
		branch := currentBranch(repoDir)
		return fmt.Errorf("pull failed (possible divergence): %w\nResolve manually in %s\nor force overwrite with: cd %s && git reset --hard origin/%s", err, repoDir, repoDir, branch)
	}
	after, _ := gitExec(repoDir, "rev-parse", "HEAD")
	if strings.TrimSpace(before) == strings.TrimSpace(after) {
		outln("Repo already up to date.")
	}

	cfg, err := LoadCloudConfig(configPath)
	if err != nil {
		return err
	}

	plan := PlanDistribute(repoDir, cfg, configPath)
	if dryRun {
		outln("\nDry run — files that would change:")
		if !printPlan(plan) {
			outln("  (no changes)")
		}
		return nil
	}

	if err := ApplyDistribute(plan); err != nil {
		return err
	}

	outln("\nFiles distributed from cloud repo.")
	return nil
}

// CloudStatus shows the state of the cloud sync repo.
func CloudStatus(configPath string) error {
	repoDir := CloudRepoDir(configPath)

	// Check if initialized
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		outln("Cloud sync not initialized.")
		outln("Run 'cpm cloud init' to get started.")
		return nil
	}

	outf("Cloud repo: %s\n", repoDir)

	// Remote
	if hasOriginRemote(repoDir) {
		remote, _ := gitExec(repoDir, "remote", "get-url", "origin")
		outf("Remote:     %s", remote)
	} else {
		outln("Remote:     (none)")
	}

	// Last commit
	log, _ := gitExec(repoDir, "log", "--oneline", "-5")
	if log != "" {
		outf("\nRecent syncs:\n")
		for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
			outf("  %s\n", line)
		}
	}

	// Local changes
	cfg, err := LoadCloudConfig(configPath)
	if err != nil {
		return err
	}

	outln("\nLocal changes (not yet pushed):")
	hasChanges := false
	files, _ := GatherSyncFiles(cfg, configPath)
	for repoPath, srcPath := range files {
		dstPath := filepath.Join(repoDir, repoPath)
		if filesAreDifferent(srcPath, dstPath) {
			outf("  modified: %s\n", repoPath)
			hasChanges = true
		}
	}
	if !hasChanges {
		outln("  (no changes)")
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

	outf("Remote set to: %s\n", url)
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

// DistributeAction is one step of applying the cloud repo locally.
type DistributeAction struct {
	// RepoPath is the path inside the cloud repo.
	RepoPath string
	// Source is the absolute file in the cloud repo (empty for deletes).
	Source string
	// Dest is the live path that changes.
	Dest string
	// Kind is create, update, delete, merge, unchanged or excluded.
	Kind string
}

// PlanDistribute compares the cloud repo with the live files and returns
// what applying it would do. Deletions are only planned inside the synced
// directories (commands/, agents/, ...), never for top-level files.
func PlanDistribute(repoDir string, cfg *Config, configPath string) []DistributeAction {
	home, _ := os.UserHomeDir()
	var plan []DistributeAction

	add := func(repoPath, dest string, excluded bool) {
		src := filepath.Join(repoDir, repoPath)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			return
		}
		kind := "unchanged"
		switch {
		case excluded:
			kind = "excluded"
		case !fileExistsAt(dest):
			kind = "create"
		case filesAreDifferent(src, dest):
			kind = "update"
		}
		plan = append(plan, DistributeAction{RepoPath: repoPath, Source: src, Dest: dest, Kind: kind})
	}

	for _, name := range cloudSyncFiles {
		add(name, filepath.Join(cfg.SourceDir, name), isExcluded(name, cfg))
	}

	for _, dirname := range cloudSyncDirs {
		repoSubDir := filepath.Join(repoDir, dirname)
		if _, err := os.Stat(repoSubDir); os.IsNotExist(err) {
			continue
		}
		excluded := isExcluded(dirname+"/", cfg)
		liveDir := filepath.Join(cfg.SourceDir, dirname)

		inRepo := map[string]bool{}
		_ = filepath.Walk(repoSubDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(repoSubDir, path)
			inRepo[rel] = true
			add(filepath.Join(dirname, rel), filepath.Join(liveDir, rel), excluded)
			return nil
		})
		if excluded {
			continue
		}
		// Files that exist live but no longer in the repo were deleted
		// on another machine.
		_ = filepath.Walk(liveDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(liveDir, path)
			if !inRepo[rel] {
				plan = append(plan, DistributeAction{RepoPath: filepath.Join(dirname, rel), Dest: path, Kind: "delete"})
			}
			return nil
		})
	}

	for _, ext := range cloudSyncExternal {
		add(ext.RepoPath, filepath.Join(home, ext.HomePath), isExcluded(ext.RepoPath, cfg))
	}

	if configPath != "" && !isExcluded("cpm/config.toml", cfg) {
		src := filepath.Join(repoDir, "cpm", "config.toml")
		if _, err := os.Stat(src); err == nil {
			plan = append(plan, DistributeAction{RepoPath: "cpm/config.toml", Source: src, Dest: ExpandPath(configPath), Kind: "merge"})
		}
	}

	sort.SliceStable(plan, func(i, j int) bool { return plan[i].RepoPath < plan[j].RepoPath })
	return plan
}

// ApplyDistribute executes a plan.
func ApplyDistribute(plan []DistributeAction) error {
	for _, a := range plan {
		switch a.Kind {
		case "create", "update":
			if err := copyFile(a.Source, a.Dest); err != nil {
				outf("  warning: cannot write %s: %v\n", a.RepoPath, err)
				continue
			}
			outf("  restored %s\n", a.RepoPath)
		case "delete":
			if err := os.Remove(a.Dest); err != nil && !os.IsNotExist(err) {
				outf("  warning: cannot delete %s: %v\n", a.Dest, err)
				continue
			}
			outf("  deleted %s\n", a.RepoPath)
		case "merge":
			if err := mergeConfigTOML(a.Source, a.Dest); err != nil {
				outf("  warning: could not merge config.toml: %v\n", err)
			}
		}
	}
	return nil
}

// DistributeSyncFiles applies the repo to the live locations (no config merge).
func DistributeSyncFiles(repoDir string, cfg *Config) error {
	return ApplyDistribute(PlanDistribute(repoDir, cfg, ""))
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

// checkRemoteHasRefs reports whether the remote already has branches. A
// remote that cannot be reached is an error, not "empty".
func checkRemoteHasRefs(remote string) (bool, error) {
	out, err := gitExecDir("", "ls-remote", "--heads", remote)
	if err != nil {
		return false, fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return strings.TrimSpace(out) != "", nil
}

func currentBranch(repoDir string) string {
	out, err := gitExec(repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(out)
}

// printPlan prints the actions that change something and reports whether
// there were any.
func printPlan(plan []DistributeAction) bool {
	any := false
	for _, a := range plan {
		switch a.Kind {
		case "create", "update", "delete", "merge":
			outf("  would %s: %s\n", a.Kind, a.RepoPath)
			any = true
		}
	}
	return any
}

func fileExistsAt(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

func saveCloudRemote(configPath string, remote string) error {
	cfgPath := ExpandPath(configPath)
	data, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	out, err := SetTableKey(data, "cloud", "remote", quoteTOMLString(remote))
	if err != nil {
		return err
	}
	return writeConfigAtomic(cfgPath, out)
}

// mergeConfigTOML adds profiles that exist in the pulled config but not
// locally. source_dir, bin_dir and existing profiles are left untouched.
func mergeConfigTOML(pulledPath, localConfigPath string) error {
	localPath := ExpandPath(localConfigPath)

	pulledData, err := os.ReadFile(pulledPath)
	if err != nil {
		return err
	}
	var pulled Config
	if _, err := toml.Decode(string(pulledData), &pulled); err != nil {
		return err
	}

	localData, err := os.ReadFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return copyFile(pulledPath, localPath)
		}
		return err
	}
	var local Config
	if _, err := toml.Decode(string(localData), &local); err != nil {
		return err
	}

	// Decide what is missing before touching anything.
	var missing []string
	for _, name := range SortedProfileNames(&pulled) {
		if _, exists := local.Profiles[name]; !exists {
			missing = append(missing, name)
		}
	}
	addCloud := pulled.Cloud != nil && local.Cloud == nil
	if len(missing) == 0 && !addCloud {
		return nil
	}

	out := localData
	for _, name := range missing {
		out, err = AppendProfileTable(out, name, pulled.Profiles[name])
		if err != nil {
			return err
		}
		outf("  added profile from cloud: %s\n", name)
	}
	if addCloud {
		if pulled.Cloud.Remote != "" {
			if out, err = SetTableKey(out, "cloud", "remote", quoteTOMLString(pulled.Cloud.Remote)); err != nil {
				return err
			}
		}
		if pulled.Cloud.AutoPush {
			if out, err = SetTableKey(out, "cloud", "auto_push", "true"); err != nil {
				return err
			}
		}
	}
	return writeConfigAtomic(localPath, out)
}
