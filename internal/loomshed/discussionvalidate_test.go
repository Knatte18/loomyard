// discussionvalidate_test.go exercises discussionValidate.Call's own mapping from
// internal/discussionparser.Validate's two return values to shedengine's outcome/pointer/error
// contract, plus this producer's cancellation discipline. The checks themselves --
// which sections are required, which files must exist, how a heading is matched -- are
// internal/discussionparser/validate_test.go's to cover; this suite would not notice if that
// package's check logic changed shape, only if this producer stopped mapping its results correctly.

package loomshed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// validDecisionRecord carries all seven required sections, in order, plus the optional eighth. It
// stays a hardcoded literal here rather than an export of internal/discussionparser: no caller
// outside that package needs the list itself now that the section-iteration case has moved there.
const validDecisionRecord = `# Decision record

## Goal

Goal text.

## Scope

Scope text.

## Decisions

Decisions text.

## Constraints

Constraints text.

## Auto-mode assumptions

Assumptions text.

## Open risks

Risks text.

## Acceptance criteria

Criteria text.

## Notes for the plan writer

Notes text.
`

func writeDiscussionFixture(t *testing.T, dir, decisionRecord, supportLog string) (decisionRecordPath, supportLogPath string) {
	t.Helper()
	decisionRecordPath = filepath.Join(dir, "decision-record.md")
	supportLogPath = filepath.Join(dir, "support-log.md")
	if decisionRecord != "" {
		if err := os.WriteFile(decisionRecordPath, []byte(decisionRecord), 0o644); err != nil {
			t.Fatalf("write decision record: %v", err)
		}
	}
	if supportLog != "" {
		if err := os.WriteFile(supportLogPath, []byte(supportLog), 0o644); err != nil {
			t.Fatalf("write support log: %v", err)
		}
	}
	return decisionRecordPath, supportLogPath
}

func TestDiscussionValidate_Call(t *testing.T) {
	t.Run("BothFilesPresentAllSections", func(t *testing.T) {
		dir := t.TempDir()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, validDecisionRecord, "support log")

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		if pointer.Path != decisionRecordPath {
			t.Errorf("Call() pointer.Path = %q; want %q", pointer.Path, decisionRecordPath)
		}
	})

	t.Run("DecisionRecordMissing", func(t *testing.T) {
		dir := t.TempDir()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, "", "support log")

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want the zero value", pointer)
		}
	})

	t.Run("SupportLogMissing", func(t *testing.T) {
		dir := t.TempDir()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, validDecisionRecord, "")

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want the zero value", pointer)
		}
	})

	t.Run("HeadingMissingMapsToStuck", func(t *testing.T) {
		dir := t.TempDir()
		withoutGoal := strings.Replace(validDecisionRecord, "## Goal\n\nGoal text.\n\n", "", 1)
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, withoutGoal, "support log")

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want the zero value", pointer)
		}
	})

	// TestDiscussionValidate_Call/DecisionRecordUnreadableReturnsErrorNotStuck pins the
	// short-circuit-order-is-load-bearing Shared Decision at the producer level: the support log
	// exists, so the decision record is read next, and a read failure that is not a not-exist (here,
	// the path is a directory) must return a non-nil error rather than any verdict -- an accumulating
	// rewrite that instead treated this as a finding would silently flip this case to Stuck.
	t.Run("DecisionRecordUnreadableReturnsErrorNotStuck", func(t *testing.T) {
		dir := t.TempDir()
		_, supportLogPath := writeDiscussionFixture(t, dir, "", "support log")
		decisionRecordPath := filepath.Join(dir, "decision-record.md")
		if err := os.MkdirAll(decisionRecordPath, 0o755); err != nil {
			t.Fatalf("mkdir decision record: %v", err)
		}

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, _, err := p.Call(context.Background())
		if err == nil {
			t.Fatalf("Call() error = nil; want a non-nil error (decision record path is a directory)")
		}
		if outcome == shedengine.Done || outcome == shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want no verdict alongside a returned error", outcome)
		}
	})

	// TestDiscussionValidate_Call/CancelledContextReturnsErrorNotVerdict is the suite's only
	// cancellation case. entryErr and nonDoneExit's cancelErr consult the identical ctx.Err() value,
	// and entryErr is Call's very first statement, so a synchronous test that pre-cancels its context
	// is always caught at entry and can never reach nonDoneExit's own check -- the two are not
	// independently reachable by fixture choice, and a second pre-cancelled case would be
	// mechanically identical to this one.
	t.Run("CancelledContextReturnsErrorNotVerdict", func(t *testing.T) {
		dir := t.TempDir()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, validDecisionRecord, "support log")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, _, err := p.Call(ctx)
		if err == nil {
			t.Fatalf("Call(cancelled) error = nil; want non-nil error")
		}
		if outcome == shedengine.Done || outcome == shedengine.Stuck {
			t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
		}
	})
}
