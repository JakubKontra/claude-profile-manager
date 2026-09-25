package internal

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// This file edits config.toml at line granularity so the user's comments and
// formatting survive. New content is rendered with the TOML encoder so
// escaping is always correct.

// tomlTable is the line range [Start, End) of a table and everything under
// it up to the next unrelated header.
type tomlTable struct {
	Name  string
	Start int
	End   int
}

// tableHeader parses a `[a.b]` header line. Comment lines, array tables and
// anything else return ok=false. Quoted segments are unquoted so
// [profiles."work"] and [profiles.work] compare equal.
func tableHeader(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "[[") {
		return "", false
	}
	end := strings.Index(trimmed, "]")
	if end < 0 {
		return "", false
	}
	rest := strings.TrimSpace(trimmed[end+1:])
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return "", false
	}
	inner := strings.TrimSpace(trimmed[1:end])
	if inner == "" {
		return "", false
	}

	var segments []string
	for _, seg := range splitDotted(inner) {
		seg = strings.TrimSpace(seg)
		if len(seg) >= 2 && (seg[0] == '"' && seg[len(seg)-1] == '"' || seg[0] == '\'' && seg[len(seg)-1] == '\'') {
			seg = seg[1 : len(seg)-1]
		}
		if seg == "" {
			return "", false
		}
		segments = append(segments, seg)
	}
	return strings.Join(segments, "."), true
}

// splitDotted splits a dotted key on '.' outside quotes.
func splitDotted(s string) []string {
	var parts []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			cur.WriteByte(c)
		case c == '.':
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	parts = append(parts, cur.String())
	return parts
}

// findTable locates a table and its subtables. The range runs from the header
// to the next header that is neither name nor name.<something>.
func findTable(lines []string, name string) (tomlTable, bool) {
	start := -1
	for i, line := range lines {
		header, ok := tableHeader(line)
		if !ok {
			continue
		}
		if start < 0 {
			if header == name {
				start = i
			}
			continue
		}
		if header != name && !strings.HasPrefix(header, name+".") {
			return tomlTable{Name: name, Start: start, End: i}, true
		}
	}
	if start < 0 {
		return tomlTable{}, false
	}
	return tomlTable{Name: name, Start: start, End: len(lines)}, true
}

// quoteTOMLString renders s as a TOML basic string.
func quoteTOMLString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// renderProfileTOML renders a single [profiles.<name>] table (with its
// subtables) using the encoder.
func renderProfileTOML(name string, p *Profile) (string, error) {
	doc := struct {
		Profiles map[string]*Profile `toml:"profiles"`
	}{Profiles: map[string]*Profile{name: p}}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(doc); err != nil {
		return "", fmt.Errorf("cannot render profile %q: %w", name, err)
	}
	// The encoder emits the parent "[profiles]" header; the file already has
	// it implicitly, and repeating it would be a duplicate table.
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if strings.TrimSpace(line) == "[profiles]" {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// splitLines splits on '\n' keeping track of whether the input ended with one.
func splitLines(data []byte) []string {
	return strings.Split(string(data), "\n")
}

func joinLines(lines []string) []byte {
	return []byte(strings.Join(lines, "\n"))
}

// ensureTrailingNewline makes appending safe.
func ensureTrailingNewline(data []byte) []byte {
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		return append(data, '\n')
	}
	return data
}

// AppendProfileTable adds [profiles.<name>] to the config text. It fails when
// the profile already exists in any form.
func AppendProfileTable(data []byte, name string, p *Profile) ([]byte, error) {
	if err := ValidateProfileName(name); err != nil {
		return nil, err
	}
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config: %w", err)
	}
	if _, exists := cfg.Profiles[name]; exists {
		return nil, fmt.Errorf("profile %q already exists", name)
	}

	rendered, err := renderProfileTOML(name, p)
	if err != nil {
		return nil, err
	}

	out := ensureTrailingNewline(data)
	if len(out) > 0 {
		out = append(out, '\n')
	}
	return append(out, rendered...), nil
}

// RemoveProfileTable deletes [profiles.<name>] and every [profiles.<name>.*]
// table, including comment lines inside those blocks. It reports whether
// anything was removed and fails when the profile is still present
// afterwards (for example when it was defined with dotted keys or an inline
// table, which this editor does not rewrite).
func RemoveProfileTable(data []byte, name string) ([]byte, bool, error) {
	lines := splitLines(data)
	tableName := "profiles." + name
	removed := false

	for {
		table, ok := findTable(lines, tableName)
		if !ok {
			// Subtables may also live on their own after other tables.
			table, ok = findSubtable(lines, tableName)
			if !ok {
				break
			}
		}
		lines = append(lines[:table.Start], lines[table.End:]...)
		removed = true
	}

	if !removed {
		var cfg Config
		if _, err := toml.Decode(string(data), &cfg); err == nil {
			if _, defined := cfg.Profiles[name]; defined {
				return nil, false, fmt.Errorf("profile %q is defined in a form this command cannot edit (inline table or dotted keys); edit the config manually", name)
			}
		}
		return data, false, nil
	}

	out := collapseBlankLines(lines)
	var cfg Config
	if _, err := toml.Decode(string(out), &cfg); err != nil {
		return nil, false, fmt.Errorf("config would be invalid after removal: %w", err)
	}
	if _, still := cfg.Profiles[name]; still {
		return nil, false, fmt.Errorf("profile %q is defined in a form this command cannot edit (inline table or dotted keys); edit the config manually", name)
	}
	return out, true, nil
}

// findSubtable finds a `[name.something]` header that was not covered by
// findTable because it appears after an unrelated table.
func findSubtable(lines []string, name string) (tomlTable, bool) {
	for i, line := range lines {
		header, ok := tableHeader(line)
		if !ok || !strings.HasPrefix(header, name+".") {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if h, ok := tableHeader(lines[j]); ok && h != name && !strings.HasPrefix(h, name+".") {
				end = j
				break
			}
		}
		return tomlTable{Name: header, Start: i, End: end}, true
	}
	return tomlTable{}, false
}

// collapseBlankLines reduces runs of blank lines to one and trims the end.
func collapseBlankLines(lines []string) []byte {
	var out []string
	blank := false
	for _, l := range lines {
		isBlank := strings.TrimSpace(l) == ""
		if isBlank && blank {
			continue
		}
		blank = isBlank
		out = append(out, l)
	}
	result := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if result == "" {
		return []byte{}
	}
	return []byte(result + "\n")
}

// SetTableKey sets `key = rawValue` inside [table]: replacing an existing
// assignment, inserting after the header, or appending a new table when the
// table does not exist. rawValue must already be valid TOML (use
// quoteTOMLString for strings). Commented-out headers are ignored.
func SetTableKey(data []byte, table, key, rawValue string) ([]byte, error) {
	lines := splitLines(data)
	assignment := fmt.Sprintf("%s = %s", key, rawValue)

	tbl, ok := findTable(lines, table)
	if !ok {
		out := ensureTrailingNewline(data)
		if len(out) > 0 {
			out = append(out, '\n')
		}
		out = append(out, []byte(fmt.Sprintf("[%s]\n%s\n", table, assignment))...)
		return out, nil
	}

	// Only the table's own lines count, not its subtables.
	ownEnd := tbl.End
	for i := tbl.Start + 1; i < tbl.End; i++ {
		if _, isHeader := tableHeader(lines[i]); isHeader {
			ownEnd = i
			break
		}
	}
	for i := tbl.Start + 1; i < ownEnd; i++ {
		if k, ok := assignedKey(lines[i]); ok && k == key {
			lines[i] = assignment
			return joinLines(lines), nil
		}
	}

	inserted := append([]string{}, lines[:tbl.Start+1]...)
	inserted = append(inserted, assignment)
	inserted = append(inserted, lines[tbl.Start+1:]...)
	return joinLines(inserted), nil
}

// assignedKey returns the bare key of a `key = value` line.
func assignedKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	eq := strings.Index(trimmed, "=")
	if eq < 0 {
		return "", false
	}
	key := strings.TrimSpace(trimmed[:eq])
	key = strings.Trim(key, `"'`)
	if key == "" {
		return "", false
	}
	return key, true
}

// writeConfigAtomic validates data as a config and replaces path atomically.
func writeConfigAtomic(path string, data []byte) error {
	path = ExpandPath(path)
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return fmt.Errorf("refusing to write invalid config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if st, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmpPath, st.Mode().Perm())
	} else {
		_ = os.Chmod(tmpPath, 0o644)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
