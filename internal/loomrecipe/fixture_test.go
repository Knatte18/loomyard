// fixture_test.go implements buildSequenceFixture, the shared Tier-1 builder of this package's tests.
// Sequence, resume, and bounce-routing tests reuse it rather than building their own fixture.
// It builds a whole temp anchor that a real New-built producer list can run against offline.
// The shedrecipe.Env/shedbuild.ShedPaths pair points at that anchor.
// The seam structs come from envkit.FullEnv and the shuttle and burler fakes from shedfake.
// This file holds what serves this package alone: the discussion and plan fixtures and the batcher config writer.
// It also holds the role-dispatching shuttle and burler scripts, and the webster run fake.

package loomrecipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeAlwaysDoneProducer is a minimal shedengine.ShedProducer fake that always reports Done.
// It substitutes the built row 1 (Preflight) without spawning git.
type fakeAlwaysDoneProducer struct{}

func (fakeAlwaysDoneProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// newLoomBurler returns the shedfake.BurlerHalfRunner every review segment's Burler rows run through.
// Each review writes the ReviewPath of its profile,
// and each fix writes its FixerReportPath with short non-empty placeholder content;
// the scripted results report shuttleengine.OutcomeDone.
// The file on disk and OutcomeDone together let a review hand on to its fix,
// and the completed round is what the Bouncer's next call judges.
// The runner's ReviewCalls and FixCalls fields count the rounds.
func newLoomBurler(t *testing.T) *shedfake.BurlerHalfRunner {
	t.Helper()
	burler := &shedfake.BurlerHalfRunner{
		ReviewResults: []burlerengine.ReviewResult{{Outcome: shuttleengine.OutcomeDone, Digest: "review-digest"}},
		FixResults:    []burlerengine.FixResult{{Outcome: shuttleengine.OutcomeDone}},
	}
	burler.DuringReview = func(callIndex int) {
		p := burler.GotReviewProfiles[callIndex]
		if err := os.WriteFile(p.ReviewPath, []byte("---\nverdict: APPROVED\nfindings: []\n---\nreview\n"), 0o644); err != nil {
			t.Fatalf("write review %s: %v", p.ReviewPath, err)
		}
	}
	burler.DuringFix = func(callIndex int) {
		p := burler.GotFixProfiles[callIndex]
		if err := os.WriteFile(p.FixerReportPath, []byte("fixer report"), 0o644); err != nil {
			t.Fatalf("write fixer report %s: %v", p.FixerReportPath, err)
		}
	}
	return burler
}

// parentReviewRoundRel is the anchor-relative path of the open parent-review request that the sequence fixtures lay out.
const parentReviewRoundRel = "_lym/reviews/parent-review/round-1/request.json"

// openParentReviewRequest is the request an older binary left for an open round: state open and no verdict beside it.
const openParentReviewRequest = `{"slug":"fixture-slug","round":1,"state":"open"}` + "\n"

// validDecisionRecord carries every required section in order, plus the optional one.
const validDecisionRecord = `# Decision record

## Goal

Goal text.

## Scope

Scope text.

## Decisions

### D1 — Fixture decision

Decisions text.

## Constraints

Constraints text.

## Auto-mode assumptions

Assumptions text.

## Open risks

Risks text.

## Acceptance criteria

- D1 holds.

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

// writePlanFixture writes a syntactically complete, one-card plan.
// Both the seeded plan and the shuttle's "plan"-role output come from it, so the two writers never drift apart.
// The sole card carries a Create group, whose targets are exempt from on-disk existence checks,
// so the path-missing check never fires whatever the worktree holds.
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

// seedPlanFixture writes a one-card plan under <anchorPath>/_lym/plan/, approved or not per approved.
func seedPlanFixture(t *testing.T, anchorPath string, approved bool) {
	t.Helper()
	if err := writePlanFixture(filepath.Join(anchorPath, lymdirs.LymDirName, "plan"), approved); err != nil {
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

// newLoomShuttle returns the shedfake.Shuttle serving the spawn roles of every Bouncer row.
// shedrecipe.Env carries one Shuttle field, not one per row,
// so this single fake serves them all, and its RunFn branches on the Spec's Role.
// The Plan-Write and Describe rows run on the seat engine, whose fake newLoomSeats shares writeLoomOutputs with it.
// For the "plan" role it writes the whole plan-directory fixture, writePlanFixture(planDir, false).
// The plan-write rotation archives every top-level .md file in the plan directory before the shuttle runs,
// so writing only spec.OutputFiles would leave the Card Index naming a card file that no longer exists,
// and the plan-write gate would fail.
// The overview is unapproved, as the plan stencil requires: the production Plan-Write never emits an approved plan.
// Plan-Bouncer's approve_seam is what flips the flag,
// so a fake that approved the overview would hand the review gate a plan the production writer cannot produce.
// For the "describe" role it writes a valid change description.
// For a role ending in "bouncer-judge", whatever segment prefix leads it, it writes three files:
// the verdict, the ledger, and the focus, in that order, all named by spec.OutputFiles;
// writeBouncerJudge gives each one's shape.
// There is no "bouncer-seed" branch: shedadapters.seedCall calls ensureFocus(1) whatever the spawn reported.
// ensureFocus synthesizes an empty focus file that parses when none is on disk;
// the default branch below therefore already serves the seed pass.
// Otherwise the discussion behaviour applies:
// when writeOutputs is true, it writes both discussion output files from spec.OutputFiles.
// It creates any missing parent directory.
// Every branch reports shuttleengine.OutcomeDone.
//
// The shuttle's Specs field holds every Spec in order, so a test counts the spawns of one role with countRole.
// A test reaches the shuttle by type-asserting env.Shuttle to *shedfake.Shuttle;
// buildSequenceFixture is the only code that fills that field, so the assertion is total.
// Its Attach reports not-found, a regression guard:
// every sequence and resume test must still drive the archive-then-spawn path through Run.
func newLoomShuttle(planDir string, writeOutputs bool) *shedfake.Shuttle {
	return &shedfake.Shuttle{
		RunFn: func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
			return runLoomShuttle(planDir, writeOutputs, spec)
		},
	}
}

// newLoomSeats returns a fake seat runner that serves the Discussion-Write row, which runs on the seat engine.
// Its RunFn writes the lead's outputs as writeLoomOutputs does for the table's role prefix.
// It reports a done lead with no GateOutcome, so the fake evaluates the table's own gate once.
func newLoomSeats(planDir string, writeOutputs bool) *shedfake.SeatRunner {
	return &shedfake.SeatRunner{
		RunFn: func(table seatengine.Table) (seatengine.Result, error) {
			outputs := table.Chair().Outputs
			if err := writeLoomOutputs(planDir, writeOutputs, table.RolePrefix, outputs); err != nil {
				return seatengine.Result{}, fmt.Errorf("loom seats: %w", err)
			}
			return seatengine.Result{
				Chair:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
				ChairOutputs: outputs,
			}, nil
		},
	}
}

// countSeatRole reports how many of seats' Run calls carried a table with the given role prefix.
func countSeatRole(seats *shedfake.SeatRunner, prefix string) int {
	count := 0
	for _, table := range seats.GotTables {
		if table.RolePrefix == prefix {
			count++
		}
	}
	return count
}

// runLoomShuttle answers one shuttle run with the fake outcome for its role.
// A bouncer-judge role gets a CONVERGED verdict, and every other role gets the outputs its fake writer leaves behind.
func runLoomShuttle(planDir string, writeOutputs bool, spec shuttleengine.Spec) (shuttleengine.Result, error) {
	if strings.HasSuffix(spec.Role, "bouncer-judge") {
		return writeBouncerJudge(spec, "CONVERGED")
	}
	if err := writeLoomOutputs(planDir, writeOutputs, spec.Role, spec.OutputFiles); err != nil {
		return shuttleengine.Result{}, fmt.Errorf("loom shuttle: %w", err)
	}
	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
}

// writeLoomOutputs writes the files a fake writer of the given role leaves behind.
// The fake shuttle and the fake seat runner both use it, so they serve one set of row fixtures.
// Role "plan" writes the whole plan directory under planDir.
// Role "parts" writes a parts file of one part.
// Role "parts-plan" writes that part's plan, the plan directory and the join record.
// Role "describe" writes a valid change description to each output.
// Any other role writes the valid discussion content to outputs when writeOutputs is true.
func writeLoomOutputs(planDir string, writeOutputs bool, role string, outputs []string) error {
	switch role {
	case "plan":
		if err := writePlanFixture(planDir, false); err != nil {
			return fmt.Errorf("write plan fixture %s: %w", planDir, err)
		}
		return nil

	case "parts":
		// One part, "whole", carrying every decision id of the fixture's record.
		ids := discussionparser.DecisionIDs([]byte(validDecisionRecord))
		parts := "# Parts: Fixture\n\n## Split note\n\nThe fixture task does not split.\n\n## Part: whole\n\n" +
			"- Decisions: " + strings.Join(ids, ", ") + "\n- Takes in: the decision record.\n- Delivers: the fixture change.\n- Area:\n  - `internal/firstcard`\n"
		for _, path := range outputs {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, []byte(parts), 0o644); err != nil {
				return fmt.Errorf("write parts file %s: %w", path, err)
			}
		}
		return nil

	case "parts-plan":
		// The chair's outputs are the join record, which sits in the parts directory beside the part's directory.
		for _, path := range outputs {
			if err := writePlanFixture(filepath.Join(filepath.Dir(path), "whole"), false); err != nil {
				return fmt.Errorf("write part plan fixture: %w", err)
			}
			if err := writePlanFixture(planDir, false); err != nil {
				return fmt.Errorf("write plan fixture %s: %w", planDir, err)
			}
			if err := os.WriteFile(path, []byte("# Join\n\nThe fixture's one part joined.\n"), 0o644); err != nil {
				return fmt.Errorf("write join record %s: %w", path, err)
			}
		}
		return nil

	case "describe":
		// A valid change description has a "# title" heading and a non-empty body, and no trailer line.
		// The description gate accepts it.
		for _, path := range outputs {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, []byte("# Fixture change\n\nFixture description body.\n"), 0o644); err != nil {
				return fmt.Errorf("write description %s: %w", path, err)
			}
		}
		return nil
	}

	if !writeOutputs {
		return nil
	}
	contents := []string{validDecisionRecord, "support log"}
	for i, path := range outputs {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
		}
		content := ""
		if i < len(contents) {
			content = contents[i]
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// writeBouncerJudge writes the three files spec.OutputFiles names, in the fixed order shedadapters uses:
// the round's verdict path, its ledger path, and the next round's focus path.
// Each file satisfies the package-private parser of its kind: parseVerdict, parseLedger, and parseFocus.
// The round number in every file is spec.Round, the round the Bouncer is judging, never a hardcoded 1.
// The verdict written is verdict.
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

	// The focus file targets round+1, the path shedadapters.focusPath(runDir, round+1) names.
	// Bouncer.settle never reads it on a CONVERGED verdict,
	// so this write matters only when the verdict scripts a CONTINUE round.
	focusContent := fmt.Sprintf("---\nround: %d\nexclude_lenses: []\nfocus: []\n---\n", round+1)
	if err := os.WriteFile(spec.OutputFiles[2], []byte(focusContent), 0o644); err != nil {
		return shuttleengine.Result{}, fmt.Errorf("loom shuttle: write bouncer focus %s: %w", spec.OutputFiles[2], err)
	}

	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
}

// blockBouncerJudge makes every bouncer-judge round of env's loom shuttle write a CONTINUE verdict.
// The Bouncer then reports Stuck on each judge call.
func blockBouncerJudge(env shedrecipe.Env) {
	shuttle := env.Shuttle.(*shedfake.Shuttle)
	dispatch := shuttle.RunFn
	shuttle.RunFn = func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
		if strings.HasSuffix(spec.Role, "bouncer-judge") {
			return writeBouncerJudge(spec, "CONTINUE")
		}
		return dispatch(spec)
	}
}

// designWrongBouncerJudge makes every bouncer-judge round of env's loom shuttle write a CONTINUE verdict.
// The ledger it writes holds one open design-wrong entry.
// The soft-cap rule then continues a spent budget and escalates at the hard cap.
func designWrongBouncerJudge(env shedrecipe.Env) {
	shuttle := env.Shuttle.(*shedfake.Shuttle)
	dispatch := shuttle.RunFn
	shuttle.RunFn = func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
		if !strings.HasSuffix(spec.Role, "bouncer-judge") {
			return dispatch(spec)
		}
		result, err := writeBouncerJudge(spec, "CONTINUE")
		if err != nil {
			return result, err
		}
		round, err := strconv.Atoi(spec.Round)
		if err != nil {
			return shuttleengine.Result{}, fmt.Errorf("loom shuttle: bouncer-judge spec.Round %q: %w", spec.Round, err)
		}
		ledgerContent := fmt.Sprintf("---\nround: %d\nledger:\n  - key: design-gap\n    status: open\n    rounds: [%d]\n    class: design\n    severity: MEDIUM\n    fix: design-wrong\n---\n", round, round)
		if err := os.WriteFile(spec.OutputFiles[1], []byte(ledgerContent), 0o644); err != nil {
			return shuttleengine.Result{}, fmt.Errorf("loom shuttle: write bouncer ledger %s: %w", spec.OutputFiles[1], err)
		}
		return result, nil
	}
}

// countRole reports how many of shuttle's Run calls carried a spec.Role ending in role.
func countRole(shuttle *shedfake.Shuttle, role string) int {
	count := 0
	for _, spec := range shuttle.Specs {
		if strings.HasSuffix(spec.Role, role) {
			count++
		}
	}
	return count
}

// writeBatcherConfig writes content as <anchorPath>/_lym/config/batcher.yaml.
func writeBatcherConfig(t *testing.T, anchorPath, content string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, lymdirs.LymDirName, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "batcher.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write batcher.yaml: %v", err)
	}
}

// buildSequenceFixture builds a temp anchor whose on-disk state makes three real, non-injectable checks pass.
// The checks are Discussion-Write's gate, Plan-Write's gate, and Webster's batcher load, all on the main line.
// It returns the anchor path with the shedrecipe.Env and shedbuild.ShedPaths pair that points at it.
// The Env starts from envkit.FullEnv.
// It overrides the anchor, the paths, the loom shuttle, the burler, and the specs the sequence tests script.
// The anchor also holds an open parent-review round at parentReviewRoundRel.
//
// Discussion-Write's gate needs both discussion files written.
// The decision record carries every required H2 section.
// writeDiscussionFixture writes both files.
//
// Plan-Write's gate needs a syntactically complete plan directory that passes every planparser.ValidateFormat check.
// That includes the checks that stat paths against the worktree root.
// seedPlanFixture writes such a plan: one self-authored card with zero findings.
// Its shape matches the files under internal/planparser/testdata/goodplan.
// It is seeded unapproved, because both gate sites run planglyph.ValidateFormat, which never demands the approval flag.
// An approved seed would show a state the real Plan-Write producer never writes.
// No batcher.yaml is written, so batcher.Active resolves the embedded template.
//
// The status file is seeded through the production loomshed.Seed, never by hand-writing JSON.
// A regression in Seed therefore shows up here.
// Row 1 is not injected here: New builds it from env.Cwd via preflightEntry.
// A caller that drives Run replaces shed.Producers[0].Producer after New.
// WebsterRun is a fake run that reports Webster's own done outcome.
// WebsterDeps keeps the placeholder seams of envkit.FullEnv.
// LockPath and StatusLockPath get two distinct paths, because shedengine rejects a pair that names one file.
//
// Merge-In, Publish and Finalize are the real producers, and this fixture never drives any of them to a genuine merge.
// The fake tip reads report the local parent ahead of its upstream.
// So Merge-In reports Stuck with OnStuck set to "", which blocks the whole run there, before it merges anything.
// Publish's and Finalize's Call is never reached either.
// Driving their merge logic for real needs a genuine two-worktree pair.
// The untagged tier of this package does not build one.
// The origin reconcile reports a stopped entry, which stops Merge-In before it reads a tip or touches the worktree.
// The integration tier covers the real merge.
//
// The Discussion-Write, Plan-Write and Describe rows run a real shedadapters.MultiLLMProducer over env.Seats.
// Each row sits behind its own decorator.
// env.Seats is a fake seat runner.
// Each row's table closure returns a one-lead table; its role prefix picks the files the fake seat runner writes.
//
// Row 3 (Discussion-Write) sits behind loomshed's commit decorator.
// env.DiscussionTable returns a table whose outputs are the [decisionRecordPath, supportLogPath] pair.
// This fixture computes that pair.
// env.CommitDiscussion keeps the no-op of envkit.FullEnv, and a test that counts commits wraps it.
// The fake seat runner writes both output files on every Run, which keeps Discussion-Write's gate passing.
// The stale-output archive renames the fixture's pre-written files away on every Call.
// Without the fake rewriting them, a clean sequence run would find both files absent.
//
// Row 6 (Plan-Write) sits behind loomshed's rotate-and-commit decorator.
// Its planDir is the _lym/plan path that seedPlanFixture builds.
// env.PlanTable returns a table with role prefix "plan" and the single overview path as its output.
// The seat runner's "plan" branch rewrites the whole plan directory, not only the overview.
// So Plan-Write's gate still finds a complete, unapproved, zero-findings plan.
// The decorator's rotation archives the seeded one first.
// The real Plan-Write producer never writes a plan approved, and neither does this fake.
//
// The Describe row sits behind the Discussion-Write commit decorator.
// env.DescribeTable returns a table with role prefix "describe" and the description path as its output.
// The fake seat runner fills that path with a valid change description.
// env.ApprovePlan is a closure that runs the real planparser.SetApproved over the same plan directory.
// It is wired to Plan-Bouncer's approve_seam key.
// The approval flag is set only once the review segment's approved settle fires.
func buildSequenceFixture(t *testing.T) (anchorPath string, env shedrecipe.Env, paths shedbuild.ShedPaths) {
	t.Helper()

	dir := t.TempDir()

	discussionDir := filepath.Join(dir, "discussion")
	if err := os.MkdirAll(discussionDir, 0o755); err != nil {
		t.Fatalf("mkdir discussion dir: %v", err)
	}
	decisionRecordPath, supportLogPath := writeDiscussionFixture(t, discussionDir, validDecisionRecord, "support log")

	// An open parent-review round, as an older binary leaves it: a request with no verdict.
	// No gate reads it, so a run waiting in it passes Discussion-Write on its next gate evaluation.
	parentReviewRound := filepath.Join(dir, parentReviewRoundRel)
	if err := os.MkdirAll(filepath.Dir(parentReviewRound), 0o755); err != nil {
		t.Fatalf("mkdir parent-review round dir: %v", err)
	}
	if err := os.WriteFile(parentReviewRound, []byte(openParentReviewRequest), 0o644); err != nil {
		t.Fatalf("write parent-review request: %v", err)
	}

	// Seeded unapproved, matching what the real Plan-Write producer must write.
	// The gate treats it as inert:
	// loomshed.NewPlanWrite's rotation archives every top-level .md file in the plan directory before the shuttle runs.
	// The loom shuttle's "plan"-role branch then rewrites the whole directory rather than only its output file.
	// Approving it here would misstate what the fixture models.
	seedPlanFixture(t, dir, false)

	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	if err := loomshed.Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}

	landing := envkit.LandingDeps(dir)
	landing.Config.DeployBranches = []string{landing.ParentBranch}
	// The origin reconcile reports the run's entry stopped, so Merge-In halts before it merges anything.
	landing.Reconcile = func() (fabricengine.OriginReport, error) {
		return fabricengine.OriginReport{
			Entries: []fabricengine.EntryOutcome{{State: fabricengine.EntryStopped, WayForward: "resolve the fixture's stop"}},
		}, nil
	}

	cwd := filepath.Join(dir, "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	planDir := filepath.Join(dir, lymdirs.LymDirName, "plan")
	descriptionPath := filepath.Join(dir, lymdirs.LymDirName, "landing", "summary.md")

	runRoot := filepath.Join(dir, "reviews")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("mkdir run root: %v", err)
	}
	stencilsDir := filepath.Join(dir, "stencils")
	stencilkit.SeedInto(t, stencilsDir)
	specsDir := filepath.Join(dir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatalf("mkdir specs dir: %v", err)
	}

	env = envkit.FullEnv(t)
	env.Cwd = cwd
	env.AnchorPath = dir
	env.ConfigDirs = configengine.Dirs{Anchor: dir}
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
	env.Seats = newLoomSeats(planDir, true)
	env.DiscussionTable = func() (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "discussion",
			Segment:    segmentcolor.Discussion,
			Seats: []seatengine.Seat{{
				Stencil: "loom-template-discussion-chair",
				Outputs: []string{decisionRecordPath, supportLogPath},
			}},
		}, nil
	}
	env.DescriptionPath = descriptionPath
	env.DescribeTable = func() (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "describe",
			Segment:    segmentcolor.Describe,
			Seats: []seatengine.Seat{{
				Stencil: "landing-template-describe",
				Outputs: []string{descriptionPath},
			}},
		}, nil
	}
	env.PlanTable = func() (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "plan",
			Segment:    segmentcolor.Plan,
			Seats: []seatengine.Seat{{
				Stencil: "loom-template-plan",
				Outputs: []string{filepath.Join(planDir, "00-overview.md")},
			}},
		}, nil
	}
	// ApprovePlan calls the real planparser.SetApproved over this fixture's own plan directory.
	// It is not a fake that only records the call.
	// The seam under test is the write itself, and a fake that only recorded invocation would let a broken writer pass.
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
