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

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeMergeShuttle is a minimal mergeresolve.Shuttle fake that satisfies NewPublish's and
// NewFinalize's non-nil Deps.Shuttle requirement. It is never actually invoked by this package's
// fixtures: buildSequenceFixture's landing passthrough makes Publish's own told-skip gate report
// Stuck before Publish (or Finalize, which never runs at all -- see buildSequenceFixture's own doc
// comment) ever reaches its resolver.
type fakeMergeShuttle struct{}

var _ mergeresolve.Shuttle = fakeMergeShuttle{}

// Run is never called by this package's fixtures; see fakeMergeShuttle's own doc comment.
func (fakeMergeShuttle) Run(shuttleengine.Spec) (shuttleengine.Result, error) {
	return shuttleengine.Result{}, nil
}

// nilFabricOpener is a landingshed pair-opener closure fake shared by every Deps builder in this
// package. It returns a typed-nil *fabricengine.Fabric and a nil error, which legally satisfies
// NewPublish/NewFinalize -- both construct their resolver from the interface value the fabric
// handle is stored behind, and a nil check on an interface holding a typed-nil pointer still
// passes -- without ever dereferencing the handle at construction time.
func nilFabricOpener() (*fabricengine.Fabric, error) {
	return nil, nil
}

// testLandingDeps returns a landingshed.Deps with every field the two producer constructors
// require filled with a synthetic-but-valid told value, so New(deps) never fails construction on
// this package's own tests. dir is used for the told absolute paths landingshed.Deps carries.
func testLandingDeps(dir string) landingshed.Deps {
	return landingshed.Deps{
		WorktreeRoot:     dir,
		TaskBranch:       "task-branch",
		ParentBranch:     "fixture-parent",
		WebsterDir:       dir,
		StencilsDir:      dir,
		ScratchDir:       filepath.Join(dir, "landing-scratch"),
		OriginURL:        "https://example.invalid/fixture/fixture.git",
		PushBranch:       func() error { return nil },
		OpenFabric:       nilFabricOpener,
		OpenParentFabric: nilFabricOpener,
		Shuttle:          fakeMergeShuttle{},
	}
}

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
//
// Rows 12 (Publish) and 13 (Finalize) are the real producers as of this task, and this fixture
// deliberately never drives either to a genuine merge: Deps.Landing.Config.RequirePRToBase names
// the same parent branch Seed above records, and PushSkipped is true, so Publish's own told-skip
// gate reports Stuck -- with OnStuck: "", which blocks the whole run right there -- before Publish
// ever reaches its resolver and long before Finalize's Call is ever invoked. Driving either
// producer's own merge logic for real needs a genuine two-worktree pair and therefore git, which
// this task's own decision keeps out of this package's untagged tier (see this batch's own Batch
// Scope); the real thing is covered by card 35's integration tier instead.
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

	landing := testLandingDeps(dir)
	landing.PushSkipped = true
	landing.Config.RequirePRToBase = []string{landing.ParentBranch}

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
		Landing:            landing,
	}
	return dir, deps
}
