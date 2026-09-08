// validate.go implements Validate, the Discussion-Validate gate's complete machine check set:
// both `_lyx/discussion/` files exist, and the decision record carries every required H2 section.
// Its control flow reproduces internal/loomshed/discussionvalidate.go's discussionValidate.Call
// step for step, per the short-circuit-order-is-load-bearing Shared Decision, so extracting it into
// this stdlib-only leaf changes nothing about when a Stuck outcome bounces back to Discussion-Write
// versus when an I/O fault aborts the run.

package discussionparser

import (
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
//
// It splits the string it was already handed rather than running a bufio.Scanner over it, and that
// is a correctness choice, not a style one. Scanner stops at the first line longer than
// bufio.MaxScanTokenSize (64 KB) and reports that only through scanner.Err(), which this function
// never checked -- so one pasted base64 blob or minified snippet, entirely ordinary in a discussion
// document an agent wrote, made every heading BELOW it report missing. loomshed's
// Discussion-Validate row maps those findings to Stuck, bounces to Discussion-Write, respawns, and
// repeats on the same document until the bounce budget escalates to a human, over a document that
// was valid all along (crucible round opus-medium-r6, R6-28). content is already fully in memory,
// so there is no line length to cap and no error left to drop.
func missingSections(content string, required []string) []string {
	found := make(map[string]bool, len(required))

	for _, raw := range strings.Split(content, "\n") {
		found[strings.TrimRight(raw, " \t\r")] = true
	}

	var missing []string
	for _, h := range required {
		if !found[h] {
			missing = append(missing, h)
		}
	}
	return missing
}
