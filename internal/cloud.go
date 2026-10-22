package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Files copied from source_dir into the cloud repo. settings.local.json is
// machine-specific by definition and is not synced unless listed in
// cloud.include.
var cloudSyncFiles = []string{
	"settings.json",
	"CLAUDE.md",
}

// Directories copied recursively from source_dir into the cloud repo.
var cloudSyncDirs = []string{
	"commands",
	"agents",
	"skills",
}

// syncFilesFor returns the top-level files to sync: the defaults plus
// cloud.include entries, minus cloud.exclude.
func syncFilesFor(cfg *Config) []string {
	files := append([]string{}, cloudSyncFiles...)
	if cfg.Cloud != nil {
		for _, extra := range cfg.Cloud.Include {
			extra = strings.TrimPrefix(filepath.Clean(extra), "./")
			if filepath.Base(extra) == "secrets.toml" || filepath.Base(extra) == ".credentials.json" || extra == "" || extra == "." || strings.HasPrefix(extra, "..") || filepath.IsAbs(extra) {
				continue
			}
			files = append(files, extra)
		}
	}
	return files
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
secrets.toml
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
func (a *App) CloudInit(remote string) error {
	return a.CloudInitWithSecrets(remote, false)
}

func (a *App) CloudInitWithSecrets(remote string, allowSecrets bool) error {
	repoDir := CloudRepoDir(a.ConfigPath)

	// Check if already initialized
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil {
		return fmt.Errorf("cloud repo already initialized at %s\nUse 'cpm cloud push' to sync, or remove the directory to reinitialize", repoDir)
	}

	// Check git is available
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH — install git first")
	}

	cfg, err := a.LoadCloudConfig()
	if err != nil {
		return err
	}

	// If remote is provided, try cloning first
	if remote != "" {
		hasRefs, err := a.checkRemoteHasRefs(remote)
		if err != nil {
			return fmt.Errorf("cannot reach remote %s: %w", remote, err)
		}
		if hasRefs {
			a.outf("Cloning from %s...\n", remote)
			if _, err := a.git("", "clone", remote, repoDir); err != nil {
				return fmt.Errorf("cannot clone remote: %w", err)
			}
			plan := a.PlanDistribute(repoDir, cfg, a.ConfigPath)
			if err := a.ApplyDistribute(plan); err != nil {
				return fmt.Errorf("distributing files: %w", err)
			}
			if err := saveCloudRemote(a.expand(a.ConfigPath), remote); err != nil {
				return fmt.Errorf("saving remote to config: %w", err)
			}
			a.outln("\nCloud repo cloned and files distributed.")
			a.outln("Run 'cpm cloud status' to verify.")
			return nil
		}
	}

	files, err := a.GatherSyncFiles(cfg, a.ConfigPath)
	if err != nil {
		return err
	}
	if err := a.checkSyncSecrets(files, allowSecrets || (cfg.Cloud != nil && cfg.Cloud.AllowSecrets)); err != nil {
		return err
	}
	// Fresh init
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return fmt.Errorf("cannot create cloud dir: %w", err)
	}

	if _, err := a.git(repoDir, "init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	// Write .gitignore
	if err := os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(cloudGitignore), 0o644); err != nil {
		return fmt.Errorf("cannot write .gitignore: %w", err)
	}

	copied := 0
	for _, repoPath := range sortedKeys(files) {
		srcPath := files[repoPath]
		dst := filepath.Join(repoDir, repoPath)
		if err := copyFile(srcPath, dst); err != nil {
			return fmt.Errorf("copy %s: %w", repoPath, err)
		}
		a.outf("  added %s\n", repoPath)
		copied++
	}

	if copied > 0 {
		if _, err := a.git(repoDir, "add", "-A"); err != nil {
			return err
		}
		if _, err := a.git(repoDir, "commit", "-m", "Initial cloud sync"); err != nil {
			return err
		}
	} else {
		a.outln("\nNo syncable files found. The repo is empty.")
	}

	// Add remote if provided
	if remote != "" {
		if _, err := a.git(repoDir, "remote", "add", "origin", remote); err != nil {
			return fmt.Errorf("cannot add remote: %w", err)
		}
		if err := saveCloudRemote(a.expand(a.ConfigPath), remote); err != nil {
			return fmt.Errorf("saving remote to config: %w", err)
		}
		a.outf("\nRemote set to: %s\n", remote)
	}

	a.outf("\nCloud repo initialized at %s (%d files)\n", repoDir, copied)
	if remote == "" {
		a.outln("\nTo add a remote: cpm cloud remote <url>")
	} else {
		a.outln("Push with: cpm cloud push")
	}

	return a.recordCloudState(repoDir, cfg, a.ConfigPath)
}

// PushOptions controls CloudPush.
type PushOptions struct {
	Message string
	// DryRun stages into the repo working tree and shows the status
	// without committing or pushing.
	DryRun bool
	// AllowSecrets pushes even when settings contain API keys.
	AllowSecrets bool
}

// CloudPush gathers syncable files, commits, and pushes to remote.
func (a *App) CloudPush(opts PushOptions) error {
	repoDir := CloudRepoDir(a.ConfigPath)
	if err := ensureCloudRepo(repoDir); err != nil {
		return err
	}

	cfg, err := a.LoadCloudConfig()
	if err != nil {
		return err
	}

	files, err := a.GatherSyncFiles(cfg, a.ConfigPath)
	if err != nil {
		return err
	}

	if err := a.checkSyncSecrets(files, opts.AllowSecrets || (cfg.Cloud != nil && cfg.Cloud.AllowSecrets)); err != nil {
		return err
	}

	if err := a.stageSyncFiles(repoDir, cfg, files); err != nil {
		return err
	}

	// Check if there are changes
	status, err := a.git(repoDir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		a.outln("No new local changes.")
		if opts.DryRun {
			return nil
		}
		if a.hasOriginRemote(repoDir) {
			if _, err := a.git(repoDir, "push", "-u", "origin", "HEAD"); err != nil {
				return err
			}
		}
		return a.recordCloudState(repoDir, cfg, a.ConfigPath)
	}

	if opts.DryRun {
		a.outln("Dry run — changes that would be committed:")
		for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
			a.outf("  %s\n", line)
		}
		return nil
	}

	message := opts.Message
	if message == "" {
		hostname, _ := os.Hostname()
		message = fmt.Sprintf("cpm sync: %s at %s", hostname, time.Now().Format(time.RFC3339))
	}
	if _, err := a.git(repoDir, "commit", "-m", message); err != nil {
		return err
	}

	// Show what changed
	diff, _ := a.git(repoDir, "show", "--stat", "--format=", "HEAD")
	if diff != "" {
		a.outln(strings.TrimSpace(diff))
	}

	// Push if remote configured
	if a.hasOriginRemote(repoDir) {
		if _, err := a.git(repoDir, "push", "-u", "origin", "HEAD"); err != nil {
			return fmt.Errorf("push failed: %w\nResolve manually in %s", err, repoDir)
		}
		a.outln("\nPushed to remote.")
	} else {
		a.outln("\nCommitted locally. Add a remote with: cpm cloud remote <url>")
	}

	return a.recordCloudState(repoDir, cfg, a.ConfigPath)
}

// CloudDiff stages the live files into the repo and shows the diff against
// the last sync.
// CloudDiff stages the local files in the cloud repo and returns what
// changed since the last sync: the file list and the full patch.
func (a *App) CloudDiff() (CloudDiffReport, error) {
	report := CloudDiffReport{Files: []CloudDiffFile{}}
	repoDir := CloudRepoDir(a.ConfigPath)
	if err := ensureCloudRepo(repoDir); err != nil {
		return report, err
	}
	cfg, err := a.LoadCloudConfig()
	if err != nil {
		return report, err
	}
	files, err := a.GatherSyncFiles(cfg, a.ConfigPath)
	if err != nil {
		return report, err
	}
	if err := a.stageSyncFiles(repoDir, cfg, files); err != nil {
		return report, err
	}
	names, _ := a.git(repoDir, "diff", "--cached", "--name-status")
	for _, line := range strings.Split(strings.TrimSpace(names), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		report.Files = append(report.Files, CloudDiffFile{Path: fields[len(fields)-1], Status: diffStatus(fields[0])})
	}
	report.stat, _ = a.git(repoDir, "diff", "--cached", "--stat")
	report.patch, _ = a.git(repoDir, "diff", "--cached")
	return report, nil
}

// CloudDiffReport is the result of `cpm cloud diff`; the patch is only
// part of the text output.
type CloudDiffReport struct {
	Files []CloudDiffFile `json:"files"`
	stat  string
	patch string
}

// CloudDiffFile is one changed file: added, modified, deleted or renamed.
type CloudDiffFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func diffStatus(code string) string {
	switch code[:1] {
	case "A":
		return "added"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	}
	return "modified"
}

// RenderCloudDiff prints the diffstat and the patch.
func (a *App) RenderCloudDiff(r CloudDiffReport) {
	if strings.TrimSpace(r.patch) == "" {
		a.outln("No local changes since the last sync.")
		return
	}
	a.outln(strings.TrimSpace(r.stat))
	a.outln()
	a.out(r.patch)
}

// stageSyncFiles copies the live files into the repo working tree, removes
// files deleted locally from the synced directories, and runs git add -A.
func (a *App) stageSyncFiles(repoDir string, cfg *Config, files map[string]string) error {
	for repoPath, srcPath := range files {
		if err := copyFile(srcPath, filepath.Join(repoDir, repoPath)); err != nil {
			return fmt.Errorf("stage %s: %w", repoPath, err)
		}
	}
	for _, dirname := range cloudSyncDirs {
		if isExcluded(dirname+"/", cfg) {
			continue
		}
		repoSubDir := filepath.Join(repoDir, dirname)
		if _, err := os.Stat(repoSubDir); err == nil {
			if err := cleanDeletedFiles(repoSubDir, filepath.Join(cfg.SourceDir, dirname)); err != nil {
				return err
			}
		}
	}
	_, err := a.git(repoDir, "add", "-A")
	return err
}

// SecretFinding is a settings value that looks like a credential.
type SecretFinding struct {
	File string
	Key  string
}

var secretKeyPattern = regexp.MustCompile(`(?i)(_KEY|_TOKEN|_SECRET|PASSWORD|PASSWD)$`)
var secretValuePattern = regexp.MustCompile(`^(sk-ant-|sk-[A-Za-z0-9]|ghp_|gho_|github_pat_|xox[bpoa]-|AKIA[0-9A-Z]{12,}|glpat-)`)

// looksLikeSecret flags env entries whose key or value resembles an API
// credential.
func looksLikeSecret(key, value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	return secretKeyPattern.MatchString(key) || secretValuePattern.MatchString(value)
}

// ScanForSecrets inspects the "env" object of every settings*.json in the
// sync set.
func ScanForSecrets(files map[string]string) []SecretFinding {
	var findings []SecretFinding
	for _, path := range sortedKeys(files) {
		ext := filepath.Ext(path)
		if ext != ".json" && ext != ".toml" {
			continue
		}
		data, err := os.ReadFile(files[path])
		var value map[string]any
		if err == nil {
			if ext == ".json" {
				err = json.Unmarshal(data, &value)
			} else {
				_, err = toml.Decode(string(data), &value)
			}
		}
		if err != nil {
			findings = append(findings, SecretFinding{File: path, Key: "unreadable or invalid structured file"})
			continue
		}
		scanSecretValues(path, "", value, &findings)
	}
	return findings
}

func sortedAnyKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// cloudPullFiles is CloudPull without the profile re-sync, for callers that
// run the install pipeline themselves.
func (a *App) cloudPullFiles() error {
	repoDir := CloudRepoDir(a.ConfigPath)
	if err := ensureCloudRepo(repoDir); err != nil {
		return err
	}
	if !a.hasOriginRemote(repoDir) {
		return fmt.Errorf("no remote configured — add one with: cpm cloud remote <url>")
	}
	if _, err := a.git(repoDir, "pull", "--ff-only", "origin", "HEAD"); err != nil {
		return fmt.Errorf("pull failed (possible divergence): %w", err)
	}
	cfg, err := a.LoadCloudConfig()
	if err != nil {
		return err
	}
	return a.ApplyDistribute(a.PlanDistribute(repoDir, cfg, a.ConfigPath))
}

// CloudPull pulls from remote and distributes files to their live locations.
func (a *App) CloudPull(opts PullOptions) error {
	dryRun := opts.DryRun
	repoDir := CloudRepoDir(a.ConfigPath)
	if err := ensureCloudRepo(repoDir); err != nil {
		return err
	}

	if !a.hasOriginRemote(repoDir) {
		return fmt.Errorf("no remote configured — add one with: cpm cloud remote <url>")
	}

	before, _ := a.git(repoDir, "rev-parse", "HEAD")
	if _, err := a.git(repoDir, "pull", "--ff-only", "origin", "HEAD"); err != nil {
		branch := a.currentBranch(repoDir)
		return fmt.Errorf("pull failed (possible divergence): %w\nResolve manually in %s\nor force overwrite with: cd %s && git reset --hard origin/%s", err, repoDir, repoDir, branch)
	}
	after, _ := a.git(repoDir, "rev-parse", "HEAD")
	if strings.TrimSpace(before) == strings.TrimSpace(after) {
		a.outln("Repo already up to date.")
	}

	// Removals and renames first, so the config merge sees the result.
	if err := a.applyCloudChanges(repoDir, opts); err != nil {
		return err
	}

	cfg, err := a.LoadCloudConfig()
	if err != nil {
		return err
	}

	plan := a.PlanDistribute(repoDir, cfg, a.ConfigPath)
	if dryRun {
		a.outln("\nDry run — files that would change:")
		if !a.printPlan(plan) {
			a.outln("  (no changes)")
		}
		return nil
	}

	if err := a.ApplyDistribute(plan); err != nil {
		return err
	}

	a.outln("\nFiles distributed from cloud repo.")

	// Profiles hold copies of settings.json / CLAUDE.md; refresh them so the
	// pulled changes actually take effect.
	if full, err := a.LoadConfig(); err == nil {
		a.outln("\nRe-syncing profiles:")
		if err := a.InstallProfiles(full, InstallOptions{Sync: true, skipCloud: true}); err != nil {
			if errors.Is(err, ErrDiverged) {
				a.outln("Profiles with local changes were left alone; run 'cpm install --sync --force' to overwrite them.")
				return err
			}
			return err
		}
	}
	return nil
}

// CloudStatus shows the state of the cloud sync repo.
func (a *App) CloudStatus() (CloudStatusReport, error) {
	repoDir := CloudRepoDir(a.ConfigPath)
	report := CloudStatusReport{Repo: repoDir, Recent: []CloudCommit{}, Modified: []string{}}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		return report, nil
	}
	report.Initialized = true

	if a.hasOriginRemote(repoDir) {
		remote, _ := a.git(repoDir, "remote", "get-url", "origin")
		report.Remote = strings.TrimSpace(remote)
	}

	log, _ := a.git(repoDir, "log", "--format=%h %s", "-5")
	for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
		if hash, subject, ok := strings.Cut(line, " "); ok {
			report.Recent = append(report.Recent, CloudCommit{Hash: hash, Subject: subject})
		}
	}

	cfg, err := a.LoadCloudConfig()
	if err != nil {
		return report, err
	}
	files, _ := a.GatherSyncFiles(cfg, a.ConfigPath)
	for repoPath, srcPath := range files {
		if filesAreDifferent(srcPath, filepath.Join(repoDir, repoPath)) {
			report.Modified = append(report.Modified, repoPath)
		}
	}
	sort.Strings(report.Modified)
	return report, nil
}

// CloudStatusReport is the result of `cpm cloud status`.
type CloudStatusReport struct {
	Initialized bool          `json:"initialized"`
	Repo        string        `json:"repo"`
	Remote      string        `json:"remote,omitempty"`
	Recent      []CloudCommit `json:"recent"`
	// Modified lists synced files changed locally since the last push.
	Modified []string `json:"modified"`
}

// CloudCommit is one recent sync in the cloud repo.
type CloudCommit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// RenderCloudStatus prints the human-readable cloud status.
func (a *App) RenderCloudStatus(r CloudStatusReport) {
	if !r.Initialized {
		a.outln("Cloud sync not initialized.")
		a.outln("Run 'cpm cloud init' to get started.")
		return
	}
	a.outf("Cloud repo: %s\n", r.Repo)
	if r.Remote != "" {
		a.outf("Remote:     %s\n", r.Remote)
	} else {
		a.outln("Remote:     (none)")
	}
	if len(r.Recent) > 0 {
		a.outf("\nRecent syncs:\n")
		for _, c := range r.Recent {
			a.outf("  %s %s\n", c.Hash, c.Subject)
		}
	}
	a.outln("\nLocal changes (not yet pushed):")
	for _, path := range r.Modified {
		a.outf("  modified: %s\n", path)
	}
	if len(r.Modified) == 0 {
		a.outln("  (no changes)")
	}
}

// CloudRemote sets or updates the remote URL.
func (a *App) CloudRemote(url string) error {
	repoDir := CloudRepoDir(a.ConfigPath)
	if err := ensureCloudRepo(repoDir); err != nil {
		return err
	}

	if a.hasOriginRemote(repoDir) {
		if _, err := a.git(repoDir, "remote", "set-url", "origin", url); err != nil {
			return err
		}
	} else {
		if _, err := a.git(repoDir, "remote", "add", "origin", url); err != nil {
			return err
		}
	}

	if err := saveCloudRemote(a.expand(a.ConfigPath), url); err != nil {
		return fmt.Errorf("saving remote to config: %w", err)
	}

	a.outf("Remote set to: %s\n", url)
	return nil
}

// GatherSyncFiles returns a map of repoPath -> absoluteSourcePath for all syncable files.
func (a *App) GatherSyncFiles(cfg *Config, configPath string) (map[string]string, error) {
	home := a.Home
	files := make(map[string]string)

	// Files from source_dir
	for _, name := range syncFilesFor(cfg) {
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
		cfgPath := a.expand(configPath)
		if _, err := os.Stat(cfgPath); err == nil {
			files["cpm/config.toml"] = cfgPath
		}
	}
	if !isExcluded(tombstonesRepoPath, cfg) && fileExistsAt(a.tombstonesPath()) {
		files[tombstonesRepoPath] = a.tombstonesPath()
	}

	for path, src := range files {
		if privateSyncFile(src, configPath) {
			delete(files, path)
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
	Kind       string
	StatePath  string
	RemoteHash string
	LocalHash  string
}

// PlanDistribute compares the cloud repo with the live files and returns
// what applying it would do. Deletions are only planned inside the synced
// directories (commands/, agents/, ...), never for top-level files.
func (a *App) PlanDistribute(repoDir string, cfg *Config, configPath string) []DistributeAction {
	home := a.Home
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

	for _, name := range syncFilesFor(cfg) {
		add(name, filepath.Join(cfg.SourceDir, name), isExcluded(name, cfg))
	}

	for _, dirname := range cloudSyncDirs {
		repoSubDir := filepath.Join(repoDir, dirname)
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
			plan = append(plan, DistributeAction{RepoPath: "cpm/config.toml", Source: src, Dest: a.expand(configPath), Kind: "merge"})
		}
		if src := filepath.Join(repoDir, filepath.FromSlash(tombstonesRepoPath)); fileExistsAt(src) && !isExcluded(tombstonesRepoPath, cfg) {
			plan = append(plan, DistributeAction{RepoPath: tombstonesRepoPath, Source: src, Dest: a.tombstonesPath(), Kind: "merge"})
		}
	}

	sort.SliceStable(plan, func(i, j int) bool { return plan[i].RepoPath < plan[j].RepoPath })
	return protectDistribute(plan, repoDir)
}

// ApplyDistribute executes a plan.
func (a *App) ApplyDistribute(plan []DistributeAction) error {
	if err := a.prepareDistribute(plan); err != nil {
		return err
	}
	var failures []error
	for _, act := range plan {
		var err error
		switch act.Kind {
		case "create", "update":
			err = copyFile(act.Source, act.Dest)
		case "delete":
			err = os.Remove(act.Dest)
			if os.IsNotExist(err) {
				err = nil
			}
		case "merge":
			if act.RepoPath == tombstonesRepoPath {
				err = mergeTombstones(act.Source, act.Dest)
			} else {
				err = a.mergeConfigTOML(act.Source, act.Dest)
			}
		default:
			continue
		}
		if err != nil {
			failure := fmt.Errorf("%s %s: %w", act.Kind, act.RepoPath, err)
			failures = append(failures, failure)
			a.outf("  failed: %v\n", failure)
		} else {
			a.outf("  completed %s: %s\n", act.Kind, act.RepoPath)
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	return saveDistributeState(plan)
}

// DistributeSyncFiles applies the repo to the live locations (no config merge).
func (a *App) DistributeSyncFiles(repoDir string, cfg *Config) error {
	return a.ApplyDistribute(a.PlanDistribute(repoDir, cfg, ""))
}

// --- helpers ---

// git runs git in dir ("" means the current directory) and returns its
// combined output.
func (a *App) git(dir string, args ...string) (string, error) {
	out, err := a.Run.Run(dir, "git", args...)
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

func (a *App) hasOriginRemote(repoDir string) bool {
	_, err := a.git(repoDir, "remote", "get-url", "origin")
	return err == nil
}

// checkRemoteHasRefs reports whether the remote already has branches. A
// remote that cannot be reached is an error, not "empty".
func (a *App) checkRemoteHasRefs(remote string) (bool, error) {
	out, err := a.git("", "ls-remote", "--heads", remote)
	if err != nil {
		return false, fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return strings.TrimSpace(out) != "", nil
}

func (a *App) currentBranch(repoDir string) string {
	out, err := a.git(repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(out)
}

// printPlan prints the actions that change something and reports whether
// there were any.
func (a *App) printPlan(plan []DistributeAction) bool {
	any := false
	for _, act := range plan {
		switch act.Kind {
		case "create", "update", "delete", "merge", "conflict":
			a.outf("  would %s: %s\n", act.Kind, act.RepoPath)
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

func cleanDeletedFiles(repoSubDir, srcDir string) error {
	return filepath.Walk(repoSubDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(repoSubDir, path)
		srcFile := filepath.Join(srcDir, rel)
		if _, err := os.Stat(srcFile); os.IsNotExist(err) {
			return os.Remove(path)
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

// mergeConfigTOML adds what the pulled config has and the local one lacks:
// profiles, the [cloud] table, the indicator setting and [[match]] rules.
// source_dir, bin_dir and everything already set locally are left untouched.
func (a *App) mergeConfigTOML(pulledPath, localConfigPath string) error {
	localPath := a.expand(localConfigPath)

	pulledData, err := os.ReadFile(pulledPath)
	if err != nil {
		return err
	}
	var pulled Config
	pulledMeta, err := toml.Decode(string(pulledData), &pulled)
	if err != nil {
		return err
	}
	if pulled.Version > ConfigVersion {
		return fmt.Errorf("the cloud config has version %d, which needs a newer cpm (this one supports %d); run cpm upgrade on this machine", pulled.Version, ConfigVersion)
	}

	localData, err := os.ReadFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return copyFile(pulledPath, localPath)
		}
		return err
	}
	var local Config
	localMeta, err := toml.Decode(string(localData), &local)
	if err != nil {
		return err
	}

	// Decide what is missing before touching anything. The new name of a
	// pending rename is left out: the rename creates it.
	renameTargets := a.pendingRenameTargets(filepath.Dir(filepath.Dir(pulledPath)), local.Profiles)
	var missing []string
	for _, name := range SortedProfileNames(&pulled) {
		if _, exists := local.Profiles[name]; !exists && !renameTargets[name] {
			missing = append(missing, name)
		}
	}
	addCloud := pulled.Cloud != nil && local.Cloud == nil
	addIndicator := pulledMeta.IsDefined("indicator") && !localMeta.IsDefined("indicator")
	// Rules go after the local ones, so local rules keep priority.
	var rules []MatchRule
	for _, r := range pulled.Match {
		if !slices.Contains(local.Match, r) && !slices.Contains(rules, r) {
			rules = append(rules, r)
		}
	}
	if len(missing) == 0 && !addCloud && !addIndicator && len(rules) == 0 {
		return nil
	}

	out := localData
	if addIndicator {
		out = SetRootKey(out, "indicator", strconv.FormatBool(pulled.Indicator))
	}
	for _, name := range missing {
		out, err = AppendProfileTable(out, name, pulled.Profiles[name])
		if err != nil {
			return err
		}
		a.outf("  added profile from cloud: %s\n", name)
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
	if len(rules) > 0 {
		if out, err = AppendMatchRules(out, rules); err != nil {
			return err
		}
		for _, r := range rules {
			a.outf("  added match rule from cloud: %s -> %s\n", r.describe(), r.Profile)
		}
	}
	return writeConfigAtomic(localPath, out)
}
