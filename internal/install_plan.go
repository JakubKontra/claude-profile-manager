package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// InstallAction describes a filesystem change shared by preview and execution.
type InstallAction struct {
	Kind   string
	Path   string
	Data   []byte
	Before []byte
	Target string
	Mode   os.FileMode
}

type installPlanner struct{ actions []InstallAction }

func (p *installPlanner) write(path string, data []byte, mode os.FileMode) error {
	st, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("refusing to overwrite non-regular file %s", path)
	}
	before, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && bytes.Equal(before, data) && st.Mode().Perm() == mode {
		return nil
	}
	p.actions = append(p.actions, InstallAction{Kind: "write", Path: path, Data: data, Before: before, Mode: mode})
	return nil
}

// PlanInstall only reads local files. Cloud operations belong to the caller.
func (a *App) PlanInstall(cfg *Config, opts InstallOptions) ([]InstallAction, error) {
	if opts.Sync && !opts.Force {
		if changes := CheckDivergence(cfg, a.BaseDir); len(changes) > 0 {
			for _, d := range changes {
				a.outf("  %s\n", d.Details)
			}
			return nil, ErrDiverged
		}
	}
	p := &installPlanner{}
	active := map[string]bool{}
	for _, name := range SortedProfileNames(cfg) {
		profile := cfg.Profiles[name]
		dir := filepath.Join(a.BaseDir, name)
		if st, err := os.Lstat(dir); err == nil && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {
			return nil, fmt.Errorf("invalid profile directory %s", dir)
		}
		if err := p.write(filepath.Join(dir, ".cpm", "profile"), []byte(name+"\n"), 0o600); err != nil {
			return nil, err
		}
		settings := filepath.Join(dir, "settings.json")
		for _, file := range copyFiles {
			dest := filepath.Join(dir, file)
			current, err := os.ReadFile(dest)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			data := current
			src, srcErr := os.ReadFile(filepath.Join(cfg.SourceDir, file))
			if srcErr != nil && !os.IsNotExist(srcErr) {
				return nil, srcErr
			}
			if srcErr == nil && (os.IsNotExist(err) || opts.Sync) {
				data = src
				if err := p.write(baselinePath(dir, file), src, 0o600); err != nil {
					return nil, err
				}
			}
			if data == nil {
				continue
			}
			if dest == settings {
				data, _ = applySettingsToSource(data, EffectiveSettingsOverrides(profile))
			}
			if err := p.write(dest, data, 0o600); err != nil {
				return nil, err
			}
		}
		if err := p.sharedDirs(dir, cfg.SourceDir, EffectiveShareDirs(cfg, profile)); err != nil {
			return nil, err
		}
		if err := p.claudeJSON(dir, profile); err != nil {
			return nil, err
		}
		wrapperPath := filepath.Join(cfg.BinDir, wrapperPrefix+name)
		active[wrapperPrefix+name] = true
		if err := checkWrapperTarget(wrapperPath); err != nil {
			return nil, err
		}
		wrapper := GenerateWrapper(name, dir, profile)
		if err := p.write(wrapperPath, []byte(wrapper), wrapperMode(wrapper)); err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(cfg.BinDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(cfg.BinDir, entry.Name())
		if !active[entry.Name()] && entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), wrapperPrefix) && isGeneratedWrapper(path) {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			p.actions = append(p.actions, InstallAction{Kind: "remove", Path: path, Before: data})
		}
	}
	return p.actions, nil
}

func (p *installPlanner) sharedDirs(dir, source string, shared []string) error {
	wanted := map[string]bool{}
	for _, name := range shared {
		wanted[name] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if wanted[entry.Name()] || entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		absSource, _ := filepath.Abs(source)
		if strings.HasPrefix(resolveLinkTarget(path, target), absSource+string(os.PathSeparator)) {
			p.actions = append(p.actions, InstallAction{Kind: "isolate", Path: path, Target: target})
		}
	}
	for _, name := range shared {
		src, dst := filepath.Join(source, name), filepath.Join(dir, name)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if target, err := os.Readlink(dst); err == nil {
			absSrc, _ := filepath.Abs(src)
			if resolveLinkTarget(dst, target) == absSrc {
				continue
			}
		} else if _, err := os.Lstat(dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		absolute, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		p.actions = append(p.actions, InstallAction{Kind: "symlink", Path: dst, Target: absolute})
	}
	return nil
}

func (p *installPlanner) claudeJSON(dir string, profile *Profile) error {
	var global map[string]any
	if data, err := os.ReadFile(DefaultClaudeJSONPath()); err == nil {
		if err := json.Unmarshal(data, &global); err != nil {
			return fmt.Errorf("invalid global .claude.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	path := filepath.Join(dir, ".claude.json")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	current := map[string]any{}
	if err == nil {
		if err := json.Unmarshal(data, &current); err != nil {
			return fmt.Errorf("invalid %s: %w", path, err)
		}
		if current == nil {
			current = map[string]any{}
		}
	} else {
		for _, key := range claudeJSONSeedKeys {
			if v, ok := global[key]; ok {
				current[key] = v
			}
		}
	}
	servers, _ := global["mcpServers"].(map[string]any)
	if servers != nil || len(profile.MCPServers) > 0 || !inheritsMCP(profile) {
		current["mcpServers"] = MergeMCPServers(servers, profile)
	}
	if len(current) == 0 && data == nil {
		return nil
	}
	encoded, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	return p.write(path, append(encoded, '\n'), 0o600)
}

func (a *App) ApplyInstall(plan []InstallAction) error {
	for _, act := range plan {
		var err error
		switch act.Kind {
		case "write":
			err = writeFileAtomic(act.Path, act.Data, act.Mode)
		case "remove":
			err = os.Remove(act.Path)
		case "isolate":
			if err = os.Remove(act.Path); err == nil {
				err = os.MkdirAll(act.Path, 0o755)
			}
		case "symlink":
			if err = os.MkdirAll(filepath.Dir(act.Path), 0o755); err == nil {
				if err = os.Remove(act.Path); err == nil || os.IsNotExist(err) {
					err = os.Symlink(act.Target, act.Path)
				}
			}
		default:
			return fmt.Errorf("unknown install action %q", act.Kind)
		}
		if err != nil {
			return fmt.Errorf("%s %s: %w", act.Kind, act.Path, err)
		}
		a.outf("  %s %s\n", act.Kind, act.Path)
	}
	return nil
}

func (a *App) RenderInstallPlan(plan []InstallAction, cfg *Config) {
	a.outln("Dry run — local changes (cloud auto-pull/auto-push skipped):")
	if len(plan) == 0 {
		a.outln("  (no changes)")
	}
	for _, act := range plan {
		a.outf("  %s %s", act.Kind, act.Path)
		if act.Kind == "symlink" {
			a.outf(" -> %s", act.Target)
		}
		a.outln()
		if act.Kind != "write" && act.Kind != "remove" {
			continue
		}
		before, after := redactPreview(act.Before, cfg), redactPreview(act.Data, cfg)
		if before == after {
			continue
		}
		a.outf("--- %s\n+++ %s (planned)\n", act.Path, act.Path)
		// A whole-file unified hunk keeps previews deterministic without act diff dependency.
		oldLines, newLines := strings.Split(strings.TrimSuffix(before, "\n"), "\n"), strings.Split(strings.TrimSuffix(after, "\n"), "\n")
		if before == "" {
			oldLines = nil
		}
		if after == "" {
			newLines = nil
		}
		oldStart, newStart := 1, 1
		if len(oldLines) == 0 {
			oldStart = 0
		}
		if len(newLines) == 0 {
			newStart = 0
		}
		a.outf("@@ -%d,%d +%d,%d @@\n", oldStart, len(oldLines), newStart, len(newLines))
		for _, line := range oldLines {
			a.outf("-%s\n", line)
		}
		for _, line := range newLines {
			a.outf("+%s\n", line)
		}
	}
}

func redactPreview(data []byte, cfg *Config) string {
	text := string(data)
	var values []string
	for _, profile := range cfg.Profiles {
		for _, env := range []map[string]string{profile.Env, profile.LocalEnv} {
			for _, v := range env {
				if v != "" {
					values = append(values, v)
				}
			}
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, value := range values {
		text = strings.ReplaceAll(text, ShellQuote(value), "'[REDACTED]'")
		escaped, _ := json.Marshal(value)
		text = strings.ReplaceAll(text, string(escaped), `"[REDACTED]"`)
		text = strings.ReplaceAll(text, value, "[REDACTED]")
	}
	var object any
	if json.Unmarshal([]byte(text), &object) == nil {
		redactObject(object)
		encoded, err := json.MarshalIndent(object, "", "  ")
		if err == nil {
			return string(encoded) + "\n"
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "export ") || strings.Contains(strings.ToLower(line), "authorization") || strings.Contains(strings.ToLower(line), "password") || strings.Contains(line, "sk-ant-") {
			lines[i] = "[REDACTED]"
		}
	}
	return strings.Join(lines, "\n")
}

func redactObject(value any) {
	switch v := value.(type) {
	case map[string]any:
		for k, item := range v {
			lower := strings.ToLower(k)
			if lower == "env" || lower == "headers" || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "apikey") || secretKeyPattern.MatchString(k) {
				v[k] = "[REDACTED]"
			} else if s, ok := item.(string); ok && secretValuePattern.MatchString(s) {
				v[k] = "[REDACTED]"
			} else {
				redactObject(item)
			}
		}
	case []any:
		for _, item := range v {
			redactObject(item)
		}
	}
}
