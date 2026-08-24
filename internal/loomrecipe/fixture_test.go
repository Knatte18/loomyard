// fixture_test.go implements buildSequenceFixture, the one shared Tier-1 builder every sequence,
// resume, and bounce-routing test in this package reuses rather than building its own fixture: a
// whole temp anchor a real New-built producer list can run against offline, plus the
// shedrecipe.Env/ShedPaths pair pointing at it.
//
// The helpers this file duplicates rather than imports -- writeDiscussionFixture,
// validDecisionRecord, seedPlanValidateFixture, fakeWebsterRun, and writeBatcherConfig -- are
// deliberate duplication, not an oversight: they live in files that stay in internal/loomshed, per
// the duplicate-test-helpers-rather-than-share-them Shared Decision. testLandingDeps already existed
// in two independent copies (internal/loomshed/fixture_test.go and
// internal/shedbuild/fixture_test.go) before this task; this file is a third.

package loomrecipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeAlwaysDoneProducer is a minimal shedengine.ShedProducer fake that always reports Done -- used
// to substitute the built row 1 (Preflight) without spawning git, per the
// row1-substitution-is-a-seam-not-a-fixed-fake Shared Decision.
type fakeAlwaysDoneProducer struct{}

func (fakeAlwaysDoneProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

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

// nilFabricOpener is a landingshed pair-opener closure fake shared by every Landing builder in this
// package. It returns a typed-nil *fabricengine.Fabric and a nil error, which legally satisfies
// NewPublish/NewFinalize -- both construct their resolver from the interface value the fabric
// handle is stored behind, and a nil check on an interface holding a typed-nil pointer still
// passes -- without ever dereferencing the handle at construction time.
func nilFabricOpener() (*fabricengine.Fabric, error) {
	return nil, nil
}

// testLandingDeps returns a landingshed.Deps with every field the two producer constructors
// require filled with a synthetic-but-valid told value, so New(env, paths) never fails
// construction on this package's own tests. dir is used for the told absolute paths
// landingshed.Deps carries.
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

// fakeMasterStarter, fakeReedOps, fakeShuttleEngine, and fakeRefMatcher satisfy
// websterengine.RunDeps' four required seams by embedding the seam interface in an empty struct,
// which yields a non-nil value satisfying the interface without implementing a single method. Each
// is a placeholder for a non-nil check only: calling any promoted method panics on the embedded nil
// interface, which no test in this package does.
type (
	fakeMasterStarter struct{ websterengine.MasterStarter }
	fakeReedOps       struct{ shuttleengine.ReedOps }
	fakeShuttleEngine struct{ shuttleengine.Engine }
	fakeRefMatcher    struct{ websterengine.RefMatcher }
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

// writeDiscussionFixture writes decisionRecord and supportLog under dir, skipping either write when
// its content is empty, and returns both paths regardless.
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

// planFixtureCard is the syntactically complete, one-card plan-format card body seedPlanValidateFixture
// and fakeLoomShuttle's "plan"-role branch both write, kept as a single package-level constant so the
// two writers never drift apart. The sole card is a Create card so path-missing never fires
// regardless of worktreeRoot's contents — a Create card's targets stay exempt from on-disk existence
// checking.
const planFixtureCard = "# Card 1 — first-card\n\n**Create:**\n- `internal/firstcard/new.go`\n\n" +
	"**Intent:** placeholder card.\n"

// planFixtureOverview returns the plan-format overview body naming approved in its frontmatter,
// pointing at the sole card planFixtureCard writes. It is kept alongside planFixtureCard as a
// single package-level function so seedPlanValidateFixture and fakeLoomShuttle's "plan"-role branch
// never drift apart.
func planFixtureOverview(approved bool) string {
	return fmt.Sprintf(
		"---\nformat: 4\napproved: %t\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n",
		approved,
	)
}

// seedPlanValidateFixture writes a syntactically complete, one-card plan-format plan under
// <anchorPath>/_lyx/plan/, approved or not per approved, via planFixtureCard and
// planFixtureOverview.
func seedPlanValidateFixture(t *testing.T, anchorPath string, approved bool) {
	t.Helper()

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(planDir, "01-first-card.md"), []byte(planFixtureCard), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(planFixtureOverview(approved)), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
}

// fakeWebsterRun is a shedadapters.WebsterRunner fake recording the RunDeps it was called with and
// returning a fixed done outcome.
type fakeWebsterRun struct {
	receivedDeps []websterengine.RunDeps
}

func (f *fakeWebsterRun) run(deps websterengine.RunDeps, _ websterengine.RunOptions) (websterengine.RunResult, error) {
	f.receivedDeps = append(f.receivedDeps, deps)
	return websterengine.RunResult{Outcome: "done"}, nil
}

// fakeLoomShuttle implements shedadapters.Shuttle for row 3 (Discussion-Write) and row 6
// (Plan-Write) both: shedrecipe.Env carries one Shuttle field, not one per row, so this single fake
// serves both real LLM rows, branching on the Spec's own Role. On spec.Role == "plan" it writes the
// whole plan-directory fixture -- planFixtureCard and planFixtureOverview(true) via f.planDir --
// rather than only spec.OutputFiles, because loomshed.NewPlanWrite's rotation archives every
// top-level .md file in the plan directory (including the card file seedPlanValidateFixture
// pre-wrote) before the shuttle runs, so writing only the overview would leave the Card Index
// naming a card file that no longer exists and Plan-Validate would report Stuck and bounce.
// Otherwise (row 3's branch) it keeps the discussion behaviour: when writeOutputs is true, it
// writes both discussion output files from the received Spec.OutputFiles, creating any missing
// parent directory. Both branches always report shuttleengine.OutcomeDone. commitDiscussionCalls
// and commitPlanCalls record how many times the fixture's CommitDiscussion and CommitPlan closures
// built over this fake were invoked, respectively.
//
// buildSequenceFixture's return signature stays fixed at exactly (anchorPath string, env
// shedrecipe.Env, paths ShedPaths): seven call sites across sequence_test.go and resume_test.go
// destructure it as "_, env, paths := buildSequenceFixture(t)", and Go requires an exact arity
// match on ":=", so a fourth return value here would fail to compile at every one of those call
// sites. A test that needs this fake instead reaches it by type-asserting
// env.Shuttle.(*fakeLoomShuttle) -- buildSequenceFixture is the only thing that ever fills
// that field, so the assertion is total.
type fakeLoomShuttle struct {
	writeOutputs          bool
	runCalls              int
	commitDiscussionCalls int
	commitPlanCalls       int
	decisionRecordContent string
	supportLogContent     string
	planDir               string
}

var _ shedadapters.Shuttle = (*fakeLoomShuttle)(nil)

// Run implements shedadapters.Shuttle: on spec.Role == "plan" it rewrites the whole plan directory
// and reports Done; otherwise it records the call and, when f.writeOutputs is true, writes every
// entry of spec.OutputFiles with this fake's configured content before reporting
// shuttleengine.OutcomeDone. See fakeLoomShuttle's own doc comment for why the "plan" branch writes
// the whole directory rather than only spec.OutputFiles.
func (f *fakeLoomShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.runCalls++

	if spec.Role == "plan" {
		if err := os.MkdirAll(f.planDir, 0o755); err != nil {
			return shuttleengine.Result{}, fmt.Errorf("fakeLoomShuttle: mkdir plan dir %s: %w", f.planDir, err)
		}
		if err := os.WriteFile(filepath.Join(f.planDir, "01-first-card.md"), []byte(planFixtureCard), 0o644); err != nil {
			return shuttleengine.Result{}, fmt.Errorf("fakeLoomShuttle: write plan card file: %w", err)
		}
		if err := os.WriteFile(filepath.Join(f.planDir, "00-overview.md"), []byte(planFixtureOverview(true)), 0o644); err != nil {
			return shuttleengine.Result{}, fmt.Errorf("fakeLoomShuttle: write plan overview file: %w", err)
		}
		return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
	}

	if f.writeOutputs {
		contents := []string{f.decisionRecordContent, f.supportLogContent}
		for i, path := range spec.OutputFiles {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return shuttleengine.Result{}, fmt.Errorf("fakeLoomShuttle: mkdir %s: %w", filepath.Dir(path), err)
			}
			content := ""
			if i < len(contents) {
				content = contents[i]
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return shuttleengine.Result{}, fmt.Errorf("fakeLoomShuttle: write %s: %w", path, err)
			}
		}
	}
	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
}

// writeBatcherConfig writes content as <anchorPath>/_lyx/config/batcher.yaml.
func writeBatcherConfig(t *testing.T, anchorPath, content string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "batcher.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write batcher.yaml: %v", err)
	}
}

// buildSequenceFixture builds a temp anchor whose on-disk state makes rows 3 (Discussion-Validate),
// 7 (Plan-Validate), and 9 (Batchifier) -- the three real, non-injectable producers this task builds
// -- genuinely pass, and returns the anchor path alongside the shedrecipe.Env/ShedPaths pair
// pointing at it.
//
// Discussion-Validate: both discussion files are written, the decision record carrying all seven
// required H2 sections (writeDiscussionFixture, duplicated above from discussionvalidate_test.go).
// Plan-Validate: a syntactically complete, approved, one-card plan directory that satisfies every
// planparser.Validate check, including the ones that stat paths against the worktree root
// (seedPlanValidateFixture, duplicated above from planvalidate_test.go) -- the same self-authored,
// single-card, zero-findings shape internal/planparser/testdata/goodplan/00-overview.md and
// 01-json-flag.md model.
// Batchifier: no batcher.yaml is written at all, so batcher.Active resolves the embedded template,
// which is a Done.
//
// The status file is seeded through the production loomshed.Seed, never by hand-writing JSON, so a
// Seed regression would not pass unnoticed here. Row 1 is not injected here: New builds it from
// env.Cwd via preflightEntry, and every caller of this fixture that drives Run substitutes
// shed.Producers[0].Producer after New per the row1-substitution-is-a-seam-not-a-fixed-fake Shared
// Decision. WebsterRun is fakeWebsterRun's run method (duplicated above from webster_test.go),
// reporting Webster's own done outcome, and WebsterDeps fills the four seams websterEntry
// requireSeam-checks with the placeholder types duplicated above. LockPath and StatusLockPath are
// given two distinct paths, since shedengine rejects them naming one file.
//
// Rows 12 (Publish) and 13 (Finalize) are the real producers as of this task, and this fixture
// deliberately never drives either to a genuine merge: env.Landing.Config.RequirePRToBase names the
// same parent branch loomshed.Seed above records, and PushSkipped is true, so Publish's own
// told-skip gate reports Stuck -- with OnStuck: "", which blocks the whole run right there -- before
// Publish ever reaches its resolver and long before Finalize's Call is ever invoked. Driving either
// producer's own merge logic for real needs a genuine two-worktree pair and therefore git, which
// this task's own decision keeps out of this package's untagged tier; the real thing is covered by
// a later integration tier instead.
//
// Row 3 (Discussion-Write) is no longer skipped over by a Stub: it now runs a real
// shedadapters.SingleLLMProducer behind loomshed's commit decorator. env.Shuttle is a
// fakeLoomShuttle{writeOutputs: true}, env.DiscussionSpec is a closure returning a Spec whose
// OutputFiles is the same [decisionRecordPath, supportLogPath] pair this fixture already computes
// above, and env.CommitDiscussion is a closure recording its invocation count on that same fake.
// The fake shuttle writing both output files on every Run is what keeps Discussion-Validate
// passing here: shedadapters.archiveStaleOutputs renames the fixture's own pre-written files away
// on every Call, so without the fake rewriting them the clean sequence run would find both files
// absent.
//
// Row 6 (Plan-Write) is likewise a real shedadapters.SingleLLMProducer behind loomshed's
// rotate-and-commit decorator. The same fakeLoomShuttle serves this row too: its planDir field is
// set to the same _lyx/plan expression seedPlanValidateFixture already builds, env.PlanSpec is a
// closure returning a Spec naming Role: "plan" and OutputFiles holding the single overview path,
// and env.CommitPlan is a closure recording its invocation count on that same fake. The fake's
// "plan"-role branch rewrites the whole plan directory (see fakeLoomShuttle's own doc comment for
// why) rather than only the overview, so Plan-Validate still finds a complete, approved,
// zero-findings plan after the decorator's rotation archived the seeded one away.
func buildSequenceFixture(t *testing.T) (anchorPath string, env shedrecipe.Env, paths ShedPaths) {
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
	if err := loomshed.Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}

	landing := testLandingDeps(dir)
	landing.PushSkipped = true
	landing.Config.RequirePRToBase = []string{landing.ParentBranch}

	cwd := filepath.Join(dir, "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	planDir := filepath.Join(dir, lyxdirs.LyxDirName, "plan")

	loomShuttle := &fakeLoomShuttle{
		writeOutputs:          true,
		decisionRecordContent: validDecisionRecord,
		supportLogContent:     "support log",
		planDir:               planDir,
	}

	env = shedrecipe.Env{
		Cwd:                cwd,
		AnchorPath:         dir,
		WorktreeRoot:       dir,
		StatusPath:         statusPath,
		StatusLockPath:     statusLockPath,
		DecisionRecordPath: decisionRecordPath,
		SupportLogPath:     supportLogPath,
		WebsterRun:         (&fakeWebsterRun{}).run,
		WebsterDeps: websterengine.RunDeps{
			Starter:    fakeMasterStarter{},
			Reed:       fakeReedOps{},
			Engine:     fakeShuttleEngine{},
			RefMatcher: fakeRefMatcher{},
		},
		Landing: landing,
		Shuttle: loomShuttle,
		DiscussionSpec: func() (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      "discussion prompt",
				OutputFiles: []string{decisionRecordPath, supportLogPath},
				Interactive: false,
				Role:        "discussion",
			}, nil
		},
		CommitDiscussion: func() error {
			loomShuttle.commitDiscussionCalls++
			return nil
		},
		PlanSpec: func() (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      "plan prompt",
				OutputFiles: []string{filepath.Join(planDir, "00-overview.md")},
				Interactive: false,
				Role:        "plan",
			}, nil
		},
		CommitPlan: func() error {
			loomShuttle.commitPlanCalls++
			return nil
		},
	}

	paths = ShedPaths{
		StatusPath:     statusPath,
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: statusLockPath,
		MaxBounces:     3,
	}

	return dir, env, paths
}
