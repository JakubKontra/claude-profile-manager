package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var updateSurface = flag.Bool("update", false, "rewrite testdata/cli-surface.txt")

// TestCLISurface pins every command, argument pattern and flag. From 1.0 on,
// commands and flags may be added but not removed or renamed; run
// `go test ./cmd -update` after an intended change.
func TestCLISurface(t *testing.T) {
	var b strings.Builder
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Name() == "help" {
			return // cobra's built-in
		}
		line := c.UseLine()
		if c.Hidden {
			line += "  (hidden)"
		}
		fmt.Fprintln(&b, line)
		var flags []string
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "help" {
				return
			}
			s := "    --" + f.Name
			if f.Shorthand != "" {
				s = "    -" + f.Shorthand + ", --" + f.Name
			}
			s += " " + f.Value.Type()
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" && f.Name != "config" {
				s += " = " + f.DefValue
			}
			flags = append(flags, s)
		})
		sort.Strings(flags)
		for _, f := range flags {
			fmt.Fprintln(&b, f)
		}
		subs := c.Commands()
		sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
		for _, sub := range subs {
			walk(sub)
		}
	}
	root := NewRoot()
	root.InitDefaultCompletionCmd()
	walk(root)

	path := filepath.Join("testdata", "cli-surface.txt")
	if *updateSurface {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./cmd -update)", err)
	}
	if b.String() != string(want) {
		t.Errorf("CLI surface changed (run go test ./cmd -update if intended):\n--- want\n%s\n--- got\n%s", want, b.String())
	}
}
