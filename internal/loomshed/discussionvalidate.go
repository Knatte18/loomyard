// discussionvalidate.go implements the Discussion-Validate producer: a mechanical gate over
// `_lyx/discussion/`'s two files, exhaustively defined by the two checks below and nothing beyond
// them.

package loomshed

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// requiredDiscussionSections are the seven H2 headings discussionValidate requires
// decisionRecordPath to carry, per contracts/stencils/loom/loom-template-discussion.md's Step 5.
var requiredDiscussionSections = []string{
	"## Goal",
	"## Scope",
	"## Decisions",
	"## Constraints",
	"## Auto-mode assumptions",
	"## Open risks",
	"## Acceptance criteria",
}

// discussionValidate is the Discussion-Validate producer: it checks that decisionRecordPath and
// supportLogPath both exist, and that decisionRecordPath carries every section in
// requiredDiscussionSections.
//
// Both paths are told rather than derived, because loomengine's own accessors for them
// (DiscussionDecisionRecord/DiscussionSupportLog) take a *lyxcwd.Location, which this package may
// not import.
type discussionValidate struct {
	name               string
	decisionRecordPath string
	supportLogPath     string
}

var _ shedengine.ShedProducer = (*discussionValidate)(nil)

// newDiscussionValidate returns a discussionValidate identified as name, checking decisionRecordPath
// and supportLogPath.
func newDiscussionValidate(name, decisionRecordPath, supportLogPath string) *discussionValidate {
	return &discussionValidate{name: name, decisionRecordPath: decisionRecordPath, supportLogPath: supportLogPath}
}

// Call implements shedengine.ShedProducer. It runs exactly two checks, and nothing beyond them is
// this producer's to look for: first, both files exist; second, the decision record contains all
// seven required H2 sections. Any failure maps to shedengine.Stuck with an empty pointer; both
// checks passing maps to shedengine.Done, reporting the decision record's path as the pointer.
//
// A read failure that is not a not-exist is a returned error, not Stuck -- a Stuck would bounce
// back to Discussion-Write, which cannot fix an I/O fault.
//
// Deliberately NOT checks, each for a stated reason: "## Notes for the plan writer" is optional by
// contract and its absence is never a violation; section *order* is pinned in the stencil but is
// not validated here; an extra unexpected H2 is not a violation either.
func (p *discussionValidate) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if _, err := os.Stat(p.supportLogPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return p.nonDoneExit(ctx, "", err)
		}
		return p.nonDoneExit(ctx, shedengine.Stuck, nil)
	}

	data, err := os.ReadFile(p.decisionRecordPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return p.nonDoneExit(ctx, "", err)
		}
		return p.nonDoneExit(ctx, shedengine.Stuck, nil)
	}

	if !hasAllSections(string(data), requiredDiscussionSections) {
		return p.nonDoneExit(ctx, shedengine.Stuck, nil)
	}

	return shedengine.Done, shedengine.OutputPointer{Path: p.decisionRecordPath}, nil
}

// nonDoneExit consults cancelErr before returning a non-Done result: a cancelled context replaces
// outcome/err with the cancellation error; otherwise it returns outcome (with an empty pointer) and
// err verbatim.
func (p *discussionValidate) nonDoneExit(ctx context.Context, outcome shedengine.Outcome, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return outcome, shedengine.OutputPointer{}, nil
}

// hasAllSections reports whether every heading in required appears as its own line in content,
// after trimming trailing whitespace -- so a heading nested inside a fenced block or appearing
// mid-sentence never counts.
func hasAllSections(content string, required []string) bool {
	want := make(map[string]bool, len(required))
	for _, h := range required {
		want[h] = true
	}
	found := make(map[string]bool, len(required))

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t\r")
		if want[line] {
			found[line] = true
		}
	}

	for _, h := range required {
		if !found[h] {
			return false
		}
	}
	return true
}
