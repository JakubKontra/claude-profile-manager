package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func cloudStatePath(repoDir string) string {
	return filepath.Join(filepath.Dir(repoDir), ".cpm", "cloud-state.json")
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func protectDistribute(plan []DistributeAction, repoDir string) []DistributeAction {
	statePath := cloudStatePath(repoDir)
	baseline := map[string]string{}
	data, err := os.ReadFile(statePath)
	if err == nil {
		err = json.Unmarshal(data, &baseline)
	}
	if err != nil && !os.IsNotExist(err) {
		return []DistributeAction{{Kind: "conflict", RepoPath: "unreadable sync baseline: " + err.Error()}}
	}
	for i := range plan {
		a := &plan[i]
		a.StatePath = statePath
		if a.Kind == "excluded" {
			continue
		}
		remote, re := fileHash(filepath.Join(repoDir, a.RepoPath))
		local, le := fileHash(a.Dest)
		a.RemoteHash, a.LocalHash = remote, local
		if re != nil || le != nil {
			a.Kind = "conflict"
			continue
		}
		if a.Kind == "merge" {
			continue
		}
		base, tracked := baseline[a.RepoPath]
		switch {
		case local == remote:
			a.Kind = "unchanged"
		case tracked && remote == base:
			a.Kind = "local"
		case !tracked && remote == "":
			a.Kind = "local"
		case !tracked && local != "":
			a.Kind = "conflict"
		case tracked && local != base:
			a.Kind = "conflict"
		case remote == "":
			a.Kind = "delete"
		case local == "":
			a.Kind = "create"
		default:
			a.Kind = "update"
		}
		if a.Kind == "create" || a.Kind == "update" || a.Kind == "delete" {
			for _, path := range []string{a.Dest, filepath.Join(repoDir, a.RepoPath)} {
				if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
					a.Kind = "conflict"
				}
			}
		}
	}
	return plan
}

// prepareDistribute checks the entire plan before touching live data and saves
// recoverable copies of all files it will overwrite or remove.
func prepareDistribute(plan []DistributeAction) error {
	var failures []error
	for _, a := range plan {
		if a.Kind == "conflict" {
			failures = append(failures, fmt.Errorf("sync conflict: %s (reconcile local and cloud files, then retry)", a.RepoPath))
		}
		if a.StatePath == "" || a.Kind == "excluded" {
			continue
		}
		hash, err := fileHash(a.Dest)
		if err != nil || hash != a.LocalHash {
			failures = append(failures, fmt.Errorf("file changed while planning: %s", a.RepoPath))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	var backupDir string
	manifest := map[string]string{}
	for _, a := range plan {
		if a.StatePath == "" || a.LocalHash == "" {
			continue
		}
		if a.Kind != "update" && a.Kind != "delete" && a.Kind != "merge" {
			continue
		}
		if backupDir == "" {
			root := filepath.Join(filepath.Dir(a.StatePath), "backups")
			if err := os.MkdirAll(root, 0o700); err != nil {
				return err
			}
			var err error
			backupDir, err = os.MkdirTemp(root, time.Now().UTC().Format("20060102T150405")+"-")
			if err != nil {
				return err
			}
		}
		data, err := os.ReadFile(a.Dest)
		if err != nil {
			return err
		}
		dest := filepath.Join(backupDir, a.RepoPath)
		if err := writeFileAtomic(dest, data, 0o600); err != nil {
			return err
		}
		manifest[a.RepoPath] = a.Dest
	}
	if backupDir != "" {
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		if err := writeFileAtomic(filepath.Join(backupDir, "restore-paths.json"), data, 0o600); err != nil {
			return err
		}
		outf("Backup: %s (restore-paths.json lists original locations)\n", backupDir)
	}
	return nil
}

func saveDistributeState(plan []DistributeAction) error {
	state := map[string]string{}
	var path string
	for _, a := range plan {
		if a.StatePath == "" {
			continue
		}
		path = a.StatePath
		if a.Kind != "excluded" && a.RemoteHash != "" {
			state[a.RepoPath] = a.RemoteHash
		}
	}
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o600)
}

func recordCloudState(repoDir string, cfg *Config, configPath string) error {
	return saveDistributeState(PlanDistribute(repoDir, cfg, configPath))
}
