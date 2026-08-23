// validate.go implements Validate, the Discussion-Validate gate's complete machine check set:
// both `_lyx/discussion/` files exist, and the decision record carries every required H2 section.
// Its control flow reproduces internal/loomshed/discussionvalidate.go's discussionValidate.Call
// step for step, per the short-circuit-order-is-load-bearing Shared Decision, so extracting it into
// this stdlib-only leaf changes nothing about when a Stuck outcome bounces back to Discussion-Write
// versus when an I/O fault aborts the run.

package discussionparser

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// Finding is one defect Validate found: which check tripped, which absolute path it concerns, and
// a human-readable detail.
type Finding struct {
	Check  string
	Path   string
	Detail string
}

// Error implements the error interface. Path is not repeated here -- Detail already names the file
// or heading concerned, and Path itself carries the absolute path as a structured field for a
// future caller.
func (f Finding) Error() string {
	return f.Check + ": " + f.Detail
}

// checkFileMissing names a Finding produced when a required `_lyx/discussion/` file does not exist.
const checkFileMissing = "discussion-file-missing"

// checkSectionMissing names a Finding produced when the decision record lacks a required H2
// heading.
const checkSectionMissing = "discussion-section-missing"

// requiredDiscussionSections are the seven H2 headings Validate requires decisionRecordPath to
// carry, per contracts/stencils/loom/loom-template-discussion.md's Step 5.
var requiredDiscussionSections = []string{
	"## Goal",
	"## Scope",
	"## Decisions",
	"## Constraints",
	"## Auto-mode assumptions",
	"## Open risks",
	"## Acceptance criteria",
}

// Validate checks that decisionRecordPath and supportLogPath both exist, and that
// decisionRecordPath carries every section in requiredDiscussionSections.
//
// The checks run in a fixed order, and an error from an earlier check always wins over a finding a
// later check would have produced, because the later check never runs: first, os.Stat
// supportLogPath -- a not-exist yields exactly one Finding and a nil error, any other stat error
// returns immediately as a returned error with a nil findings slice.
// Then os.ReadFile decisionRecordPath, with the identical two-way split.
// Only then does the heading check run, which is the sole place findings accumulate -- one Finding
// per missing heading.
//
// A clean run returns a nil (or empty) slice and a nil error. Validate never returns a non-empty
// findings slice together with a non-nil error.
//
// Deliberately NOT checks, each for a stated reason: "## Notes for the plan writer" is optional by
// contract and its absence is never a violation; section *order* is pinned in the stencil but is not
// validated here; an extra unexpected H2 is not a violation either.
func Validate(decisionRecordPath, supportLogPath string) ([]Finding, error) {
	if _, err := os.Stat(supportLogPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return []Finding{{Check: checkFileMissing, Path: supportLogPath, Detail: "support log does not exist"}}, nil
	}

	data, err := os.ReadFile(decisionRecordPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return []Finding{{Check: checkFileMissing, Path: decisionRecordPath, Detail: "decision record does not exist"}}, nil
	}

	var findings []Finding
	for _, missing := range missingSections(string(data), requiredDiscussionSections) {
		findings = append(findings, Finding{
			Check:  checkSectionMissing,
			Path:   decisionRecordPath,
			Detail: "decision record is missing required section " + missing,
		})
	}

	return findings, nil
}

// missingSections returns every heading in required that does not appear as its own line in
// content, after right-trimming " \t\r" -- so a heading nested inside a fenced block or appearing
// mid-sentence never counts. The result preserves required's own order.
func missingSections(content string, required []string) []string {
	found := make(map[string]bool, len(required))

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t\r")
		found[line] = true
	}

	var missing []string
	for _, h := range required {
		if !found[h] {
			missing = append(missing, h)
		}
	}
	return missing
}
