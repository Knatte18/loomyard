// diff.go — the pure diff of two capture directories: it lists each file as a case of identified lines and reports the cells that differ.
// It reads files only, takes the compiled key patterns from its caller and reads no config.

package driftengine

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Named groups a key pattern must declare.
const (
	keyGroup   = "key"
	valueGroup = "value"
)

// CompileKeyPatterns compiles the key patterns, in order.
// A pattern that does not compile, or lacks a named group `key` or `value`, is refused naming its index and text.
// An empty list compiles to none, which makes every line unkeyed.
func CompileKeyPatterns(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for index, pattern := range patterns {
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("key pattern %d %q does not compile: %w", index, pattern, err)
		}
		for _, group := range []string{keyGroup, valueGroup} {
			if expression.SubexpIndex(group) < 0 {
				return nil, fmt.Errorf("key pattern %d %q has no named group %q", index, pattern, group)
			}
		}
		compiled = append(compiled, expression)
	}
	return compiled, nil
}

// Identity names one line of a case.
// A keyed line carries its Key and its occurrence N, counted from 1 among the file's lines holding that key, with Line 0.
// An unkeyed line carries Line, its position among the file's unkeyed lines counted from 1, with an empty Key and N 0.
type Identity struct {
	Key  string
	N    int
	Line int
}

// CellKind says how a cell differs between the two captures.
type CellKind string

// The kinds of a cell.
const (
	// CellChanged is an identity on both sides with different values.
	CellChanged CellKind = "changed"
	// CellAddedLine is an identity only the current capture holds.
	CellAddedLine CellKind = "added line"
	// CellRemovedLine is an identity only the baseline capture holds.
	CellRemovedLine CellKind = "removed line"
	// CellAddedCase is a file only the current capture holds.
	CellAddedCase CellKind = "added case"
	// CellRemovedCase is a file only the baseline capture holds.
	CellRemovedCase CellKind = "removed case"
)

// Cell is one difference between the baseline and the current capture.
// A case cell carries a zero Identity and no Old or New.
// Old is the baseline's value and New the current one; the side a line is missing from is empty.
type Cell struct {
	Case     string
	Identity Identity
	Kind     CellKind
	Old      string
	New      string
}

// ListedLine is one line of a case.
// Text is the whole line, and Value is the pattern's `value` group for a keyed line and the whole line for an unkeyed one.
type ListedLine struct {
	Identity Identity
	Text     string
	Value    string
}

// CaseListing is the lines of one case, in file order.
type CaseListing struct {
	// Case is the file's slash-separated path relative to the listed directory.
	Case  string
	Lines []ListedLine
}

// Volatile names what a hub's capture writes that differs between captures whatever the code does, such as provenance, so it is no drift.
// The zero Volatile names nothing.
type Volatile struct {
	// Files are the glob patterns of the cases left out whole: a pattern with no slash matches a case's file name at any depth, and one with a slash its whole path.
	Files []string `json:"files,omitempty"`
	// Keys are the line keys left out of every case.
	Keys []string `json:"keys,omitempty"`
}

// holdsCase reports whether name is a case the list leaves out whole.
// The patterns were checked when the config loaded, so a match cannot fail.
func (v Volatile) holdsCase(name string) bool {
	return slices.ContainsFunc(v.Files, func(pattern string) bool {
		target := name
		if !strings.Contains(pattern, "/") {
			target = path.Base(name)
		}
		matched, _ := path.Match(pattern, target)
		return matched
	})
}

// withoutCells returns cells with every cell of a volatile case or key removed.
func (v Volatile) withoutCells(cells []Cell) []Cell {
	return slices.DeleteFunc(cells, func(cell Cell) bool {
		return v.holdsCase(cell.Case) || slices.Contains(v.Keys, cell.Identity.Key)
	})
}

// withoutListed returns listings with every volatile case and every line of a volatile key removed.
func (v Volatile) withoutListed(listings []CaseListing) []CaseListing {
	kept := make([]CaseListing, 0, len(listings))
	for _, listing := range listings {
		if v.holdsCase(listing.Case) {
			continue
		}
		listing.Lines = slices.DeleteFunc(listing.Lines, func(line ListedLine) bool {
			return slices.Contains(v.Keys, line.Identity.Key)
		})
		kept = append(kept, listing)
	}
	return kept
}

// ListCaptures reads every regular file under dir, at any depth, as a case named by its slash-separated path relative to dir, sorted by name.
// It normalises CRLF to LF and drops the empty line a trailing newline leaves.
// The first pattern that matches a line with a non-empty `key` group keys it by that group, with the `value` group as its value; a line no pattern keys that way is unkeyed.
func ListCaptures(dir string, patterns []*regexp.Regexp) ([]CaseListing, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list capture files under %s: %w", dir, err)
	}
	sort.Strings(names)

	listings := make([]CaseListing, 0, len(names))
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, fmt.Errorf("read capture file %s: %w", name, err)
		}
		listings = append(listings, CaseListing{Case: name, Lines: listLines(string(content), patterns)})
	}
	return listings, nil
}

// listLines classifies the lines of one file's content.
func listLines(content string, patterns []*regexp.Regexp) []ListedLine {
	content = strings.TrimSuffix(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if content == "" {
		return nil
	}
	occurrences := map[string]int{}
	unkeyed := 0
	var lines []ListedLine
	for _, text := range strings.Split(content, "\n") {
		line := ListedLine{Text: text, Value: text}
		keyed := false
		for _, pattern := range patterns {
			match := pattern.FindStringSubmatch(text)
			if match == nil {
				continue
			}
			key := match[pattern.SubexpIndex(keyGroup)]
			if key == "" {
				continue
			}
			occurrences[key]++
			line.Identity = Identity{Key: key, N: occurrences[key]}
			line.Value = match[pattern.SubexpIndex(valueGroup)]
			keyed = true
			break
		}
		if !keyed {
			unkeyed++
			line.Identity = Identity{Line: unkeyed}
		}
		lines = append(lines, line)
	}
	return lines
}

// DiffCaptures lists both capture directories and returns their cells in case order, then in file order.
// A file only in currentDir is an added case, one only in baselineDir a removed case.
// For a case on both sides, a changed or removed line follows the baseline's file order and an added line follows them in the current file's order.
// Unkeyed identities pair by position among the unkeyed lines, so a keyed line inserted moves no unkeyed identity.
// A case named in leaveOut is skipped on both sides.
func DiffCaptures(baselineDir, currentDir string, patterns []*regexp.Regexp, leaveOut []string) ([]Cell, error) {
	baseline, err := listCasesByName(baselineDir, patterns, leaveOut)
	if err != nil {
		return nil, err
	}
	current, err := listCasesByName(currentDir, patterns, leaveOut)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(baseline)+len(current))
	for name := range baseline {
		names = append(names, name)
	}
	for name := range current {
		if _, both := baseline[name]; !both {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var cells []Cell
	for _, name := range names {
		old, inBaseline := baseline[name]
		now, inCurrent := current[name]
		switch {
		case !inBaseline:
			cells = append(cells, Cell{Case: name, Kind: CellAddedCase})
		case !inCurrent:
			cells = append(cells, Cell{Case: name, Kind: CellRemovedCase})
		default:
			cells = append(cells, diffLines(name, old.Lines, now.Lines)...)
		}
	}
	return cells, nil
}

// listCasesByName lists dir and keys the cases by name, leaving out the cases leaveOut names.
func listCasesByName(dir string, patterns []*regexp.Regexp, leaveOut []string) (map[string]CaseListing, error) {
	listings, err := ListCaptures(dir, patterns)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]CaseListing, len(listings))
	for _, listing := range listings {
		byName[listing.Case] = listing
	}
	for _, name := range leaveOut {
		delete(byName, name)
	}
	return byName, nil
}

// diffLines returns the cells of one case present on both sides.
func diffLines(name string, baseline, current []ListedLine) []Cell {
	currentByIdentity := make(map[Identity]ListedLine, len(current))
	for _, line := range current {
		currentByIdentity[line.Identity] = line
	}
	baselineIdentities := make(map[Identity]bool, len(baseline))

	var cells []Cell
	for _, line := range baseline {
		baselineIdentities[line.Identity] = true
		now, both := currentByIdentity[line.Identity]
		switch {
		case !both:
			cells = append(cells, Cell{Case: name, Identity: line.Identity, Kind: CellRemovedLine, Old: line.Value})
		case now.Value != line.Value:
			cells = append(cells, Cell{Case: name, Identity: line.Identity, Kind: CellChanged, Old: line.Value, New: now.Value})
		}
	}
	for _, line := range current {
		if !baselineIdentities[line.Identity] {
			cells = append(cells, Cell{Case: name, Identity: line.Identity, Kind: CellAddedLine, New: line.Value})
		}
	}
	return cells
}
