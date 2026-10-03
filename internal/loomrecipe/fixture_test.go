// fixture_test.go implements buildSequenceFixture, the one shared Tier-1 builder every sequence,
// resume, and bounce-routing test in this package reuses rather than building its own fixture: a
// whole temp anchor a real New-built producer list can run against offline, plus the
// shedrecipe.Env/shedbuild.ShedPaths pair pointing at it.
//
// The seam structs come from envkit.FullEnv and the shuttle and burler fakes from shedfake.
// What stays here serves this package alone: the discussion and plan fixtures, the batcher config writer, the role-dispatching shuttle and burler scripts, and the webster run fake.

package loomrecipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeAlwaysDoneProducer is a minimal shedengine.ShedProducer fake that always reports Done -- used
// to substitute the built row 1 (Preflight) without spawning git, per the
// row1-substitution-is-a-seam-not-a-fixed-fake Shared Decision.
type fakeAlwaysDoneProducer struct{}

func (fakeAlwaysDoneProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// seedBouncerStencils writes the six stencils a live Plan-Write, Discussion-Review, Plan-Review, or Webster-Review segment reads at dir, keyed by stencilstore.Path(dir, name): the two generic bouncer templates (bouncer-template-seed, bouncer-template-judge) and all three segments' rubrics (loom-rubric-discussion-review, loom-rubric-plan-review, loom-rubric-webster-review) plus loom-template-prior-plan, which the Plan-Write rotator renders once it moves a seeded plan, each seeded from its real embedded contracts/stencils bytes rather than dummy content.
// shedadapters.NewBouncer probes the rubric eagerly at construction, and seedCall/judgeCall read the two templates at call
// time and degrade to Stuck when either is unreadable, so dummy templates would make
// shedengine.Done unreachable and would also diverge from the marker set internal/stencil's Fill
// requires in production.
func seedBouncerStencils(t *testing.T, dir string) {
	t.Helper()

	seeds := map[string][]byte{
		"bouncer-template-seed":         stencils.BouncerTemplateSeed,
		"bouncer-template-judge":        stencils.BouncerTemplateJudge,
		"loom-rubric-discussion-review": stencils.LoomRubricDiscussionReview,
		"loom-rubric-plan-review":       stencils.LoomRubricPlanReview,
		"loom-rubric-webster-review":    stencils.LoomRubricWebsterReview,
		"loom-template-prior-plan":      stencils.LoomTemplatePriorPlan,
	}
	for name, content := range seeds {
		path := stencilstore.Path(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir stencil dir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write stencil %s: %v", name, err)
		}
	}
}

// newLoomBurler returns the shedfake.BurlerRunner every review segment's Burler row runs through.
// Each round's DuringRun hook writes the ReviewPath and FixerReportPath the handed burlerengine.Profile names, each with short non-empty placeholder content, and the scripted Result reports shuttleengine.OutcomeDone.
// That pair-on-disk plus OutcomeDone is what makes BurlerProducer.Call return Stuck with a real report rather than erroring, which is what the Bouncer's next call then judges.
// The runner's Calls field counts the rounds.
func newLoomBurler(t *testing.T) *shedfake.BurlerRunner {
	t.Helper()
	burler := &shedfake.BurlerRunner{
		Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}},
	}
	burler.DuringRun = func(callIndex int) {
		p := burler.GotProfiles[callIndex]
		if err := os.WriteFile(p.ReviewPath, []byte("review"), 0o644); err != nil {
			t.Fatalf("write review %s: %v", p.ReviewPath, err)
		}
		if err := os.WriteFile(p.FixerReportPath, []byte("fixer report"), 0o644); err != nil {
			t.Fatalf("write fixer report %s: %v", p.FixerReportPath, err)
		}
	}
	return burler
}

// testParentReviewConfig fills the Env.ParentReview the embedded recipe's parent-review gate entry needs to build.
// Reviewer is left empty, so the gate passes without opening a request.
func testParentReviewConfig(dir, decisionRecordPath, supportLogPath string) parentreview.GateConfig {
	return parentreview.GateConfig{
		Store:          parentreview.Store{Root: filepath.Join(dir, "parent-review"), LockDir: filepath.Join(dir, "parent-review-lock")},
		Slug:           "fixture",
		DecisionRecord: decisionRecordPath,
		SupportLog:     supportLogPath,
		WaitBound:      time.Hour,
		RenderDelivery: func(string) (string, error) { return "delivery", nil },
		RenderBrief:    func() (string, error) { return "brief", nil },
	}
}

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

// writePlanFixture writes the syntactically complete, one-card plan seedPlanFixture and the loom shuttle's "plan"-role branch both write, so the two writers never drift apart.
// The sole card carries a Create group so path-missing never fires regardless of worktreeRoot's contents — a Create group's targets stay exempt from on-disk existence checking.
func writePlanFixture(planDir string, approved bool) error {
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return err
	}
	files := plankit.Render(plankit.Plan{
		Approved: approved,
		Language: "none",
		Framing:  "Framing.",
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "first-card",
			Summary: "placeholder card 1",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/firstcard/new.go"}}},
			Intent:  "placeholder card.",
		}},
	})
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(planDir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// seedPlanFixture writes a syntactically complete, one-card plan-format plan under
// <anchorPath>/_lyx/plan/, approved or not per approved, via writePlanFixture.
func seedPlanFixture(t *testing.T, anchorPath string, approved bool) {
	t.Helper()
	if err := writePlanFixture(filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan"), approved); err != nil {
		t.Fatalf("write plan fixture: %v", err)
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

// newLoomShuttle returns the shedfake.Shuttle serving row 3 (Discussion-Write), row 6 (Plan-Write),
// and all three segments' Bouncer rows' spawn roles: shedrecipe.Env carries one Shuttle field, not
// one per row, so this single fake serves all of them, and its RunFn branches on the Spec's own Role.
// On spec.Role == "plan" it writes the whole plan-directory fixture -- writePlanFixture(planDir, false) -- rather than only spec.OutputFiles, because loomshed.NewPlanWrite's rotation archives every top-level .md file in the plan directory (including the card file seedPlanFixture pre-wrote) before the shuttle runs,
// so writing only the overview would leave the Card Index naming a card file that no longer exists and Plan-Write's own gate would fail.
// The overview it writes is unapproved, mirroring the plan stencil's own "you never self-approve" rule: the real Plan-Write producer can never emit an approved plan,
// and a fake writer that did would hand the review gate a plan the production writer can never produce -- Plan-Bouncer's approve_seam is what flips the flag, not this row.
// On spec.Role == "describe" it writes a valid change description.
// On spec.Role == "bouncer-judge" it writes the verdict, ledger, and focus files named by spec.OutputFiles, in that fixed order -- see writeBouncerJudge for the shape each carries.
// There is no "bouncer-seed" branch: shedadapters.seedCall calls ensureFocus(1) regardless of what the spawn reported, synthesizing an empty-but-parsing focus file when none is on disk,
// so the default no-write-by-role branch below is already correct for the seed pass.
// Otherwise (row 3's branch) it keeps the discussion behaviour: when writeOutputs is true, it writes both discussion output files from the received Spec.OutputFiles, creating any missing parent directory.
// Every branch reports shuttleengine.OutcomeDone.
//
// The shuttle's Specs field holds every Spec in order, so a test counts the spawns of one role with countRole.
// A test that needs the shuttle reaches it by type-asserting env.Shuttle.(*shedfake.Shuttle) -- buildSequenceFixture is the only thing that ever fills that field, so the assertion is total.
// Its Attach reports not-found, the regression guard that every sequence and resume test still drives the unchanged archive-then-spawn path through Run.
func newLoomShuttle(planDir string, writeOutputs bool) *shedfake.Shuttle {
	return &shedfake.Shuttle{
		RunFn: func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
			return runLoomShuttle(planDir, writeOutputs, spec)
		},
	}
}

// runLoomShuttle is newLoomShuttle's per-role dispatch.
func runLoomShuttle(planDir string, writeOutputs bool, spec shuttleengine.Spec) (shuttleengine.Result, error) {
	switch spec.Role {
	case "plan":
		if err := writePlanFixture(planDir, false); err != nil {
			return shuttleengine.Result{}, fmt.Errorf("loom shuttle: write plan fixture %s: %w", planDir, err)
		}
		return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil

	case "describe":
		// A valid change description: a "# title" heading and a non-empty body, no trailer line,
		// so the description gate passes.
		for _, path := range spec.OutputFiles {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return shuttleengine.Result{}, fmt.Errorf("loom shuttle: mkdir %s: %w", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, []byte("# Fixture change\n\nFixture description body.\n"), 0o644); err != nil {
				return shuttleengine.Result{}, fmt.Errorf("loom shuttle: write description %s: %w", path, err)
			}
		}
		return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil

	case "bouncer-judge":
		return writeBouncerJudge(spec, "APPROVED")
	}

	if writeOutputs {
		contents := []string{validDecisionRecord, "support log"}
		for i, path := range spec.OutputFiles {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return shuttleengine.Result{}, fmt.Errorf("loom shuttle: mkdir %s: %w", filepath.Dir(path), err)
			}
			content := ""
			if i < len(contents) {
				content = contents[i]
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return shuttleengine.Result{}, fmt.Errorf("loom shuttle: write %s: %w", path, err)
			}
		}
	}
	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
}

// writeBouncerJudge writes the three files spec.OutputFiles names, in shedadapters' own fixed
// order -- the round's verdict path, its ledger path, and the next round's focus path -- each
// satisfying its own package-private parser (parseVerdict, parseLedger, and parseFocus
// respectively). The round number embedded in every file is spec.Round, which the Bouncer fills
// with the round it is judging, never a hardcoded 1. The verdict written is verdict.
func writeBouncerJudge(spec shuttleengine.Spec, verdict string) (shuttleengine.Result, error) {
	if len(spec.OutputFiles) != 3 {
		return shuttleengine.Result{}, fmt.Errorf("loom shuttle: bouncer-judge spec.OutputFiles has %d entries; want 3 (verdict, ledger, focus)", len(spec.OutputFiles))
	}
	round, err := strconv.Atoi(spec.Round)
	if err != nil {
		return shuttleengine.Result{}, fmt.Errorf("loom shuttle: bouncer-judge spec.Round %q: %w", spec.Round, err)
	}

	verdictContent := fmt.Sprintf("---\nverdict: %s\nrationale: fixture rationale for round %d\n---\n", verdict, round)
	if err := os.WriteFile(spec.OutputFiles[0], []byte(verdictContent), 0o644); err != nil {
		return shuttleengine.Result{}, fmt.Errorf("loom shuttle: write bouncer verdict %s: %w", spec.OutputFiles[0], err)
	}

	ledgerContent := fmt.Sprintf("---\nround: %d\nledger: []\n---\n", round)
	if err := os.WriteFile(spec.OutputFiles[1], []byte(ledgerContent), 0o644); err != nil {
		return shuttleengine.Result{}, fmt.Errorf("loom shuttle: write bouncer ledger %s: %w", spec.OutputFiles[1], err)
	}

	// The focus file targets round+1, matching the path shedadapters.focusPath(runDir, round+1)
	// names; Bouncer.settle on an APPROVED verdict never reads it, so this write only matters when
	// verdict scripts a BLOCKING round.
	focusContent := fmt.Sprintf("---\nround: %d\nexclude_lenses: []\nfocus: []\n---\n", round+1)
	if err := os.WriteFile(spec.OutputFiles[2], []byte(focusContent), 0o644); err != nil {
		return shuttleengine.Result{}, fmt.Errorf("loom shuttle: write bouncer focus %s: %w", spec.OutputFiles[2], err)
	}

	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
}

// blockBouncerJudge makes every bouncer-judge round of env's loom shuttle write a BLOCKING verdict instead of APPROVED,
// so the Bouncer reports Stuck on each judge call.
func blockBouncerJudge(env shedrecipe.Env) {
	shuttle := env.Shuttle.(*shedfake.Shuttle)
	dispatch := shuttle.RunFn
	shuttle.RunFn = func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
		if spec.Role == "bouncer-judge" {
			return writeBouncerJudge(spec, "BLOCKING")
		}
		return dispatch(spec)
	}
}

// countRole reports how many of shuttle's Run calls carried spec.Role == role.
func countRole(shuttle *shedfake.Shuttle, role string) int {
	count := 0
	for _, spec := range shuttle.Specs {
		if spec.Role == role {
			count++
		}
	}
	return count
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

// buildSequenceFixture builds a temp anchor whose on-disk state makes rows 3 (Discussion-Write's own
// gate), 7 (Plan-Write's own gate), and 9 (Batchifier) -- the three real, non-injectable producers
// this task builds -- genuinely pass, and returns the anchor path alongside the
// shedrecipe.Env/shedbuild.ShedPaths pair pointing at it.
// The Env starts from envkit.FullEnv and overrides the anchor, the paths, the loom shuttle, the burler, and the specs the sequence tests script.
//
// Discussion-Write's gate: both discussion files are written, the decision record carrying all
// seven required H2 sections (writeDiscussionFixture). Plan-Write's gate: a
// syntactically complete, unapproved, one-card plan directory that satisfies every
// planparser.ValidateFormat check, including the ones that stat paths against the worktree root
// (seedPlanFixture) -- the same self-authored, single-card, zero-findings shape
// internal/planparser/testdata/goodplan/00-overview.md and 01-json-flag.md model. Seeded unapproved
// rather than approved: both gate sites run planglyph.ValidateFormat, which never demands the
// approval flag, and an approved seed would misrepresent what the real Plan-Write producer is ever
// allowed to write. Batchifier: no batcher.yaml is written at all, so batcher.Active resolves the
// embedded template, which is a Done.
//
// The status file is seeded through the production loomshed.Seed, never by hand-writing JSON, so a
// Seed regression would not pass unnoticed here. Row 1 is not injected here: New builds it from
// env.Cwd via preflightEntry, and every caller of this fixture that drives Run substitutes
// shed.Producers[0].Producer after New per the row1-substitution-is-a-seam-not-a-fixed-fake Shared
// Decision.
// WebsterRun is fakeWebsterRun's run method, reporting Webster's own done outcome,
// and WebsterDeps keeps FullEnv's placeholder seams.
// LockPath and StatusLockPath are given two distinct paths, since shedengine rejects them naming one file.
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
// newLoomShuttle writing its outputs, env.DiscussionSpec is a closure returning a Spec whose
// OutputFiles is the same [decisionRecordPath, supportLogPath] pair this fixture already computes
// above, and env.CommitDiscussion keeps FullEnv's no-op, which a test that counts commits wraps.
// The fake shuttle writing both output files on every Run is what keeps Discussion-Write's own gate
// passing here: shedadapters.archiveStaleOutputs renames the fixture's own pre-written files away
// on every Call, so without the fake rewriting them the clean sequence run would find both files
// absent.
//
// Row 6 (Plan-Write) is likewise a real shedadapters.SingleLLMProducer behind loomshed's rotate-and-commit decorator.
// The same loom shuttle serves this row too: its planDir is the same _lyx/plan expression seedPlanFixture already builds,
// env.PlanSpec is a closure returning a Spec naming Role: "plan" and OutputFiles holding the single overview path.
// The shuttle's "plan"-role branch rewrites the whole plan directory (see newLoomShuttle's own doc comment for why) rather than only the overview,
// so Plan-Write's own gate still finds a complete, unapproved, zero-findings plan after the decorator's rotation archived the seeded one away -- the real Plan-Write producer can never write it approved, and neither does this fake.
// env.ApprovePlan is a closure running the real planparser.SetApproved over the same plan directory, wired to Plan-Bouncer's approve_seam key -- the flag is only ever set once the review segment's approved settle fires.
func buildSequenceFixture(t *testing.T) (anchorPath string, env shedrecipe.Env, paths shedbuild.ShedPaths) {
	t.Helper()

	dir := t.TempDir()

	discussionDir := filepath.Join(dir, "discussion")
	if err := os.MkdirAll(discussionDir, 0o755); err != nil {
		t.Fatalf("mkdir discussion dir: %v", err)
	}
	decisionRecordPath, supportLogPath := writeDiscussionFixture(t, discussionDir, validDecisionRecord, "support log")

	// Seeded unapproved, matching what the real Plan-Write producer must write: it is inert at the
	// gate -- loomshed.NewPlanWrite's rotation archives every top-level .md file in the plan
	// directory before the shuttle runs, so the loom shuttle's "plan"-role branch rewrites the whole
	// directory rather than only its declared output file -- but leaving it approved here would be
	// dishonest about what the fixture models.
	seedPlanFixture(t, dir, false)

	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	if err := loomshed.Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}

	landing := envkit.LandingDeps(dir)
	landing.PushSkipped = true
	landing.Config.RequirePRToBase = []string{landing.ParentBranch}

	cwd := filepath.Join(dir, "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	planDir := filepath.Join(dir, lyxdirs.LyxDirName, "plan")
	descriptionPath := filepath.Join(dir, lyxdirs.LyxDirName, "landing", "summary.md")

	runRoot := filepath.Join(dir, "reviews")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("mkdir run root: %v", err)
	}
	stencilsDir := filepath.Join(dir, "stencils")
	seedBouncerStencils(t, stencilsDir)
	specsDir := filepath.Join(dir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatalf("mkdir specs dir: %v", err)
	}

	env = envkit.FullEnv(t)
	env.ParentReview = testParentReviewConfig(dir, decisionRecordPath, supportLogPath)
	env.Cwd = cwd
	env.AnchorPath = dir
	env.WorktreeRoot = dir
	env.StatusPath = statusPath
	env.StatusLockPath = statusLockPath
	env.DecisionRecordPath = decisionRecordPath
	env.SupportLogPath = supportLogPath
	env.WebsterRun = (&fakeWebsterRun{}).run
	env.Landing = landing
	env.Shuttle = newLoomShuttle(planDir, true)
	env.RunRoot = runRoot
	env.StencilsDir = stencilsDir
	env.SpecsDir = specsDir
	env.Burler = newLoomBurler(t)
	env.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	env.DiscussionSpec = func() (shuttleengine.Spec, error) {
		return shuttleengine.Spec{
			Prompt:      "discussion prompt",
			OutputFiles: []string{decisionRecordPath, supportLogPath},
			Interactive: false,
			Role:        "discussion",
		}, nil
	}
	env.DescriptionPath = descriptionPath
	env.DescribeSpec = func() (shuttleengine.Spec, error) {
		return shuttleengine.Spec{
			Prompt:      "describe prompt",
			OutputFiles: []string{descriptionPath},
			Interactive: false,
			Role:        "describe",
		}, nil
	}
	env.PlanSpec = func() (shuttleengine.Spec, error) {
		return shuttleengine.Spec{
			Prompt:      "plan prompt",
			OutputFiles: []string{filepath.Join(planDir, "00-overview.md")},
			Interactive: false,
			Role:        "plan",
		}, nil
	}
	// ApprovePlan is a closure running the real planparser.SetApproved over this fixture's own
	// plan directory, not a fake that merely records the call: the seam under test is the write
	// itself, and a fake that only recorded invocation would let a broken writer pass.
	env.ApprovePlan = func() error {
		return planparser.SetApproved(planDir)
	}

	paths = shedbuild.ShedPaths{
		StatusPath:     statusPath,
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: statusLockPath,
		MaxBounces:     3,
	}

	return dir, env, paths
}
