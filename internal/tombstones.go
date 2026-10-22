package internal

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
)

// ProfileChange is a profile removal or rename, recorded so that other
// machines can repeat it after a cloud pull. Config merging only adds
// profiles, so without it a renamed profile would live on elsewhere.
type ProfileChange struct {
	ID   string `toml:"id"`
	Kind string `toml:"kind"` // "removed" or "renamed"
	// Name is the removed profile; From and To describe a rename.
	Name string `toml:"name,omitempty"`
	From string `toml:"from,omitempty"`
	To   string `toml:"to,omitempty"`
	At   string `toml:"at"`
	Host string `toml:"host,omitempty"`
}

func (c ProfileChange) String() string {
	what := "remove " + c.Name
	if c.Kind == "renamed" {
		what = "rename " + c.From + " -> " + c.To
	}
	if c.Host != "" {
		what += " (on " + c.Host + ")"
	}
	return what
}

const tombstonesRepoPath = "cpm/tombstones.toml"

// tombstonesPath is the local journal of profile changes, synced as
// cpm/tombstones.toml.
func (a *App) tombstonesPath() string { return filepath.Join(a.BaseDir, ".cpm", "tombstones.toml") }

// tombstonesAppliedPath lists the ids of changes this machine has made,
// applied or found not to apply, so none runs twice.
func (a *App) tombstonesAppliedPath() string {
	return filepath.Join(a.BaseDir, ".cpm", "tombstones-applied.json")
}

func readTombstones(path string) ([]ProfileChange, error) {
	var journal struct {
		Change []ProfileChange `toml:"change"`
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := toml.Decode(string(data), &journal); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return journal.Change, nil
}

func writeTombstones(path string, changes []ProfileChange) error {
	var buf bytes.Buffer
	buf.WriteString("# Profile removals and renames, replayed on other machines by cpm cloud pull.\n\n")
	if err := toml.NewEncoder(&buf).Encode(struct {
		Change []ProfileChange `toml:"change"`
	}{changes}); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes(), 0o600)
}

func (a *App) readApplied() map[string]bool {
	applied := map[string]bool{}
	data, err := os.ReadFile(a.tombstonesAppliedPath())
	if err != nil {
		return applied
	}
	var ids []string
	if json.Unmarshal(data, &ids) == nil {
		for _, id := range ids {
			applied[id] = true
		}
	}
	return applied
}

func (a *App) markApplied(ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	applied := a.readApplied()
	for _, id := range ids {
		applied[id] = true
	}
	list := make([]string, 0, len(applied))
	for id := range applied {
		list = append(list, id)
	}
	sort.Strings(list)
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(a.tombstonesAppliedPath(), append(data, '\n'), 0o600)
}

// recordChange appends c to the journal when cloud sync is configured. It
// is skipped while replaying changes from the cloud, which already have an
// id.
func (a *App) recordChange(cfg *Config, c ProfileChange) error {
	if cfg.Cloud == nil || a.replaying {
		return nil
	}
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	c.ID = hex.EncodeToString(id)
	c.At = time.Now().UTC().Format(time.RFC3339)
	c.Host, _ = os.Hostname()
	changes, err := readTombstones(a.tombstonesPath())
	if err != nil {
		return err
	}
	if err := writeTombstones(a.tombstonesPath(), append(changes, c)); err != nil {
		return err
	}
	return a.markApplied(c.ID)
}

// mergeTombstones adds the changes in the pulled journal to the local one.
func mergeTombstones(pulledPath, localPath string) error {
	pulled, err := readTombstones(pulledPath)
	if err != nil {
		return err
	}
	local, err := readTombstones(localPath)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, c := range local {
		known[c.ID] = true
	}
	merged := local
	for _, c := range pulled {
		if !known[c.ID] {
			merged = append(merged, c)
			known[c.ID] = true
		}
	}
	if len(merged) == len(local) {
		return nil
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].At < merged[j].At })
	return writeTombstones(localPath, merged)
}

// pendingChanges returns the changes in the cloud repo's journal that this
// machine has not handled yet and that apply here, in order, plus the ids
// of those that never will (for example a rename of a profile this machine
// never had). A change applies only when the pulled config agrees with it,
// so config.toml stays the source of truth.
func (a *App) pendingChanges(repoDir string, local map[string]*Profile) (pending []ProfileChange, inapplicable []string, err error) {
	changes, err := readTombstones(filepath.Join(repoDir, filepath.FromSlash(tombstonesRepoPath)))
	if err != nil || len(changes) == 0 {
		return nil, nil, err
	}
	var pulled Config
	if data, err := os.ReadFile(filepath.Join(repoDir, "cpm", "config.toml")); err == nil {
		if _, err := toml.Decode(string(data), &pulled); err != nil {
			return nil, nil, err
		}
	}
	names := map[string]bool{}
	for name := range local {
		names[name] = true
	}
	applied := a.readApplied()
	for _, c := range changes {
		if applied[c.ID] {
			continue
		}
		switch {
		case c.Kind == "renamed" && names[c.From] && !names[c.To] && pulled.Profiles[c.To] != nil && pulled.Profiles[c.From] == nil:
			pending = append(pending, c)
			delete(names, c.From)
			names[c.To] = true
		case c.Kind == "removed" && names[c.Name] && pulled.Profiles[c.Name] == nil:
			pending = append(pending, c)
			delete(names, c.Name)
		default:
			inapplicable = append(inapplicable, c.ID)
		}
	}
	return pending, inapplicable, nil
}

// pendingRenameTargets returns the new names of pending renames. Config
// merging must not add them as new profiles before the rename runs.
func (a *App) pendingRenameTargets(repoDir string, local map[string]*Profile) map[string]bool {
	targets := map[string]bool{}
	pending, _, _ := a.pendingChanges(repoDir, local)
	for _, c := range pending {
		if c.Kind == "renamed" {
			targets[c.To] = true
		}
	}
	return targets
}

// PullOptions holds the flags of `cpm cloud pull`.
type PullOptions struct {
	DryRun bool
	// Yes applies profile changes from other machines without asking.
	Yes bool
	// Stdin answers the prompt; IsTerminal says whether prompting is possible.
	Stdin      io.Reader
	IsTerminal bool
}

// applyCloudChanges lists the profile removals and renames made on other
// machines and, once confirmed, repeats them here. Removal never deletes
// profile data; it removes the wrapper and the config entry, like
// `cpm remove` without --purge.
func (a *App) applyCloudChanges(repoDir string, opts PullOptions) error {
	cfg, err := a.LoadCloudConfig()
	if err != nil {
		return err
	}
	pending, inapplicable, err := a.pendingChanges(repoDir, cfg.Profiles)
	if err != nil {
		return err
	}
	if !opts.DryRun {
		if err := a.markApplied(inapplicable...); err != nil {
			return err
		}
	}
	if len(pending) == 0 {
		return nil
	}

	a.outln("\nProfile changes from other machines:")
	for _, c := range pending {
		a.outf("  %s\n", c)
	}
	switch {
	case opts.DryRun:
		return nil
	case !opts.Yes && !opts.IsTerminal:
		a.outln("Not applied; run 'cpm cloud pull --yes' to apply them.")
		return nil
	case !opts.Yes:
		a.outf("Apply them here? [y/N] ")
		if !confirm(opts.Stdin) {
			a.outln("Not applied; they will be offered again on the next pull.")
			return nil
		}
	}

	a.replaying = true
	defer func() { a.replaying = false }()
	for _, c := range pending {
		cfg, err := a.LoadCloudConfig()
		if err != nil {
			return err
		}
		if c.Kind == "renamed" {
			_, err = a.RenameProfile(cfg, c.From, c.To, false)
		} else {
			err = a.RemoveProfile(cfg, c.Name, RemoveOptions{})
		}
		if err != nil {
			return fmt.Errorf("%s: %w", c, err)
		}
		if err := a.markApplied(c.ID); err != nil {
			return err
		}
	}
	return nil
}
