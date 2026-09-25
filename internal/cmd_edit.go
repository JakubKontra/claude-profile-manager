package internal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// EditorCommand resolves the editor from $VISUAL, then $EDITOR, then vi.
// The value may contain arguments ("code --wait").
func EditorCommand(getenv func(string) string) []string {
	for _, key := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return strings.Fields(v)
		}
	}
	return []string{"vi"}
}

// EditConfig opens the config in the user's editor and validates it after.
// run is injected for tests.
func EditConfig(configPath string, getenv func(string) string, run func(*exec.Cmd) error) error {
	path := ExpandPath(configPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config not found at %s (run 'cpm init' first)", path)
	}

	editor := EditorCommand(getenv)
	cmd := exec.Command(editor[0], append(editor[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := run(cmd); err != nil {
		return fmt.Errorf("editor %s failed: %w", editor[0], err)
	}

	if _, err := LoadConfig(path); err != nil {
		outf("Warning: %v\n", err)
		return nil
	}
	outln("Config is valid. Run 'cpm install' to apply changes.")
	return nil
}
