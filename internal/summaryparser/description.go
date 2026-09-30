// description.go validates the change-description artifact: ValidateDescription is the single
// function both the description gate and `lyx loom validate-description` call, turning each of
// Parse's failures, plus a few structural checks, into findings.

package summaryparser

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"unicode/utf8"
)

// maxTitleLen is the longest permitted description title, in characters.
const maxTitleLen = 72

// Finding check names, asserted exactly by callers and tests.
const (
	CheckMissing       = "missing-file"
	CheckEmpty         = "empty-file"
	CheckNoHeading     = "no-heading"
	CheckEmptyTitle    = "empty-title"
	CheckTitleTooLong  = "title-too-long"
	CheckEmptyBody     = "empty-body"
	CheckCoAuthorTrail = "co-authored-by-trailer"
)

// Finding is one failed description check.
type Finding struct {
	Check  string
	Detail string
}

// Error renders the finding as "<check>: <detail>".
func (f Finding) Error() string {
	return f.Check + ": " + f.Detail
}

// ValidateDescription checks the change description at path.
// A missing file, an empty file, a bad heading, an empty title, an over-long title, an empty body and
// a Co-Authored-By trailer line are findings; only a read failure other than not-exist is an error.
func ValidateDescription(path string) ([]Finding, error) {
	s, err := Parse(path)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return []Finding{{CheckMissing, err.Error()}}, nil
		case errors.Is(err, errEmptyFile):
			return []Finding{{CheckEmpty, err.Error()}}, nil
		case errors.Is(err, errNoHeading):
			return []Finding{{CheckNoHeading, err.Error()}}, nil
		case errors.Is(err, errEmptyTitle):
			return []Finding{{CheckEmptyTitle, err.Error()}}, nil
		}
		return nil, err
	}

	var findings []Finding
	if n := utf8.RuneCountInString(s.Title); n > maxTitleLen {
		findings = append(findings, Finding{CheckTitleTooLong, fmt.Sprintf("title is %d characters; the limit is %d", n, maxTitleLen)})
	}
	if strings.TrimSpace(s.Body) == "" {
		findings = append(findings, Finding{CheckEmptyBody, "body is empty"})
	}
	for _, line := range strings.Split(s.Body, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "co-authored-by:") {
			findings = append(findings, Finding{CheckCoAuthorTrail, "body contains a Co-Authored-By trailer; Go appends it at landing"})
			break
		}
	}
	return findings, nil
}
