// summary.go implements the producer-agnostic final-summary artifact's read contract: FileName and
// Path name the artifact, Parse reads and validates it with minimal fail-loud checks (presence,
// non-empty, a "# <title>" first non-blank line with a non-empty title), and CommitMessage composes
// the parsed Summary into a git-conventional subject/body commit message.

package summaryparser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileName is the final-summary artifact's fixed filename.
const FileName = "summary.md"

// Path returns the path to the final-summary artifact inside dir, a told directory.
func Path(dir string) string {
	return filepath.Join(dir, FileName)
}

// Summary is a producer's final-action prose artifact: Title from the "# <title>" heading, Body the
// remaining lines verbatim.
type Summary struct {
	Title string
	Body  string
}

// Parse reads and validates the final-summary artifact at path with minimal fail-loud validation:
// file must exist, have at least one non-blank line, and that first line must be a "# <title>"
// heading with a non-empty title.
// Every violation is its own distinct wrapped error.
func Parse(path string) (*Summary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("summaryparser: read summary file %s: %w", path, err)
	}

	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("summaryparser: summary file %s is empty", path)
	}

	// The non-empty check above guarantees at least one line is non-blank,
	// so this loop always finds a headingIdx before running out of lines.
	lines := strings.Split(string(data), "\n")
	headingIdx := 0
	for headingIdx < len(lines) && strings.TrimSpace(lines[headingIdx]) == "" {
		headingIdx++
	}

	heading := strings.TrimSpace(lines[headingIdx])
	if !strings.HasPrefix(heading, "# ") {
		return nil, fmt.Errorf("summaryparser: summary file %s: first non-blank line %q is not a %q heading", path, heading, "# <title>")
	}
	title := strings.TrimSpace(strings.TrimPrefix(heading, "# "))
	if title == "" {
		return nil, fmt.Errorf("summaryparser: summary file %s: title heading has an empty title", path)
	}

	body := strings.Join(lines[headingIdx+1:], "\n")
	return &Summary{Title: title, Body: body}, nil
}

// CommitMessage composes s into a git-conventional subject/blank-line/body commit message: the bare
// Title when Body is empty or whitespace-only, otherwise Title followed by a blank line and Body
// with its leading whitespace trimmed.
// The trim lives here rather than in Parse because Parse's Body is used verbatim elsewhere (the
// pull-request body), and a conventionally formatted artifact's Body starts with the newline after
// its heading -- joining that raw here would emit two blank lines between subject and body.
// Trailing whitespace is left untouched; git normalizes that itself.
func (s *Summary) CommitMessage() string {
	if strings.TrimSpace(s.Body) == "" {
		return s.Title
	}
	return s.Title + "\n\n" + strings.TrimLeft(s.Body, " \t\r\n")
}
