// description.go validates the change-description artifact.
// ValidateDescription turns the failures of Parse, plus a few structural checks, into Findings.
// One check covers the Merge danger section, which calls whether a revert undoes the change and what breaks if wrong.
// Another is the forbidden-term check, which refuses a line naming a term of the tooling that wrote the change.

package summaryparser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxTitleLen is the longest permitted description title, in characters.
const maxTitleLen = 72

// Finding check names, asserted exactly by callers and tests.
const (
	CheckMissing                = "missing-file"
	CheckEmpty                  = "empty-file"
	CheckNoHeading              = "no-heading"
	CheckEmptyTitle             = "empty-title"
	CheckTitleTooLong           = "title-too-long"
	CheckEmptyBody              = "empty-body"
	CheckCoAuthorTrail          = "co-authored-by-trailer"
	CheckNoMergeDanger          = "no-merge-danger"
	CheckMergeDangerDoor        = "merge-danger-door"
	CheckMergeDangerBlastRadius = "merge-danger-blast-radius"
	CheckForbiddenTerm          = "forbidden-term"
)

// The Merge danger section's heading, its two labelled lines and the door's two legal values.
const (
	MergeDangerHeading     = "## Merge danger"
	MergeDangerDoorLabel   = "**Door:**"
	MergeDangerRadiusLabel = "**Blast radius:**"
	DoorOneWay             = "one-way"
	DoorTwoWay             = "two-way"
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
// These conditions are reported as Findings:
// a missing or empty file, a bad heading, an empty or over-long title, an empty body, or a Co-Authored-By trailer line.
// A non-empty body must also hold a "## Merge danger" section.
// That section needs a "**Door:**" line reading one-way or two-way.
// It also needs a "**Blast radius:**" line of one word.
// Each line of the file that one of forbidden matches is one finding naming the line's number and the matched text;
// a nil or empty forbidden checks no term.
// The file never holds the credit block or the regression check, which the landing appends after it.
// Only a read failure other than not-exist is an error.
func ValidateDescription(path string, forbidden []*regexp.Regexp) ([]Finding, error) {
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
	} else {
		findings = append(findings, mergeDangerFindings(s.Body)...)
	}
	for _, line := range strings.Split(s.Body, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "co-authored-by:") {
			findings = append(findings, Finding{CheckCoAuthorTrail, "body contains a Co-Authored-By trailer; the landing appends its own credit block"})
			break
		}
	}
	if len(forbidden) > 0 {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("summaryparser: read summary file %s: %w", path, err)
		}
		findings = append(findings, forbiddenTermFindings(string(data), forbidden)...)
	}
	return findings, nil
}

// forbiddenTermFindings returns one finding per line of content that a pattern of forbidden matches.
// The finding names the line's one-based number and the first pattern's matched text.
func forbiddenTermFindings(content string, forbidden []*regexp.Regexp) []Finding {
	var findings []Finding
	for i, line := range strings.Split(content, "\n") {
		for _, pattern := range forbidden {
			if match := pattern.FindString(line); match != "" {
				findings = append(findings, Finding{CheckForbiddenTerm, fmt.Sprintf("line %d names %q, a term of the tooling that wrote the change; rewrite the line in the repository's own terms, for a reader who never saw that tooling", i+1, match)})
				break
			}
		}
	}
	return findings
}

// mergeDangerFindings checks body's Merge danger section.
// The section runs from its heading to the next heading of level one or two.
// A labelled line may stand as a list item, and its value may be in backticks.
func mergeDangerFindings(body string) []Finding {
	section, found := mergeDangerSection(body)
	if !found {
		return []Finding{{CheckNoMergeDanger, fmt.Sprintf("body has no %q section; end it with one carrying a %s line and a %s line", MergeDangerHeading, MergeDangerDoorLabel, MergeDangerRadiusLabel)}}
	}

	var findings []Finding
	door, hasDoor := labelledValue(section, MergeDangerDoorLabel)
	switch {
	case !hasDoor:
		findings = append(findings, Finding{CheckMergeDangerDoor, fmt.Sprintf("the Merge danger section has no %s line", MergeDangerDoorLabel)})
	case !strings.EqualFold(door, DoorOneWay) && !strings.EqualFold(door, DoorTwoWay):
		findings = append(findings, Finding{CheckMergeDangerDoor, fmt.Sprintf("door is %q; want %s or %s", door, DoorOneWay, DoorTwoWay)})
	}
	radius, hasRadius := labelledValue(section, MergeDangerRadiusLabel)
	switch {
	case !hasRadius:
		findings = append(findings, Finding{CheckMergeDangerBlastRadius, fmt.Sprintf("the Merge danger section has no %s line", MergeDangerRadiusLabel)})
	case radius == "" || len(strings.Fields(radius)) != 1:
		findings = append(findings, Finding{CheckMergeDangerBlastRadius, fmt.Sprintf("blast radius is %q; want one word", radius)})
	}
	return findings
}

// mergeDangerSection returns the lines under body's Merge danger heading, and whether the heading is there.
// Lines inside a code fence, and the fence lines themselves, are neither the heading nor a heading that ends the section.
func mergeDangerSection(body string) ([]string, bool) {
	var section []string
	openFence, found := "", false
	for _, line := range strings.Split(body, "\n") {
		if run := fenceRun(line); run != "" {
			switch {
			case openFence == "":
				openFence = run
			case run[0] == openFence[0] && len(run) >= len(openFence) && strings.TrimSpace(line) == run:
				openFence = ""
			}
			continue
		}
		switch {
		case openFence != "":
		case !found:
			found = strings.EqualFold(strings.TrimSpace(line), MergeDangerHeading)
		case isSectionHeading(line):
			return section, true
		default:
			section = append(section, line)
		}
	}
	return section, found
}

// fenceRun returns the run of three or more backticks or tildes that opens line, or "" when line opens with no such run.
// A fence closes only on a run of the same character, at least as long as the one that opened it, with no text after it.
func fenceRun(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return ""
	}
	run := trimmed[:len(trimmed)-len(strings.TrimLeft(trimmed, trimmed[:1]))]
	if len(run) < 3 {
		return ""
	}
	return run
}

// isSectionHeading reports whether line opens a heading of level one or two.
func isSectionHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ")
}

// labelledValue returns the value after label on the first of lines that opens with it, and whether one does.
// A leading list marker is skipped, and the value is trimmed of spaces, backticks and a final period.
func labelledValue(lines []string, label string) (string, bool) {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* "))
		if !strings.HasPrefix(trimmed, label) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, label))
		value = strings.TrimSuffix(value, ".")
		return strings.TrimSpace(strings.Trim(value, "`")), true
	}
	return "", false
}
