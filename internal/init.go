package internal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) RunInit() error {
	configPath := a.expand(a.ConfigPath)

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config already exists at %s", configPath)
	}

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create directory: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)

	a.out("Welcome to cpm (Claude Profile Manager) setup!\n\n")

	// Source dir
	a.out("Source directory [~/.claude]: ")
	sourceDir, _ := reader.ReadString('\n')
	sourceDir = strings.TrimSpace(sourceDir)
	if sourceDir == "" {
		sourceDir = "~/.claude"
	}

	// Bin dir
	a.out("Bin directory [~/.local/bin]: ")
	binDir, _ := reader.ReadString('\n')
	binDir = strings.TrimSpace(binDir)
	if binDir == "" {
		binDir = "~/.local/bin"
	}

	// Profiles
	var profiles []profileEntry
	a.out("\nLet's add your profiles. Enter an empty name to finish.\n\n")

	for i := 1; ; i++ {
		a.outf("Profile %d name (e.g. personal, work): ", i)
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)
		if name == "" {
			break
		}
		if err := ValidateProfileName(name); err != nil {
			a.outf("  %v — try again\n\n", err)
			i--
			continue
		}

		a.outf("  Description: ")
		desc, _ := reader.ReadString('\n')
		desc = strings.TrimSpace(desc)

		a.outf("  Default model (leave empty for none): ")
		model, _ := reader.ReadString('\n')
		model = strings.TrimSpace(model)

		var isolation AddOptions
		if err := a.ChooseIsolation(reader, &isolation); err != nil {
			return err
		}
		profiles = append(profiles, profileEntry{name: name, desc: desc, model: model, shareProjects: *isolation.ShareProjects, inheritMCP: *isolation.InheritMCP})
		a.outln()
	}

	if len(profiles) == 0 {
		return fmt.Errorf("no profiles defined, aborting")
	}

	// Generate TOML
	var b strings.Builder
	fmt.Fprintf(&b, "version = %d\n", ConfigVersion)
	fmt.Fprintf(&b, "source_dir = %q\n", sourceDir)
	fmt.Fprintf(&b, "bin_dir = %q\n", binDir)

	for _, p := range profiles {
		fmt.Fprintf(&b, "\n[profiles.%s]\n", p.name)
		fmt.Fprintf(&b, "description = %q\n", p.desc)
		if !p.shareProjects {
			b.WriteString("isolate = [\"projects\"]\n")
		}
		fmt.Fprintf(&b, "inherit_mcp = %t\n", p.inheritMCP)
		if p.model != "" {
			fmt.Fprintf(&b, "model = %q\n", p.model)
		}
	}

	if err := os.WriteFile(configPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}

	a.outf("\nConfig written to %s\n", configPath)
	a.outln("Run 'cpm install' to create profiles and wrapper scripts.")

	return nil
}

type profileEntry struct {
	shareProjects bool
	inheritMCP    bool
	name          string
	desc          string
	model         string
}
