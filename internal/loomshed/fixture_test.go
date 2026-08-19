// fixture_test.go implements buildSequenceFixture, the one shared Tier-1 builder card 11 (see
// _mill/plan/03-sequence-and-integration.md) requires: a whole temp anchor a real New-built
// producer list can run against offline, plus the Deps pointing at it. Every sequence test in this
// batch -- this file's own TestSequence_FullRunReachesDone and resume_test.go's suite -- reuses it
// rather than building its own fixture.

package loomshed

import (
	"os"
	"path/filepath"
	"testing"
)

// buildSequenceFixture builds a temp anchor whose on-disk state makes rows 3 (Discussion-Validate),
// 7 (Plan-Validate), and 9 (Batchifier) -- the three real, non-injectable producers this task builds
// -- genuinely pass, and returns the anchor path alongside the Deps pointing at it.
//
// Discussion-Validate: both discussion files are written, the decision record carrying all seven
// required H2 sections (writeDiscussionFixture, from discussionvalidate_test.go).
// Plan-Validate: a syntactically complete, approved, one-card plan directory that satisfies every
// planparser.Validate check, including the ones that stat paths against the worktree root
// (seedPlanValidateFixture, from planvalidate_test.go) -- the same self-authored, single-card,
// zero-findings shape internal/planparser/testdata/goodplan/00-overview.md and 01-json-flag.md
// model.
// Batchifier: no batcher.yaml is written at all, so batcher.Active resolves the embedded template,
// which is a Done.
//
// The status file is seeded through the production Seed, never by hand-writing JSON, so a Seed
// regression would not pass unnoticed here. Preflight and WebsterRun are the only two injectable
// rows (the explicit-deps-struct Shared Decision): Preflight is fakeAlwaysDoneProducer (from
// loomshed_test.go) and WebsterRun is fakeWebsterRun's run method (from webster_test.go), reporting
// Webster's own done outcome. LockPath and StatusLockPath are given two distinct paths, since
// shedengine rejects them naming one file.
func buildSequenceFixture(t *testing.T) (anchorPath string, deps Deps) {
	t.Helper()

	dir := t.TempDir()

	discussionDir := filepath.Join(dir, "discussion")
	if err := os.MkdirAll(discussionDir, 0o755); err != nil {
		t.Fatalf("mkdir discussion dir: %v", err)
	}
	decisionRecordPath, supportLogPath := writeDiscussionFixture(t, discussionDir, validDecisionRecord, "support log")

	seedPlanValidateFixture(t, dir, true)

	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	if err := Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}

	deps = Deps{
		StatusPath:         statusPath,
		LockPath:           filepath.Join(dir, "run.lock"),
		StatusLockPath:     statusLockPath,
		MaxBounces:         3,
		AnchorPath:         dir,
		WorktreeRoot:       dir,
		DecisionRecordPath: decisionRecordPath,
		SupportLogPath:     supportLogPath,
		Preflight:          fakeAlwaysDoneProducer{},
		WebsterRun:         (&fakeWebsterRun{}).run,
	}
	return dir, deps
}
