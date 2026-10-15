// relevant.go implements CheckRelevant, the format check of `_lyx/discussion/relevant.md`.
// That file is the Discussion-Write lead's list of the code it read, per contracts/specs/loom-discussion-spec.md.

package discussionparser

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// checkRelevantNoEntry names a Finding produced when a present relevant list holds no entry line.
const checkRelevantNoEntry = "relevant-no-entry"

// checkRelevantStrayLine names a Finding produced by a non-blank line that is neither the optional H1 title nor a top-level bullet.
const checkRelevantStrayLine = "relevant-stray-line"

// checkRelevantPathMalformed names a Finding produced by an entry whose backticked path is missing, unclosed, empty, absolute or climbing out of the worktree.
const checkRelevantPathMalformed = "relevant-path-malformed"

// checkRelevantReasonMissing names a Finding produced by an entry with no ` — ` and reason after its path.
const checkRelevantReasonMissing = "relevant-reason-missing"

// relevantReasonSeparator separates an entry's path and members from its reason.
const relevantReasonSeparator = " — "

// CheckRelevant checks that the relevant list at relevantPath is well-formed.
// That is an optional H1 title followed by top-level bullets, each opening with one backticked worktree-relative path and ending in ` — ` and a reason.
// Member names between the path and the reason are not checked, and no named path or member is checked for existence.
//
// A missing file returns nil findings and a nil error, since the list is optional.
// Any other read error is returned with nil findings.
// A present list yields one finding per defective line, and one finding when it holds no entry.
func CheckRelevant(relevantPath string) ([]Finding, error) {
	data, err := os.ReadFile(relevantPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var findings []Finding
	addFinding := func(check string, lineNumber int, problem string) {
		findings = append(findings, Finding{
			Check:  check,
			Path:   relevantPath,
			Detail: fmt.Sprintf("relevant list line %d: %s", lineNumber, problem),
		})
	}

	entries := 0
	seenNonBlank := false
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" {
			continue
		}
		lineNumber := i + 1
		isFirstNonBlank := !seenNonBlank
		seenNonBlank = true

		if isFirstNonBlank && strings.HasPrefix(line, "# ") && strings.TrimSpace(line[2:]) != "" {
			continue
		}
		if line != "-" && !strings.HasPrefix(line, "- ") {
			addFinding(checkRelevantStrayLine, lineNumber, "expected a top-level \"- \" entry")
			continue
		}

		entries++
		if check, problem := checkRelevantEntry(strings.TrimPrefix(line, "-")); problem != "" {
			addFinding(check, lineNumber, problem)
		}
	}

	if entries == 0 {
		findings = append(findings, Finding{
			Check:  checkRelevantNoEntry,
			Path:   relevantPath,
			Detail: "relevant list holds no entry",
		})
	}
	return findings, nil
}

// checkRelevantEntry checks one entry's text, the line after its leading "-".
// It returns the first defect's check name and description, or an empty description when the entry is well-formed.
func checkRelevantEntry(text string) (check, problem string) {
	text = strings.TrimPrefix(text, " ")
	if !strings.HasPrefix(text, "`") {
		return checkRelevantPathMalformed, "entry does not open with a backticked path"
	}
	closing := strings.Index(text[1:], "`")
	if closing < 0 {
		return checkRelevantPathMalformed, "entry's backticked path is not closed"
	}
	path := text[1 : 1+closing]
	if problem := relevantPathProblem(path); problem != "" {
		return checkRelevantPathMalformed, problem
	}

	_, reason, found := strings.Cut(text[closing+2:], relevantReasonSeparator)
	if !found || strings.TrimSpace(reason) == "" {
		return checkRelevantReasonMissing, "entry has no \"" + relevantReasonSeparator + "\" followed by a reason"
	}
	return "", ""
}

// relevantPathProblem describes why path is not a worktree-relative path, or returns an empty string when it is.
// It reads the path as text on any host, treating both "/" and "\" as separators, so a verdict never depends on the OS the check runs on.
func relevantPathProblem(path string) string {
	if path == "" {
		return "entry's backticked path is empty"
	}
	if strings.ContainsAny(path[:1], `/\~`) {
		return "entry's path is not worktree-relative"
	}
	if len(path) >= 2 && path[1] == ':' && (path[0] >= 'a' && path[0] <= 'z' || path[0] >= 'A' && path[0] <= 'Z') {
		return "entry's path is not worktree-relative"
	}
	for _, segment := range strings.Split(strings.ReplaceAll(path, `\`, "/"), "/") {
		if segment == ".." {
			return "entry's path climbs out of the worktree"
		}
	}
	return ""
}
