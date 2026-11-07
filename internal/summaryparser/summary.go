// summary.go implements the read contract of the producer-agnostic final-summary artifact.
// FileName and Path name the artifact.
// Parse reads it from a file and ParseData reads it from content read elsewhere.
// Both check for non-empty content and a first non-blank line that is a "# <title>" heading with a non-empty title.
// CommitMessage composes the parsed Summary into a git-conventional subject/body commit message.

package summaryparser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileName is the final-summary artifact's fixed filename.
const FileName = "summary.md"

// Path returns the path of the final-summary artifact inside dir.
func Path(dir string) string {
	return filepath.Join(dir, FileName)
}

// Sentinels for Parse's three validation failures, wrapped with %w so callers classify structurally.
var (
	errEmptyFile  = errors.New("summary file is empty")
	errNoHeading  = errors.New("first non-blank line is not a heading")
	errEmptyTitle = errors.New("title heading has an empty title")
)

// Summary is a producer's final-action prose artifact:
// Title from the "# <title>" heading, and Body the remaining lines verbatim.
type Summary struct {
	Title string
	Body  string
}

// Parse reads the final-summary artifact at path and validates its content.
// A file that cannot be read is an error too.
func Parse(path string) (*Summary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("summaryparser: read summary file %s: %w", path, err)
	}
	return ParseData(data, path)
}

// ParseData validates final-summary content with minimal fail-loud validation:
// it must have at least one non-blank line, and that first line must be a "# <title>" heading with a non-empty title.
// source names the content in an error, such as the file or the archived revision it was read from.
// Every violation is its own distinct wrapped error.
func ParseData(data []byte, source string) (*Summary, error) {
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("summaryparser: summary file %s is empty: %w", source, errEmptyFile)
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
		return nil, fmt.Errorf("summaryparser: summary file %s: first non-blank line %q is not a %q heading: %w", source, heading, "# <title>", errNoHeading)
	}
	title := strings.TrimSpace(strings.TrimPrefix(heading, "# "))
	if title == "" {
		return nil, fmt.Errorf("summaryparser: summary file %s: title heading has an empty title: %w", source, errEmptyTitle)
	}

	body := strings.Join(lines[headingIdx+1:], "\n")
	return &Summary{Title: title, Body: body}, nil
}

// CommitMessage composes s into a git-conventional commit message.
// It is the bare Title when Body is empty or whitespace-only.
// Otherwise it is Title, a blank line, and Body with its leading whitespace trimmed.
// Trimming is needed because a conventionally formatted Body starts with the newline after its heading.
// Joining it untrimmed would emit two blank lines between subject and body.
// Trailing whitespace is left untouched; git normalizes that itself.
func (s *Summary) CommitMessage() string {
	if strings.TrimSpace(s.Body) == "" {
		return s.Title
	}
	return s.Title + "\n\n" + strings.TrimLeft(s.Body, " \t\r\n")
}

// LandingMessage composes the landing commit message.
// It is the subject and body, followed by a blank line and credit verbatim when credit is non-empty.
// An empty credit leaves out the credit block.
// It never inspects the body for an existing trailer; the description gate guarantees there is none.
func (s *Summary) LandingMessage(credit string) string {
	if credit == "" {
		return s.CommitMessage()
	}
	return s.CommitMessage() + "\n\n" + credit
}
