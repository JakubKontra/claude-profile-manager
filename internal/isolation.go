package internal

import (
	"bufio"
	"fmt"
	"strings"
)

// ChooseIsolation asks only for choices not explicitly supplied by flags.
func ChooseIsolation(reader *bufio.Reader, opts *AddOptions) error {
	choices := []struct {
		prompt string
		value  **bool
	}{
		{"Share project history with other profiles? [y/N]: ", &opts.ShareProjects},
		{"Inherit global MCP servers? [y/N]: ", &opts.InheritMCP},
	}
	for _, choice := range choices {
		if *choice.value != nil {
			continue
		}
		for {
			out(choice.prompt)
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading isolation preference: %w", err)
			}
			answer := strings.ToLower(strings.TrimSpace(line))
			if answer == "" || answer == "n" || answer == "no" {
				v := false
				*choice.value = &v
				break
			}
			if answer == "y" || answer == "yes" {
				v := true
				*choice.value = &v
				break
			}
			outln("Please answer yes or no.")
		}
	}
	return nil
}

func inheritsMCP(p *Profile) bool { return p == nil || p.InheritMCP == nil || *p.InheritMCP }
