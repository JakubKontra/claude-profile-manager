package internal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RunInit(configPath string) error {
	configPath = ExpandPath(configPath)

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config already exists at %s", configPath)
	}

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create directory: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)

	out("Welcome to cpm (Claude Profile Manager) setup!\n\n")

	// Source dir
	out("Source directory [~/.claude]: ")
	sourceDir, _ := reader.ReadString('\n')
	sourceDir = strings.TrimSpace(sourceDir)
	if sourceDir == "" {
		sourceDir = "~/.claude"
	}

	// Bin dir
	out("Bin directory [~/.local/bin]: ")
	binDir, _ := reader.ReadString('\n')
	binDir = strings.TrimSpace(binDir)
	if binDir == "" {
		binDir = "~/.local/bin"
	}

	// Profiles
	var profiles []profileEntry
	out("\nLet's add your profiles. Enter an empty name to finish.\n\n")

	for i := 1; ; i++ {
		outf("Profile %d name (e.g. personal, work): ", i)
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)
		if name == "" {
			break
		}

		outf("  Description: ")
		desc, _ := reader.ReadString('\n')
		desc = strings.TrimSpace(desc)

		outf("  Default model (leave empty for none): ")
		model, _ := reader.ReadString('\n')
		model = strings.TrimSpace(model)

		profiles = append(profiles, profileEntry{name, desc, model})
		outln()
	}

	if len(profiles) == 0 {
		return fmt.Errorf("no profiles defined, aborting")
	}

	// Generate TOML
	var b strings.Builder
	fmt.Fprintf(&b, "source_dir = %q\n", sourceDir)
	fmt.Fprintf(&b, "bin_dir = %q\n", binDir)

	for _, p := range profiles {
		fmt.Fprintf(&b, "\n[profiles.%s]\n", p.name)
		fmt.Fprintf(&b, "description = %q\n", p.desc)
		if p.model != "" {
			fmt.Fprintf(&b, "model = %q\n", p.model)
		}
	}

	if err := os.WriteFile(configPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}

	outf("\nConfig written to %s\n", configPath)
	outln("Run 'cpm install' to create profiles and wrapper scripts.")

	return nil
}

type profileEntry struct {
	name  string
	desc  string
	model string
}
