// scan.go implements the walk and the per-line classification codestats reports on:
// which files are visited, which of them are tests, and how each line divides into
// code, comment, and blank.

package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileStats is one scanned file's tally.
// Lines is always Code + Comment + Blank: a line carrying both code and a trailing
// comment counts as code, so the three buckets partition the file rather than
// overlapping, and a whitespace-only line inside a block comment counts as blank.
type FileStats struct {
	Path    string `json:"path"`
	Ext     string `json:"ext"`
	IsTest  bool   `json:"is_test"`
	Lines   int    `json:"lines"`
	Code    int    `json:"code"`
	Comment int    `json:"comment"`
	Blank   int    `json:"blank"`
}

// skipDirs are directory names never descended into, whatever -hidden says.
// Build output and vendored trees are not the scanned repo's own code, and .git is not
// source at all -- counting any of them would make every figure depend on whether the
// tree happened to be built before the scan.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"bin":          true,
	"obj":          true,
}

// commentFamily is how a language spells its comments, which is what decides whether a
// line can be classified as comment at all.
type commentFamily int

const (
	// familyNone is every extension with no comment syntax this tool reads -- markdown,
	// JSON, plain text. Their non-blank lines all count as code, which is why the
	// report gives them lines rather than a comment share.
	familyNone commentFamily = iota
	// familyC is the // and /* */ family: Go, C#, and their relatives.
	familyC
	// familyHash is the # family: Python, shell, YAML, TOML.
	familyHash
)

// familyFor maps an extension onto its comment family.
// The list is deliberately explicit rather than a catch-all default: an unrecognised
// extension reporting zero comments is honest, while guessing a family for it would
// quietly invent a comment share.
func familyFor(ext string) commentFamily {
	switch ext {
	case ".go", ".cs", ".java", ".js", ".jsx", ".ts", ".tsx",
		".c", ".h", ".cc", ".cpp", ".hpp", ".rs", ".swift", ".kt", ".scala":
		return familyC
	case ".py", ".sh", ".bash", ".zsh", ".yaml", ".yml", ".toml", ".rb", ".pl", ".r":
		return familyHash
	}
	return familyNone
}

// cScanner classifies C-family lines.
// It is a type rather than a function because two states survive a line break: an open
// /* block comment, and Go's backtick raw string. C#'s @"" verbatim string is NOT
// tracked across line breaks -- see the command doc comment for why that limit is
// accepted.
type cScanner struct {
	inBlock bool
	inRaw   bool
}

// classify reports whether line carries code and whether it carries comment.
// Both can be true, which is a trailing comment; the caller resolves the overlap.
func (s *cScanner) classify(line string) (code, comment bool) {
	for i := 0; i < len(line); {
		switch {
		case s.inBlock:
			j := strings.Index(line[i:], "*/")
			if j < 0 {
				return code, true
			}
			comment = true
			s.inBlock = false
			i += j + 2
		case s.inRaw:
			j := strings.IndexByte(line[i:], '`')
			if j < 0 {
				return true, comment
			}
			code = true
			s.inRaw = false
			i += j + 1
		case strings.HasPrefix(line[i:], "//"):
			return code, true
		case strings.HasPrefix(line[i:], "/*"):
			s.inBlock = true
			comment = true
			i += 2
		case line[i] == '`':
			s.inRaw = true
			code = true
			i++
		case line[i] == '"' || line[i] == '\'':
			code = true
			i = skipQuoted(line, i)
		case line[i] == ' ' || line[i] == '\t':
			i++
		default:
			code = true
			i++
		}
	}
	return code, comment
}

// skipQuoted returns the index just past the quoted literal opening at i, honouring
// backslash escapes.
// An unterminated literal consumes the rest of the line: that is a line continuation or
// a malformed file, and neither adds a comment to the line.
func skipQuoted(line string, i int) int {
	quote := line[i]
	for i++; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if line[i] == quote {
			return i + 1
		}
	}
	return len(line)
}

// hashClassify classifies a #-family line.
// It is deliberately line-local and string-unaware: a # inside a quoted string is read
// as a comment start, which overcounts slightly in shell and YAML. Tracking quoting
// across those languages' very different rules would cost more than it buys a
// statistics tool, and the overcount is bounded by how rarely a # appears in a string.
func hashClassify(line string) (code, comment bool) {
	if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
		return false, true
	}
	if strings.IndexByte(line, '#') >= 0 {
		return true, true
	}
	return true, false
}

// isTestPath reports whether rel is a test file, by each ecosystem's own convention:
// Go pins it in the file name, while C# and the rest put tests in a directory or a file
// whose name says so.
func isTestPath(rel, ext string) bool {
	base := filepath.Base(rel)
	if ext == ".go" {
		return strings.HasSuffix(base, "_test.go")
	}
	lower := strings.ToLower(base)
	stem := strings.TrimSuffix(lower, ext)
	for _, marker := range []string{"test", "tests", "spec", "specs"} {
		if stem == marker || strings.HasSuffix(stem, marker) {
			return true
		}
	}
	for _, seg := range strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/") {
		switch s := strings.ToLower(seg); {
		case s == "test", s == "tests", s == "spec", s == "specs":
			return true
		case strings.HasSuffix(s, ".tests"), strings.HasSuffix(s, "_test"), strings.HasSuffix(s, "-tests"):
			return true
		}
	}
	return false
}

// splitLines splits data into lines, dropping the empty element a trailing newline would
// otherwise produce and normalising CRLF, so a file checked out with Windows line
// endings does not report a stray \r on every line.
func splitLines(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// looksBinary reports whether data holds a NUL byte in its first 8 KiB -- the cheap
// check a file must pass before its "lines" mean anything.
func looksBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0
}

// classifyFile tallies one file, given its path relative to the scan root and its bytes.
func classifyFile(rel string, data []byte) FileStats {
	ext := strings.ToLower(filepath.Ext(rel))
	stats := FileStats{Path: rel, Ext: ext, IsTest: isTestPath(rel, ext)}
	family := familyFor(ext)

	var scanner cScanner
	for _, line := range splitLines(data) {
		stats.Lines++
		if strings.TrimSpace(line) == "" {
			stats.Blank++
			continue
		}
		code, comment := true, false
		switch family {
		case familyC:
			code, comment = scanner.classify(line)
		case familyHash:
			code, comment = hashClassify(line)
		}
		switch {
		case code:
			stats.Code++
		case comment:
			stats.Comment++
		default:
			stats.Blank++
		}
	}
	return stats
}

// Scan walks root and returns one FileStats per readable text file.
// It skips the names in skipDirs, every dotted file and directory unless hidden is set,
// anything matching excludes, and any file that looks binary.
func Scan(root string, hidden bool, excludes []string) ([]FileStats, error) {
	var out []FileStats
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := entry.Name()
		hiddenName := strings.HasPrefix(name, ".")

		if entry.IsDir() {
			if skipDirs[name] || (!hidden && hiddenName) || matchesAny(excludes, rel, name) {
				return filepath.SkipDir
			}
			return nil
		}
		if (!hidden && hiddenName) || matchesAny(excludes, rel, name) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if looksBinary(data) {
			return nil
		}
		out = append(out, classifyFile(rel, data))
		return nil
	})
	return out, err
}

// matchesAny reports whether any pattern matches the path relative to the scan root, its
// base name alone, or a directory prefix of it -- so -exclude takes 'docs/generated',
// '*.pb.go', and 'crucible' alike.
func matchesAny(patterns []string, rel, name string) bool {
	for _, pattern := range patterns {
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
		if prefix := strings.TrimSuffix(pattern, "/"); strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return false
}
