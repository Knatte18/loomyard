package loomshed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// validDecisionRecord carries all seven required sections, in order, plus the optional eighth.
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
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
	})

	t.Run("SupportLogMissing", func(t *testing.T) {
		dir := t.TempDir()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, validDecisionRecord, "")

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
	})

	t.Run("EachRequiredSectionMissing", func(t *testing.T) {
		for _, missing := range requiredDiscussionSections {
			t.Run(missing, func(t *testing.T) {
				dir := t.TempDir()
				var lines []string
				for _, line := range strings.Split(validDecisionRecord, "\n") {
					if line == missing {
						continue
					}
					lines = append(lines, line)
				}
				decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, strings.Join(lines, "\n"), "support log")

				p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
				outcome, _, err := p.Call(context.Background())
				if err != nil {
					t.Fatalf("Call() error = %v; want nil", err)
				}
				if outcome != shedengine.Stuck {
					t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
				}
			})
		}
	})

	t.Run("NotesForPlanWriterAbsentStillPasses", func(t *testing.T) {
		dir := t.TempDir()
		withoutNotes := strings.Split(validDecisionRecord, "## Notes for the plan writer")[0]
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, withoutNotes, "support log")

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
	})

	t.Run("SectionsOutOfOrderStillPasses", func(t *testing.T) {
		dir := t.TempDir()
		reordered := `# Decision record

## Scope

Scope text.

## Goal

Goal text.

## Acceptance criteria

Criteria text.

## Decisions

Decisions text.

## Constraints

Constraints text.

## Auto-mode assumptions

Assumptions text.

## Open risks

Risks text.
`
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, reordered, "support log")

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
	})

	t.Run("ExtraUnexpectedH2StillPasses", func(t *testing.T) {
		dir := t.TempDir()
		withExtra := validDecisionRecord + "\n## Unexpected Extra Section\n\nExtra text.\n"
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, withExtra, "support log")

		p := NewDiscussionValidate("Discussion-Validate", decisionRecordPath, supportLogPath)
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
	})

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
