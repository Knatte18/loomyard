// claudeimports.go implements Expand, the expansion of a Claude instruction file's `@` imports.

package claudeimports

import (
	"os"
	"path/filepath"
	"strings"
)

// maxImportHops is the deepest import chain Claude Code follows below the file named by the caller.
const maxImportHops = 5

// Expanded is a Claude instruction file with its imports expanded.
type Expanded struct {
	// Text is the file's text with each import replaced by the expanded text of the file it names.
	Text string
	// Files lists the absolute, cleaned path of every file an import resolved to, in the order first met, each once.
	Files []string
}

// Expand reads the file at path and expands its `@` imports.
// A relative import resolves against the directory of the importing file and `~/` against the user's home directory.
// An import stays as written when it sits inside an inline code span or a fenced code block.
// It also stays as written when its target is not a readable regular file.
// It also stays as written when it lies past the fifth hop or names a file already on the current import chain.
// It returns the read error of path itself.
func Expand(path string) (Expanded, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Expanded{}, err
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return Expanded{}, err
	}
	expander := &expander{seen: map[string]bool{}}
	text := expander.expandText(string(data), absolute, []string{absolute})
	return Expanded{Text: text, Files: expander.files}, nil
}

// expander carries the files met so far across one Expand call.
type expander struct {
	files []string
	seen  map[string]bool
}

// expandText expands the imports of text, the content of the file at filePath.
// chain holds the files from the root down to filePath; its length minus one is filePath's hop depth.
func (e *expander) expandText(text, filePath string, chain []string) string {
	var out strings.Builder
	var fence string
	for _, line := range strings.SplitAfter(text, "\n") {
		marker := fenceMarker(line)
		switch {
		case fence != "":
			out.WriteString(line)
			if marker != "" && marker[0] == fence[0] && len(marker) >= len(fence) {
				fence = ""
			}
		case marker != "":
			fence = marker
			out.WriteString(line)
		default:
			out.WriteString(e.expandLine(line, filePath, chain))
		}
	}
	return out.String()
}

// fenceMarker returns the run of three or more backticks or tildes that opens line after at most three spaces, or "".
func fenceMarker(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || trimmed == "" {
		return ""
	}
	fenceChar := trimmed[0]
	if fenceChar != '`' && fenceChar != '~' {
		return ""
	}
	run := 0
	for run < len(trimmed) && trimmed[run] == fenceChar {
		run++
	}
	if run < 3 {
		return ""
	}
	return trimmed[:run]
}

// expandLine expands the imports of one line outside a fenced block, leaving inline code spans untouched.
func (e *expander) expandLine(line, filePath string, chain []string) string {
	var out strings.Builder
	for i := 0; i < len(line); {
		switch {
		case line[i] == '`':
			end := codeSpanEnd(line, i)
			out.WriteString(line[i:end])
			i = end
		case line[i] == '@' && (i == 0 || isSpace(line[i-1])):
			end := i + 1
			for end < len(line) && !isSpace(line[end]) {
				end++
			}
			out.WriteString(e.importText(line[i:end], filePath, chain))
			i = end
		default:
			out.WriteByte(line[i])
			i++
		}
	}
	return out.String()
}

// codeSpanEnd returns the index just past the inline code span opening at start.
// A backtick run with no closing run of the same length is literal, so only the run itself is skipped.
func codeSpanEnd(line string, start int) int {
	runEnd := start
	for runEnd < len(line) && line[runEnd] == '`' {
		runEnd++
	}
	opening := runEnd - start
	for i := runEnd; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		closeEnd := i
		for closeEnd < len(line) && line[closeEnd] == '`' {
			closeEnd++
		}
		if closeEnd-i == opening {
			return closeEnd
		}
		i = closeEnd
	}
	return runEnd
}

// importText returns the expanded text of the file the token (an `@` and its path) names.
// It returns the token itself when it names no readable regular file, passes the last hop or closes an import cycle.
func (e *expander) importText(token, filePath string, chain []string) string {
	target, ok := resolveImport(token[1:], filepath.Dir(filePath))
	if !ok || len(chain)-1 >= maxImportHops {
		return token
	}
	for _, onChain := range chain {
		if onChain == target {
			return token
		}
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return token
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return token
	}
	if !e.seen[target] {
		e.seen[target] = true
		e.files = append(e.files, target)
	}
	return e.expandText(string(data), target, append(chain[:len(chain):len(chain)], target))
}

// resolveImport returns the absolute, cleaned path an import's path names, relative to importerDir.
func resolveImport(importPath, importerDir string) (string, bool) {
	switch {
	case importPath == "":
		return "", false
	case strings.HasPrefix(importPath, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		return filepath.Join(home, importPath[2:]), true
	case filepath.IsAbs(importPath):
		return filepath.Clean(importPath), true
	default:
		return filepath.Join(importerDir, importPath), true
	}
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
