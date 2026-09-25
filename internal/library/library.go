// Package library parses the pipe-delimited library files that define
// runnable command entries (e.g. default/libraries/connect_library).
//
// File format:
//
//	name|command line 1 \
//	  continuation line 2 \
//	  continuation line 3
//
//	next_name|single line command
//
// Entries are separated by one or more blank lines. A command may span
// multiple physical lines via a trailing " \" (backslash) line
// continuation. Placeholders look like {{NAME}}.
package library

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// placeholderRe matches {{NAME}} tokens.
var placeholderRe = regexp.MustCompile(`\{\{([A-Za-z0-9_]+)\}\}`)

// Entry is a single named command from a library file.
type Entry struct {
	Name string

	// RawCommand preserves the command exactly as written in the source
	// file, including any backslash line-continuations and original
	// indentation. This is what the preview area renders.
	RawCommand string

	// FlatCommand is RawCommand with line-continuations joined into a
	// single physical line, suitable for passing to `sh -c`.
	FlatCommand string

	// Placeholders lists the {{NAME}} tokens referenced by the command,
	// in order of first appearance, deduplicated.
	Placeholders []string
}

// HasPlaceholder reports whether the entry's command references the given
// placeholder name.
func (e Entry) HasPlaceholder(name string) bool {
	for _, p := range e.Placeholders {
		if p == name {
			return true
		}
	}
	return false
}

// Library is one library file (one nav tab).
type Library struct {
	// Key is the filename as it appears on disk (e.g. "connect_library").
	Key string
	// Label is Key with a trailing "_library" suffix stripped, used for
	// the tab title (e.g. "connect").
	Label string
	// Path is the absolute path to the source file.
	Path string
	// Entries are in file order (as parsed).
	Entries []Entry
}

// FindEntry returns the entry with the given name, if present.
func (l Library) FindEntry(name string) (Entry, bool) {
	for _, e := range l.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// LabelFor derives a tab label from a library filename.
func LabelFor(filename string) string {
	return strings.TrimSuffix(filename, "_library")
}

// ParseFile parses a single library file.
func ParseFile(path string) (Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Library{}, err
	}
	filename := filepath.Base(path)
	lib := Library{
		Key:   filename,
		Label: LabelFor(filename),
		Path:  path,
	}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, block := range splitBlocks(text) {
		entry, ok := parseBlock(block)
		if ok {
			lib.Entries = append(lib.Entries, entry)
		}
	}
	return lib, nil
}

// LoadDir parses every file directly inside dir as a library, returning the
// libraries sorted alphabetically by filename.
func LoadDir(dir string) ([]Library, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		names = append(names, f.Name())
	}
	sort.Strings(names)

	libs := make([]Library, 0, len(names))
	for _, name := range names {
		lib, err := ParseFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		libs = append(libs, lib)
	}
	return libs, nil
}

// splitBlocks groups the physical lines of text into blocks, where blocks
// are separated by one or more blank (whitespace-only) lines.
func splitBlocks(text string) [][]string {
	var blocks [][]string
	var current []string
	for _, ln := range strings.Split(text, "\n") {
		if strings.TrimSpace(ln) == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, ln)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

// parseBlock turns one block of physical lines into an Entry. The first
// line must contain "name|command...". Returns ok=false for malformed
// blocks (no "|" on the first line).
func parseBlock(lines []string) (Entry, bool) {
	if len(lines) == 0 {
		return Entry{}, false
	}
	first := lines[0]
	idx := strings.Index(first, "|")
	if idx < 0 {
		return Entry{}, false
	}
	name := strings.TrimSpace(first[:idx])
	if name == "" {
		return Entry{}, false
	}

	cmdLines := make([]string, 0, len(lines))
	cmdLines = append(cmdLines, first[idx+1:])
	cmdLines = append(cmdLines, lines[1:]...)

	raw := strings.Join(cmdLines, "\n")
	flat := flatten(cmdLines)

	return Entry{
		Name:         name,
		RawCommand:   raw,
		FlatCommand:  flat,
		Placeholders: extractPlaceholders(raw),
	}, true
}

// flatten joins continuation lines into a single shell command line. Each
// non-final line has its trailing "\" continuation marker stripped (the
// rest of the line, including any trailing space before the backslash, is
// preserved) and lines are concatenated directly, mirroring how a POSIX
// shell treats a backslash-newline sequence.
func flatten(lines []string) string {
	var b strings.Builder
	for i, ln := range lines {
		if i < len(lines)-1 {
			b.WriteString(strings.TrimSuffix(ln, "\\"))
		} else {
			b.WriteString(ln)
		}
	}
	return b.String()
}

// extractPlaceholders returns the ordered, deduplicated list of {{NAME}}
// tokens referenced in s.
func extractPlaceholders(s string) []string {
	matches := placeholderRe.FindAllStringSubmatch(s, -1)
	if matches == nil {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// Substitute replaces every {{NAME}} token in cmd using vals (missing
// names are substituted with an empty string). Used for building the final
// command to execute, once validation has already guaranteed every
// referenced name is filled.
func Substitute(cmd string, vals map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(cmd, func(tok string) string {
		name := tok[2 : len(tok)-2]
		return vals[name]
	})
}

// SubstitutePreview is like Substitute but leaves a token as the literal
// {{NAME}} wherever vals has no non-empty value for it, so the live preview
// still shows the shape of the command instead of blanking out unfilled
// spots. Used only for display, never for the command that actually runs.
func SubstitutePreview(cmd string, vals map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(cmd, func(tok string) string {
		name := tok[2 : len(tok)-2]
		if v, ok := vals[name]; ok && v != "" {
			return v
		}
		return tok
	})
}
