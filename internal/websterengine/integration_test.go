//go:build integration

// integration_test.go exercises the plan-level integration-suite stage end
// to end through Run itself (Tier 2 — see docs/benchmarks/running-tests.md):
// a real scratch git repo backs WorktreeRoot (via newRunFixture, reused from
// runlevel_test.go), and the Master spawn's own onWait side effect scripts
// the integration fork's own report file the same way it scripts
// outcome.yaml/summary.md — modeling Master having already spawned and
// awaited that fork, per webster-template-master.md's own integration-fork bracket
// instruction, before Master's session itself finishes. This package's
// testmain_test.go already wires gitkit.HermeticGitEnv() for the whole
// test binary; every fixture helper this file uses (newRunFixture, mustGit,
// commitFile, seedMatchingState, seedShuttleRunState) is already defined in
// beginbatch_test.go/runlevel_test.go, in the same external
// websterengine_test package.

package websterengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// appendIntegrationVerify appends a plan-level "## verify:" section to an
// already-seeded plan directory's 00-overview.md, so a test built on
// newRunFixture's own plan dir can additionally exercise the integration
// stage (ShouldRunIntegration(plan) == true) without needing a second,
// separately-seeded plan directory.
func appendIntegrationVerify(t *testing.T, planDir, verify string) {
	t.Helper()
	path := filepath.Join(planDir, "00-overview.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read overview fixture: %v", err)
	}
	data = append(data, []byte("\n## verify:\n\n"+verify+"\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write overview fixture with verify: %v", err)
	}
}

// TestShouldRunIntegration_TrueOnlyWhenPlanHasVerify proves the skip-check itself: a plan with no
// plan-level "## verify:" section reports false,
// and the same plan directory reports true once that section is appended.
func TestShouldRunIntegration_TrueOnlyWhenPlanHasVerify(t *testing.T) {
	fx := newRunFixture(t, 1)

	plan, err := planparser.ParsePlan(fx.PlanDir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v", err)
	}
	if websterengine.ShouldRunIntegration(plan) {
		t.Errorf("ShouldRunIntegration() = true for a plan with no \"## verify:\" section; want false")
	}

	appendIntegrationVerify(t, fx.PlanDir, "true")
	plan2, err := planparser.ParsePlan(fx.PlanDir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v", err)
	}
	if !websterengine.ShouldRunIntegration(plan2) {
		t.Errorf("ShouldRunIntegration() = false for a plan with a \"## verify:\" section; want true")
	}
}

// TestIntegrationStage_SkipsWhenPlanHasNoVerify proves the whole-run behavior for a plan with no
// plan-level verify: Run finishes with outcome: done exactly as it would without this task's own
// integration stage,
// and the stage never even looks for an integration report.
func TestIntegrationStage_SkipsWhenPlanHasNoVerify(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-noverify",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-noverify",
			RunDir:    "/run/dir/noverify",
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped batch1\n\nAll good.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-noverify", "master-session-noverify")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("RunResult.Outcome = %q; want %q", result.Outcome, "done")
	}

	if _, statErr := os.Stat(websterengine.IntegrationReportPath(fx.Deps.Geom.ReportsDir)); !os.IsNotExist(statErr) {
		t.Errorf("integration report present for a plan with no \"## verify:\" section; want the stage to have never looked for one")
	}
}

// TestIntegrationStage_PassingForkFinishesNormally proves the happy path: a plan-level verify
// present, every batch terminal-done, and an OK integration report already on disk by the time
// Master's own spawn finishes — Run succeeds with outcome: done and records no escalation.
func TestIntegrationStage_PassingForkFinishesNormally(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "true")

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{"deadbeef"}},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-intpass",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-intpass",
			RunDir:    "/run/dir/intpass",
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			head := gitkit.RevParse(t, fx.Worktree, "HEAD")
			reportPath := websterengine.IntegrationReportPath(fx.Deps.Geom.ReportsDir)
			if err := os.WriteFile(reportPath, []byte("status: OK\nhead_sha: "+head+"\ndeviations: []\n"), 0o644); err != nil {
				t.Fatalf("write integration report: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped batch1\n\nAll good.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-intpass", "master-session-intpass")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("RunResult.Outcome = %q; want %q", result.Outcome, "done")
	}

	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if _, escalated := st.Batches[-1]; escalated {
		t.Errorf("integration escalation record present after a passing integration report; want none")
	}

	// Run must have pre-rendered the integration fork's prompt file: Master
	// may write nothing but its two contract files, so the prompt Master
	// forwards to the integration fork exists only if Go wrote it (crucible
	// round fable-r1's F18 — a live Master correctly refused to improvise
	// one and the whole stage was unreachable).
	if _, statErr := os.Stat(filepath.Join(fx.Deps.Geom.PromptsDir, "integration.md")); statErr != nil {
		t.Errorf("stat(integration prompt) = %v; want run to have pre-rendered it for a plan with a verify section", statErr)
	}
}

// TestIntegrationStage_FailingForkTriggersBisectAndEscalates proves the full failure path against a
// real scratch repo with a known-bad commit: three batches' worth of real commits, the third
// introducing a file the plan-level verify command fails against.
// A FAILED integration report triggers bisect, which localizes the third card without a linear scan
// (the binary search only ever checks the middle candidate), records the terminal escalation under
// state.json's reserved key, extends summary.md naming the offending card, and restores HEAD to the
// original branch.
func TestIntegrationStage_FailingForkTriggersBisectAndEscalates(t *testing.T) {
	fx := newRunFixture(t, 3)
	appendIntegrationVerify(t, fx.PlanDir, "test ! -f bad.marker")

	originalBranch := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "symbolic-ref", "--short", "HEAD"))

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")
	sha2 := gitkit.CommitFile(t, fx.Worktree, "card2.txt", "two", "card2")
	sha3 := gitkit.CommitFile(t, fx.Worktree, "bad.marker", "bad", "card3 introduces the bug")

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha1}, StartSHA: start},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha2}},
			3: {Slug: "batch3", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha3}},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-intfail",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-intfail",
			RunDir:    "/run/dir/intfail",
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
					{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
					{TranscriptPath: "/transcripts/fork3.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			reportPath := websterengine.IntegrationReportPath(fx.Deps.Geom.ReportsDir)
			if err := os.WriteFile(reportPath, []byte("status: FAILED\nhead_sha: "+sha3+"\ndeviations: []\n"), 0o644); err != nil {
				t.Fatalf("write integration report: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: stuck\nstuck_reason: \"integration suite failed\"\nbatches_done: 3\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Batches shipped\n\nAll three batches landed; integration failed.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-intfail", "master-session-intfail")

	if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{}); err != nil {
		t.Fatalf("Run() error = %v; want nil (a FAILED integration report is escalated, not a Run() error)", err)
	}

	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	escalated, ok := st.Batches[-1]
	if !ok || escalated == nil {
		t.Fatalf("state.json carries no integration escalation record; want one at the reserved key")
	}
	if escalated.Slug != "03-batch3" {
		t.Errorf("escalated record Slug = %q; want %q (the localized offending card)", escalated.Slug, "03-batch3")
	}
	if escalated.Digest == nil || escalated.Digest.HeadSHA != sha3 {
		t.Errorf("escalated record digest = %+v; want head_sha %q", escalated.Digest, sha3)
	}

	summaryData, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if !strings.Contains(string(summaryData), "03-batch3") {
		t.Errorf("summary.md does not name the localized offending card; got:\n%s", summaryData)
	}

	branch := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "symbolic-ref", "--short", "HEAD"))
	if branch != originalBranch {
		t.Errorf("HEAD branch after bisect = %q; want restored to %q", branch, originalBranch)
	}
}

// TestBisectAndEscalate_EmptySHAsDegradesGracefully proves bisect's other edge shape — a genuinely
// empty shas slice, distinct from the already-covered single-SHA "sole/HEAD card" degrade above —
// also degrades gracefully rather than erroring: BisectAndEscalate still records a terminal
// escalation and extends summary.md, falling back to "unknown" for both the offending card and SHA
// instead of indexing into the empty slice.
// bisect returns before ever touching repo when shas is empty, so this needs no real git repo — a
// nil FabricBisector is never dereferenced.
func TestBisectAndEscalate_EmptySHAsDegradesGracefully(t *testing.T) {
	websterDir := t.TempDir()
	summaryPath := summaryparser.Path(websterDir)
	if err := os.WriteFile(summaryPath, []byte("# Batches shipped\n"), 0o644); err != nil {
		t.Fatalf("seed summary.md: %v", err)
	}

	st := &websterengine.State{}
	if err := websterengine.BisectAndEscalate(nil, nil, nil, "true", "/unused", websterDir, st, nil); err != nil {
		t.Fatalf("BisectAndEscalate() with empty shas error = %v; want nil (graceful degrade, not a hard error)", err)
	}

	escalated, ok := st.Batches[-1]
	if !ok || escalated == nil {
		t.Fatalf("state carries no integration escalation record; want one at the reserved key")
	}
	if escalated.Slug != "unknown" {
		t.Errorf("escalated record Slug = %q; want %q (no card to localize with empty shas)", escalated.Slug, "unknown")
	}

	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if !strings.Contains(string(summaryData), "unknown") {
		t.Errorf("summary.md does not name the fallback \"unknown\" card; got:\n%s", summaryData)
	}
}

// TestIntegrationStage_MissingReport_StuckOutcomePreserved proves the outcome-aware missing-report
// rule: a plan with a "## verify:" section, every batch terminal-done, Master reporting stuck, and
// NO integration report on disk is a CONSISTENT state (the integration fork died, or Master stuck
// out before the stage) — Run returns Master's own graceful stuck judgment rather than overwriting
// it with a missing-report error (crucible round fable-r1: the pre-fix backstop turned exactly this
// graceful stuck into a run ERROR after a real 30s wait).
func TestIntegrationStage_MissingReport_StuckOutcomePreserved(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "true")
	fx.Deps.Clock = &recoverFakeClock{now: time.Unix(0, 0)}

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{"deadbeef"}},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-intmiss",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-intmiss",
			RunDir:    "/run/dir/intmiss",
			ForkAudit: &shuttleengine.ForkAudit{},
		},
		onWait: func() {
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: stuck\nstuck_reason: \"integration fork died\"\nbatches_done: 1\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Batches shipped\n\nIntegration fork died.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-intmiss", "master-session-intmiss")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want Master's own stuck outcome preserved", err)
	}
	if result.Outcome != "stuck" {
		t.Errorf("RunResult.Outcome = %q; want %q", result.Outcome, "stuck")
	}
}

// TestIntegrationStage_MissingReport_DoneOutcomeFailsLoud proves the other half of the
// outcome-aware rule: outcome: done CLAIMS a passing integration suite, so a missing integration
// report under a done outcome is a genuine inconsistency Run fails loud on.
func TestIntegrationStage_MissingReport_DoneOutcomeFailsLoud(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "true")
	fx.Deps.Clock = &recoverFakeClock{now: time.Unix(0, 0)}

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{"deadbeef"}},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-intmiss2",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-intmiss2",
			RunDir:    "/run/dir/intmiss2",
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Batches shipped\n\nAll good.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-intmiss2", "master-session-intmiss2")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatal("Run() = nil error for outcome: done with no integration report; want the fail-loud inconsistency error")
	}
	if !strings.Contains(err.Error(), "integration report never landed") {
		t.Errorf("Run() error = %q; want the missing-integration-report inconsistency named", err.Error())
	}
}

// countingBisector wraps a FabricBisector and counts its detached checkouts,
// so a test can tell whether triage's baseline run or the localizing bisect ever moved the worktree.
type countingBisector struct {
	websterengine.FabricBisector
	checkouts int
}

func (c *countingBisector) CheckoutDetached(sha string) error {
	c.checkouts++
	return c.FabricBisector.CheckoutDetached(sha)
}

// seedVerifyScripts commits the three verify scripts the triage tests run, so they exist at every SHA the run checks out.
// verify.sh fails with a go test-shaped TestBad failure whenever bad.marker exists;
// always.sh always fails with a TestAlways failure;
// dirty.sh does the same after dirtying the tree with an untracked file and an uncommitted change to the tracked base.txt.
func seedVerifyScripts(t *testing.T, worktree string) {
	t.Helper()
	failure := func(test string) string {
		return "printf -- '--- FAIL: " + test + " (0.00s)\\n    x_test.go:1: " + test + " failed\\nFAIL\\nFAIL\\texample/pkg\\t0.01s\\n'\n"
	}
	gitkit.CommitFile(t, worktree, "verify.sh", "if [ -f bad.marker ]; then\n"+failure("TestBad")+"exit 1\nfi\n", "verify script")
	gitkit.CommitFile(t, worktree, "always.sh", failure("TestAlways")+"exit 1\n", "always-failing script")
	gitkit.CommitFile(t, worktree, "dirty.sh", "echo dirty >> base.txt\necho x > untracked.txt\n"+failure("TestAlways")+"exit 1\n", "dirtying script")
}

// failedSuite scripts one run whose integration fork reports FAILED.
type failedSuite struct {
	// batches is each batch's CardSHAs; its length must equal the fixture's card count.
	batches [][]string
	// startSHA is batch 1's recorded StartSHA.
	startSHA string
	// laterStartSHAs maps a batch number above 1 to its recorded StartSHA; unlisted batches record none.
	laterStartSHAs map[int]string
	// masterOutcome is "done" or "stuck".
	masterOutcome string
	// forkLog is the fork's captured first-run log; empty writes none.
	forkLog string
	// fixer scripts the integration-fix strand; nil installs one that reports FAILED.
	fixer *fakeFixStarter
	// noFixStarter leaves RunDeps.FixStarter nil.
	noFixStarter bool
	// integrationFix seeds state's IntegrationFix record.
	integrationFix *websterengine.IntegrationFixState
	// reportStatus is the integration report's status; empty writes FAILED.
	reportStatus string
}

// fakeFixStarter is a hermetic websterengine.FixStarter double.
// The strand's own turn runs from the handle's Wait: work does whatever the strand does to the worktree and returns the status and head_sha it reports.
type fakeFixStarter struct {
	t        *testing.T
	worktree string
	calls    int
	startErr error
	// onStart runs after a successful start, before the handle is returned.
	onStart func(t *testing.T)
	// outcome is the strand's shuttle outcome; empty means done.
	outcome shuttleengine.Outcome
	// work is the strand's turn, run only when the outcome is done; nil reports FAILED at HEAD.
	work func(t *testing.T) (status, head string)
}

func newFakeFixStarter(t *testing.T, worktree string) *fakeFixStarter {
	return &fakeFixStarter{t: t, worktree: worktree}
}

func (f *fakeFixStarter) StartFix(spec shuttleengine.Spec) (websterengine.MasterHandle, error) {
	f.calls++
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.onStart != nil {
		f.onStart(f.t)
	}
	outcome := f.outcome
	if outcome == "" {
		outcome = shuttleengine.OutcomeDone
	}
	return &runFakeHandle{
		strandGUID: "fix-strand",
		result:     shuttleengine.Result{Outcome: outcome},
		onWait: func() {
			if outcome != shuttleengine.OutcomeDone {
				return
			}
			status, head := websterengine.ReportStatusFailed, strings.TrimSpace(gitkit.Git(f.t, f.worktree, "rev-parse", "HEAD"))
			if f.work != nil {
				status, head = f.work(f.t)
			}
			report := "status: " + status + "\nhead_sha: " + head + "\ndeviations: []\n"
			if err := os.WriteFile(spec.OutputFiles[0], []byte(report), 0o644); err != nil {
				f.t.Fatalf("write fix report: %v", err)
			}
		},
	}, nil
}

var _ websterengine.FixStarter = (*fakeFixStarter)(nil)

// runFailedSuite seeds fx's state from s, scripts Master and the integration fork from onWait, and returns Run's result.
func runFailedSuite(t *testing.T, fx *runFixture, s failedSuite) (websterengine.RunResult, error) {
	t.Helper()
	n := len(s.batches)

	st := &websterengine.State{Batches: map[int]*websterengine.BatchState{}}
	var forks []shuttleengine.ForkReport
	for i, shas := range s.batches {
		st.Batches[i+1] = &websterengine.BatchState{
			Slug: fmt.Sprintf("batch%d", i+1), Kind: "fork", Terminal: true, Status: "done", CardSHAs: shas,
		}
		forks = append(forks, shuttleengine.ForkReport{TranscriptPath: fmt.Sprintf("/transcripts/fork%d.jsonl", i+1), ReportReturned: true})
	}
	st.Batches[1].StartSHA = s.startSHA
	for number, sha := range s.laterStartSHAs {
		st.Batches[number].StartSHA = sha
	}
	st.IntegrationFix = s.integrationFix
	seedMatchingState(t, fx, st)

	if !s.noFixStarter {
		fixer := s.fixer
		if fixer == nil {
			fixer = newFakeFixStarter(t, fx.Worktree)
		}
		fx.Deps.FixStarter = fixer
	}
	reportStatus := s.reportStatus
	if reportStatus == "" {
		reportStatus = websterengine.ReportStatusFailed
	}

	head := gitkit.RevParse(t, fx.Worktree, "HEAD")
	outcome := "outcome: done\nstuck_reason: null\n"
	if s.masterOutcome == "stuck" {
		outcome = "outcome: stuck\nstuck_reason: \"master says stuck\"\n"
	}
	outcome += fmt.Sprintf("batches_done: %d\n", n)

	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-triage",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-triage",
			RunDir:    "/run/dir/triage",
			ForkAudit: &shuttleengine.ForkAudit{Forks: forks},
		},
		onWait: func() {
			write := func(path, content string) {
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}
			if s.forkLog != "" {
				write(websterengine.IntegrationLogPath(fx.Deps.Geom.ScratchDir), s.forkLog)
			}
			write(websterengine.IntegrationReportPath(fx.Deps.Geom.ReportsDir), "status: "+reportStatus+"\nhead_sha: "+head+"\ndeviations: []\n")
			write(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), outcome)
			write(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), "# Batches shipped\n\nAll batches landed.\n")
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-triage", "master-session-triage")

	return websterengine.Run(fx.Deps, websterengine.RunOptions{})
}

// integrationReportOf parses the run's integration report.
func integrationReportOf(t *testing.T, fx *runFixture) *websterengine.IntegrationReport {
	t.Helper()
	r, err := websterengine.ParseIntegrationReport(websterengine.IntegrationReportPath(fx.Deps.Geom.ReportsDir))
	if err != nil {
		t.Fatalf("ParseIntegrationReport() error = %v", err)
	}
	return r
}

// summaryOf reads the run's summary.md.
func summaryOf(t *testing.T, fx *runFixture) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	return string(data)
}

// hasEscalationRecord reports whether state.json carries the reserved -1 integration record.
func hasEscalationRecord(t *testing.T, fx *runFixture) bool {
	t.Helper()
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	rec, ok := st.Batches[-1]
	return ok && rec != nil
}

// assertOnBranch fails unless the worktree is back on branch.
func assertOnBranch(t *testing.T, fx *runFixture, branch string) {
	t.Helper()
	if got := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "symbolic-ref", "--short", "HEAD")); got != branch {
		t.Errorf("HEAD branch = %q; want restored to %q", got, branch)
	}
}

const flakyForkLog = "--- FAIL: TestFlaky (0.00s)\n    flaky_test.go:1: intermittent boom\nFAIL\nFAIL\texample/pkg\t0.01s\n"

// TestIntegrationStage_Flaky_DoneKeepsDone proves a verify that fails once and passes on rerun under Master done ends done:
// the report, summary, warnings and friction note record the flaky identity, nothing is bisected, and no escalation record exists.
func TestIntegrationStage_Flaky_DoneKeepsDone(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "true")
	fx.Deps.FrictionDir = t.TempDir()
	bis := &countingBisector{FabricBisector: gitrepo.New(fx.Worktree)}
	fx.Deps.OpenBisector = func() (websterengine.FabricBisector, error) { return bis, nil }

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")

	result, err := runFailedSuite(t, fx, failedSuite{batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "done", forkLog: flakyForkLog})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("Outcome = %q; want done", result.Outcome)
	}

	report := integrationReportOf(t, fx)
	if report.Triage == nil || report.Triage.Verdict != websterengine.TriageVerdictFlaky {
		t.Fatalf("report triage = %+v; want verdict flaky", report.Triage)
	}
	if len(report.Failures) != 1 || report.Failures[0].ID != "example/pkg.TestFlaky" || !strings.Contains(report.Failures[0].Tail, "intermittent boom") {
		t.Errorf("report failures = %+v; want the first-run TestFlaky identity with its tail", report.Failures)
	}
	if !strings.Contains(summaryOf(t, fx), "## Integration suite triage") {
		t.Errorf("summary.md carries no triage section; got:\n%s", summaryOf(t, fx))
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "example/pkg.TestFlaky") {
		t.Errorf("Warnings = %v; want one naming the flaky identity", result.Warnings)
	}
	// NotePath allocates the next free name, so a written note shows up as the file it skips.
	if matches, _ := filepath.Glob(filepath.Join(fx.Deps.FrictionDir, "webster-verify-triage*")); len(matches) != 1 {
		t.Errorf("friction notes in %s = %v; want exactly one", fx.Deps.FrictionDir, matches)
	}
	if bis.checkouts != 0 {
		t.Errorf("detached checkouts = %d; want 0 (a flaky verdict never bisects)", bis.checkouts)
	}
	if hasEscalationRecord(t, fx) {
		t.Errorf("escalation record present for a flaky verdict; want none")
	}
}

// TestIntegrationStage_Flaky_NoFrictionNoteWithoutDir proves the friction note is absent when FrictionDir is empty.
func TestIntegrationStage_Flaky_NoFrictionNoteWithoutDir(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "true")
	fx.Deps.FrictionDir = ""

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")

	if _, err := runFailedSuite(t, fx, failedSuite{batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "done", forkLog: flakyForkLog}); err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if got := friction.NotePath("", "webster-verify-triage"); got != "" {
		t.Errorf("NotePath with an empty dir = %q; want empty", got)
	}
}

// TestIntegrationStage_Flaky_FrictionNoteFailureKeepsDone proves an unwritable friction dir does not fail the run:
// the note is best-effort, and the report and summary still record the flaky verdict.
func TestIntegrationStage_Flaky_FrictionNoteFailureKeepsDone(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "true")
	// A regular file where the friction dir should be makes every note write fail.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	fx.Deps.FrictionDir = blocker

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")

	result, err := runFailedSuite(t, fx, failedSuite{batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "done", forkLog: flakyForkLog})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a failed friction note is not a run failure)", err)
	}
	if result.Outcome != "done" {
		t.Errorf("Outcome = %q; want done", result.Outcome)
	}
	if report := integrationReportOf(t, fx); report.Triage == nil || report.Triage.Verdict != websterengine.TriageVerdictFlaky {
		t.Errorf("report triage = %+v; want verdict flaky", report.Triage)
	}
	if !strings.Contains(summaryOf(t, fx), "## Integration suite triage") {
		t.Errorf("summary.md carries no triage section; got:\n%s", summaryOf(t, fx))
	}
}

// TestIntegrationStage_PreExisting_DoneKeepsDone proves a failure present at both head and the plan's starting commit ends done with a pre-existing verdict,
// and only the baseline run checks anything out.
func TestIntegrationStage_PreExisting_DoneKeepsDone(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "sh always.sh")
	seedVerifyScripts(t, fx.Worktree)
	bis := &countingBisector{FabricBisector: gitrepo.New(fx.Worktree)}
	fx.Deps.OpenBisector = func() (websterengine.FabricBisector, error) { return bis, nil }
	branch := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "symbolic-ref", "--short", "HEAD"))

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")

	result, err := runFailedSuite(t, fx, failedSuite{
		batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "done",
		forkLog: "--- FAIL: TestAlways (0.00s)\n    x_test.go:1: TestAlways failed\nFAIL\nFAIL\texample/pkg\t0.01s\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("Outcome = %q; want done", result.Outcome)
	}
	report := integrationReportOf(t, fx)
	if report.Triage == nil || report.Triage.Verdict != websterengine.TriageVerdictPreExisting {
		t.Fatalf("report triage = %+v; want verdict pre-existing", report.Triage)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "pre-existing") {
		t.Errorf("Warnings = %v; want a pre-existing warning", result.Warnings)
	}
	if !strings.Contains(summaryOf(t, fx), "## Integration suite triage") {
		t.Errorf("summary.md carries no triage section")
	}
	if bis.checkouts != 1 {
		t.Errorf("detached checkouts = %d; want 1 (the baseline run only, no bisect)", bis.checkouts)
	}
	if hasEscalationRecord(t, fx) {
		t.Errorf("escalation record present for a pre-existing verdict; want none")
	}
	assertOnBranch(t, fx, branch)
}

// TestIntegrationStage_Regression_DemotesDone proves a failure at head that passes at the starting commit demotes Master's done to stuck, localizes the card, writes the -1 record, and puts the failing test and its tail in summary.md.
func TestIntegrationStage_Regression_DemotesDone(t *testing.T) {
	fx := newRunFixture(t, 3)
	appendIntegrationVerify(t, fx.PlanDir, "sh verify.sh")
	seedVerifyScripts(t, fx.Worktree)
	branch := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "symbolic-ref", "--short", "HEAD"))

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")
	sha2 := gitkit.CommitFile(t, fx.Worktree, "card2.txt", "two", "card2")
	sha3 := gitkit.CommitFile(t, fx.Worktree, "bad.marker", "bad", "card3 introduces the bug")

	result, err := runFailedSuite(t, fx, failedSuite{
		batches: [][]string{{sha1}, {sha2}, {sha3}}, startSHA: start, masterOutcome: "done",
		forkLog: "--- FAIL: TestBad (0.00s)\n    x_test.go:1: TestBad failed\nFAIL\nFAIL\texample/pkg\t0.01s\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a demotion is a result, not an error)", err)
	}
	if result.Outcome != "stuck" {
		t.Fatalf("Outcome = %q; want stuck", result.Outcome)
	}
	for _, want := range []string{"example/pkg.TestBad", "03-batch3"} {
		if !strings.Contains(result.StuckReason, want) {
			t.Errorf("StuckReason = %q; want it to name %q", result.StuckReason, want)
		}
	}
	if result.BatchesDone != 3 || result.SummaryTitle == "" {
		t.Errorf("BatchesDone = %d, SummaryTitle = %q; want Master's own 3 and a title", result.BatchesDone, result.SummaryTitle)
	}
	if report := integrationReportOf(t, fx); report.Triage == nil || report.Triage.Verdict != websterengine.TriageVerdictRegression {
		t.Errorf("report triage = %+v; want verdict regression", report.Triage)
	}
	if !hasEscalationRecord(t, fx) {
		t.Errorf("no -1 escalation record after a regression")
	}
	summary := summaryOf(t, fx)
	if !strings.Contains(summary, "example/pkg.TestBad") || !strings.Contains(summary, "TestBad failed") {
		t.Errorf("summary.md does not name the regressing test with its tail; got:\n%s", summary)
	}
	assertOnBranch(t, fx, branch)
}

// TestIntegrationStage_MasterStuck_KeepsStuckAndGetsTriage proves a Master stuck over a FAILED report with a flaky verdict stays stuck with its own reason,
// and the report and summary still carry the triage.
func TestIntegrationStage_MasterStuck_KeepsStuckAndGetsTriage(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "true")

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")

	result, err := runFailedSuite(t, fx, failedSuite{batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "stuck", forkLog: flakyForkLog})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "stuck" || result.StuckReason != "master says stuck" {
		t.Errorf("Outcome = %q, StuckReason = %q; want Master's own stuck and reason", result.Outcome, result.StuckReason)
	}
	if report := integrationReportOf(t, fx); report.Triage == nil || report.Triage.Verdict != websterengine.TriageVerdictFlaky {
		t.Errorf("report triage = %+v; want verdict flaky", report.Triage)
	}
	if !strings.Contains(summaryOf(t, fx), "## Integration suite triage") {
		t.Errorf("summary.md carries no triage section")
	}
}

// TestIntegrationStage_BaselineIsBatchOneStartSHA proves the baseline is the HEAD before batch 1 began, not the commit before its last card:
// batch 1 holds two commits and the first breaks a test, so the test is a regression.
func TestIntegrationStage_BaselineIsBatchOneStartSHA(t *testing.T) {
	fx := newRunFixture(t, 2)
	appendIntegrationVerify(t, fx.PlanDir, "sh verify.sh")
	seedVerifyScripts(t, fx.Worktree)

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	c1 := gitkit.CommitFile(t, fx.Worktree, "bad.marker", "bad", "batch 1 first commit breaks a test")
	c2 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "batch 1 second commit")
	c3 := gitkit.CommitFile(t, fx.Worktree, "card2.txt", "two", "batch 2")

	result, err := runFailedSuite(t, fx, failedSuite{
		batches: [][]string{{c1, c2}, {c3}}, startSHA: start, masterOutcome: "done",
		forkLog: "--- FAIL: TestBad (0.00s)\n    x_test.go:1: TestBad failed\nFAIL\nFAIL\texample/pkg\t0.01s\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "stuck" {
		t.Errorf("Outcome = %q; want stuck (TestBad passes at the starting commit)", result.Outcome)
	}
	report := integrationReportOf(t, fx)
	if report.Triage == nil || report.Triage.Verdict != websterengine.TriageVerdictRegression || report.Triage.BaselineSHA != start {
		t.Errorf("report triage = %+v; want a regression against baseline %s", report.Triage, start)
	}
}

// TestIntegrationStage_BaselineIsEarliestStartSHA proves the baseline is the earliest batch start commit even when batch 2 began before batch 1:
// batch 2's commit breaks a test and batch 1 began on top of it, so batch 1's own start commit already fails that test,
// yet the failure is a regression against batch 2's earlier start commit.
func TestIntegrationStage_BaselineIsEarliestStartSHA(t *testing.T) {
	fx := newRunFixture(t, 2)
	appendIntegrationVerify(t, fx.PlanDir, "sh verify.sh")
	seedVerifyScripts(t, fx.Worktree)

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	c2 := gitkit.CommitFile(t, fx.Worktree, "bad.marker", "bad", "batch 2, begun first, breaks a test")
	c1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "batch 1, begun on top of batch 2")

	result, err := runFailedSuite(t, fx, failedSuite{
		batches: [][]string{{c1}, {c2}}, startSHA: c2, laterStartSHAs: map[int]string{2: start}, masterOutcome: "done",
		forkLog: "--- FAIL: TestBad (0.00s)\n    x_test.go:1: TestBad failed\nFAIL\nFAIL\texample/pkg\t0.01s\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "stuck" {
		t.Errorf("Outcome = %q; want stuck (TestBad passes at the earliest start commit)", result.Outcome)
	}
	report := integrationReportOf(t, fx)
	if report.Triage == nil || report.Triage.Verdict != websterengine.TriageVerdictRegression || report.Triage.BaselineSHA != start {
		t.Errorf("report triage = %+v; want a regression against the earliest start commit %s", report.Triage, start)
	}
}

// TestIntegrationStage_DirtyBaselineRunRestoresBranch proves the worktree is back on its branch after a baseline verify that leaves an untracked file and an uncommitted change to a tracked file that is identical at baseline and head.
func TestIntegrationStage_DirtyBaselineRunRestoresBranch(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "sh dirty.sh")
	seedVerifyScripts(t, fx.Worktree)
	branch := strings.TrimSpace(gitkit.Git(t, fx.Worktree, "symbolic-ref", "--short", "HEAD"))

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")

	result, err := runFailedSuite(t, fx, failedSuite{batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "done"})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("Outcome = %q; want done (the failure is pre-existing)", result.Outcome)
	}
	assertOnBranch(t, fx, branch)
}

// TestIntegrationStage_MissingForkLogDoesNotError proves a missing fork log is not an error and the identities come from the rerun.
func TestIntegrationStage_MissingForkLogDoesNotError(t *testing.T) {
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "sh always.sh")
	seedVerifyScripts(t, fx.Worktree)

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")

	if _, err := runFailedSuite(t, fx, failedSuite{batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "done"}); err != nil {
		t.Fatalf("Run() error = %v; want nil for a missing fork log", err)
	}
	report := integrationReportOf(t, fx)
	if len(report.Failures) != 1 || report.Failures[0].ID != "example/pkg.TestAlways" {
		t.Errorf("report failures = %+v; want the rerun's TestAlways identity", report.Failures)
	}
}

// TestBisectAndEscalate_UnattributableFailureBlamesNoCard covers the round-4 review's R4-17. The
// binary search converges on the LAST index whenever every SHA it actually tested passed, so it used
// to blame the last card by arithmetic rather than by evidence — and AppendIntegrationFailure then
// wrote "SHA-bisect localized the failure to card X" into summary.md, which is the PR text.
//
// Here the verify command passes at every recorded SHA (the integration failure came from somewhere
// the card SHAs do not capture: the tree state after the last card, the environment, or the verify
// command itself). The honest answer is the "unknown" offender the empty-shas path already reports.
func TestBisectAndEscalate_UnattributableFailureBlamesNoCard(t *testing.T) {
	worktree := newScratchRepo(t)
	sha1 := gitkit.CommitFile(t, worktree, "card1.txt", "one", "card1")
	sha2 := gitkit.CommitFile(t, worktree, "card2.txt", "two", "card2")
	sha3 := gitkit.CommitFile(t, worktree, "card3.txt", "three", "card3")
	originalBranch := strings.TrimSpace(gitkit.Git(t, worktree, "symbolic-ref", "--short", "HEAD"))

	websterDir := t.TempDir()
	if err := os.WriteFile(summaryparser.Path(websterDir), []byte("# Batches shipped\n"), 0o644); err != nil {
		t.Fatalf("seed summary.md: %v", err)
	}

	st := &websterengine.State{}
	shas := []string{sha1, sha2, sha3}
	labels := []string{"01-batch1", "02-batch2", "03-batch3"}
	// "true" passes at every SHA, so no recorded card SHA implicates itself.
	if err := websterengine.BisectAndEscalate(gitrepo.New(worktree), shas, labels, "true", worktree, websterDir, st, nil); err != nil {
		t.Fatalf("BisectAndEscalate() error = %v; want nil", err)
	}

	escalated, ok := st.Batches[-1]
	if !ok || escalated == nil {
		t.Fatalf("state carries no integration escalation record; want one at the reserved key")
	}
	if escalated.Slug != "unknown" {
		t.Errorf("escalated record Slug = %q; want %q — no recorded card SHA failed, so none may be named", escalated.Slug, "unknown")
	}

	branch := strings.TrimSpace(gitkit.Git(t, worktree, "symbolic-ref", "--short", "HEAD"))
	if branch != originalBranch {
		t.Errorf("HEAD branch after bisect = %q; want restored to %q", branch, originalBranch)
	}
}

const badForkLog = "--- FAIL: TestBad (0.00s)\n    x_test.go:1: TestBad failed\nFAIL\nFAIL\texample/pkg\t0.01s\n"

// regressionScene is a three-card run whose third card introduces the failure verify.sh reports.
type regressionScene struct {
	fx    *runFixture
	suite failedSuite
	start string
	shas  []string
}

// newRegressionScene builds the scene.
// planInWorktree commits a copy of the plan under the worktree's plan/ directory and points the run at it, so a strand commit can touch the plan.
func newRegressionScene(t *testing.T, planInWorktree bool) *regressionScene {
	t.Helper()
	fx := newRunFixture(t, 3)
	appendIntegrationVerify(t, fx.PlanDir, "sh verify.sh")
	if planInWorktree {
		entries, err := os.ReadDir(fx.PlanDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(fx.PlanDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			gitkit.CommitFile(t, fx.Worktree, filepath.Join("plan", e.Name()), string(data), "plan "+e.Name())
		}
		fx.PlanDir = filepath.Join(fx.Worktree, "plan")
		fx.Deps.Geom.PlanDir = fx.PlanDir
	}
	seedVerifyScripts(t, fx.Worktree)

	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	shas := []string{
		gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1"),
		gitkit.CommitFile(t, fx.Worktree, "card2.txt", "two", "card2"),
		gitkit.CommitFile(t, fx.Worktree, "bad.marker", "bad", "card3 introduces the bug"),
	}
	return &regressionScene{
		fx:    fx,
		start: start,
		shas:  shas,
		suite: failedSuite{batches: [][]string{{shas[0]}, {shas[1]}, {shas[2]}}, startSHA: start, masterOutcome: "done", forkLog: badForkLog},
	}
}

// commitFix commits the removal of bad.marker, the fix for the scene's regression, and returns the commit.
func commitFix(t *testing.T, worktree string) string {
	t.Helper()
	gitkit.Git(t, worktree, "rm", "-q", "bad.marker")
	gitkit.Git(t, worktree, "commit", "-m", "fix the regression")
	return strings.TrimSpace(gitkit.Git(t, worktree, "rev-parse", "HEAD"))
}

// fixerReporting returns a fix strand that runs work and reports OK at the head work returns.
func fixerReporting(t *testing.T, worktree string, work func(t *testing.T) string) *fakeFixStarter {
	f := newFakeFixStarter(t, worktree)
	f.work = func(t *testing.T) (string, string) { return websterengine.ReportStatusOK, work(t) }
	return f
}

// stateOf loads the run's state.json.
func stateOf(t *testing.T, fx *runFixture) *websterengine.State {
	t.Helper()
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	return st
}

// assertFixWayForward fails unless reason names preFix, every commit, the reset, the verify-and-commit step and the re-run.
func assertFixWayForward(t *testing.T, reason, preFix string, commits ...string) {
	t.Helper()
	wants := append([]string{preFix, "git reset --hard " + preFix, "## verify:", "commit", "lyx webster run"}, commits...)
	for _, want := range wants {
		if !strings.Contains(reason, want) {
			t.Errorf("StuckReason = %q; want it to name %q", reason, want)
		}
	}
}

// TestIntegrationStage_FixAttempt_Regression332b_FixedKeepsDone is the #332b scene:
// a regression bisected to card 3, a strand that commits the fix, and a re-run triage that passes.
// The run stays done with no -1 record, and summary.md and the report record the fix commit and the cleared identity.
func TestIntegrationStage_FixAttempt_Regression332b_FixedKeepsDone(t *testing.T) {
	sc := newRegressionScene(t, false)
	var fixCommit string
	fixer := fixerReporting(t, sc.fx.Worktree, func(t *testing.T) string {
		fixCommit = commitFix(t, sc.fx.Worktree)
		return fixCommit
	})
	sc.suite.fixer = fixer

	result, err := runFailedSuite(t, sc.fx, sc.suite)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Fatalf("Outcome = %q, StuckReason = %q; want done", result.Outcome, result.StuckReason)
	}
	if fixer.calls != 1 {
		t.Errorf("StartFix calls = %d; want 1", fixer.calls)
	}
	if hasEscalationRecord(t, sc.fx) {
		t.Errorf("-1 escalation record present after a fixed regression; want none")
	}

	report := integrationReportOf(t, sc.fx)
	if report.Triage == nil || report.Triage.Verdict != websterengine.TriageVerdictRegression {
		t.Errorf("report triage = %+v; want the pre-fix regression verdict kept", report.Triage)
	}
	if report.Fix == nil || report.Fix.Result != websterengine.FixResultFixed || report.Fix.PreFixHead != sc.shas[2] ||
		len(report.Fix.Commits) != 1 || report.Fix.Commits[0] != fixCommit || len(report.Fix.Cleared) != 1 || report.Fix.Cleared[0] != "example/pkg.TestBad" {
		t.Errorf("report fix = %+v; want fixed from %s with commit %s and TestBad cleared", report.Fix, sc.shas[2], fixCommit)
	}
	summary := summaryOf(t, sc.fx)
	for _, want := range []string{"## Integration suite fix", fixCommit, "example/pkg.TestBad"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary.md does not contain %q; got:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "## Integration suite failed") {
		t.Errorf("summary.md carries a failure section after a fixed regression")
	}
	if fix := stateOf(t, sc.fx).IntegrationFix; fix == nil || fix.Result != websterengine.FixResultFixed || fix.StrandGUID != "fix-strand" {
		t.Errorf("state IntegrationFix = %+v; want result fixed and the strand recorded", fix)
	}
}

// TestIntegrationStage_FixAttempt_UnfixedEscalates proves each way the strand can fail to fix the regression escalates exactly as before, plus the attempt's record:
// the -1 record, outcome stuck, and summary.md and the report recording the attempt.
func TestIntegrationStage_FixAttempt_UnfixedEscalates(t *testing.T) {
	cases := []struct {
		name       string
		result     string
		build      func(t *testing.T, sc *regressionScene) *fakeFixStarter
		wantCommit bool
	}{
		{
			name:   "strand reports FAILED",
			result: websterengine.FixResultFailed,
			build:  func(t *testing.T, sc *regressionScene) *fakeFixStarter { return newFakeFixStarter(t, sc.fx.Worktree) },
		},
		{
			name:   "strand times out",
			result: websterengine.FixResultTimeout,
			build: func(t *testing.T, sc *regressionScene) *fakeFixStarter {
				f := newFakeFixStarter(t, sc.fx.Worktree)
				f.outcome = shuttleengine.OutcomeTimeout
				return f
			},
		},
		{
			name:   "strand start fails",
			result: websterengine.FixResultFailed,
			build: func(t *testing.T, sc *regressionScene) *fakeFixStarter {
				f := newFakeFixStarter(t, sc.fx.Worktree)
				f.startErr = fmt.Errorf("provider never came up")
				return f
			},
		},
		{
			name:       "strand commit leaves the regression",
			result:     websterengine.FixResultFailed,
			wantCommit: true,
			build: func(t *testing.T, sc *regressionScene) *fakeFixStarter {
				return fixerReporting(t, sc.fx.Worktree, func(t *testing.T) string {
					return gitkit.CommitFile(t, sc.fx.Worktree, "unrelated.txt", "x", "unrelated change")
				})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := newRegressionScene(t, false)
			sc.suite.fixer = tc.build(t, sc)

			result, err := runFailedSuite(t, sc.fx, sc.suite)
			if err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}
			if result.Outcome != "stuck" {
				t.Fatalf("Outcome = %q; want stuck", result.Outcome)
			}
			if !hasEscalationRecord(t, sc.fx) {
				t.Errorf("no -1 escalation record after an unfixed regression")
			}
			report := integrationReportOf(t, sc.fx)
			if report.Fix == nil || report.Fix.Result != tc.result || report.Fix.PreFixHead != sc.shas[2] {
				t.Fatalf("report fix = %+v; want result %q from %s", report.Fix, tc.result, sc.shas[2])
			}
			if !strings.Contains(summaryOf(t, sc.fx), "## Integration suite fix") {
				t.Errorf("summary.md does not record the attempt")
			}
			if fix := stateOf(t, sc.fx).IntegrationFix; fix == nil || fix.Result != tc.result {
				t.Errorf("state IntegrationFix = %+v; want result %q", fix, tc.result)
			}
			var commits []string
			if tc.wantCommit {
				commits = report.Fix.Commits
				if len(commits) != 1 {
					t.Fatalf("report fix commits = %v; want the strand's one commit", commits)
				}
			}
			assertFixWayForward(t, result.StuckReason, sc.shas[2], commits...)
		})
	}
}

// TestIntegrationStage_FixAttempt_SpentEscalatesWithoutSpawn proves a state that already records the attempt escalates with no StartFix call,
// and the stuck reason names the recorded pre-fix head and every commit after it.
func TestIntegrationStage_FixAttempt_SpentEscalatesWithoutSpawn(t *testing.T) {
	sc := newRegressionScene(t, false)
	fixer := newFakeFixStarter(t, sc.fx.Worktree)
	sc.suite.fixer = fixer
	sc.suite.integrationFix = &websterengine.IntegrationFixState{PreFixHead: sc.shas[0], SpawnedAt: "2026-10-02T00:00:00Z", Result: websterengine.FixResultFailed}

	result, err := runFailedSuite(t, sc.fx, sc.suite)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "stuck" {
		t.Fatalf("Outcome = %q; want stuck", result.Outcome)
	}
	if fixer.calls != 0 {
		t.Errorf("StartFix calls = %d; want 0 for a spent attempt", fixer.calls)
	}
	if report := integrationReportOf(t, sc.fx); report.Fix == nil || report.Fix.Result != websterengine.FixResultSpent {
		t.Errorf("report fix = %+v; want result spent", report.Fix)
	}
	if !hasEscalationRecord(t, sc.fx) {
		t.Errorf("no -1 escalation record for a spent attempt")
	}
	assertFixWayForward(t, result.StuckReason, sc.shas[0], sc.shas[1], sc.shas[2])
}

// TestIntegrationStage_FixAttempt_RefusedCommitsEscalate proves a strand whose work checkFixCommits or the plan-fingerprint compare refuses fails the attempt with result refused.
func TestIntegrationStage_FixAttempt_RefusedCommitsEscalate(t *testing.T) {
	cases := []struct {
		name           string
		planInWorktree bool
		work           func(t *testing.T, sc *regressionScene) string
		wantDetail     string
	}{
		{
			name:           "commit touches the plan directory",
			planInWorktree: true,
			wantDetail:     "plan",
			work: func(t *testing.T, sc *regressionScene) string {
				card := filepath.Join(sc.fx.PlanDir, "01-batch1.md")
				data, err := os.ReadFile(card)
				if err != nil {
					t.Fatal(err)
				}
				return gitkit.CommitFile(t, sc.fx.Worktree, "plan/01-batch1.md", string(data)+"\nstrand note\n", "touch the plan")
			},
		},
		{
			name:       "commit touches _lyx",
			wantDetail: "_lyx",
			work: func(t *testing.T, sc *regressionScene) string {
				return gitkit.CommitFile(t, sc.fx.Worktree, "_lyx/notes.md", "x", "touch _lyx")
			},
		},
		{
			name:       "uncommitted plan edit left on disk",
			wantDetail: "changed the plan on disk",
			work: func(t *testing.T, sc *regressionScene) string {
				card := filepath.Join(sc.fx.PlanDir, "01-batch1.md")
				data, err := os.ReadFile(card)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(card, append(data, []byte("\nstrand note\n")...), 0o644); err != nil {
					t.Fatal(err)
				}
				return strings.TrimSpace(gitkit.Git(t, sc.fx.Worktree, "rev-parse", "HEAD"))
			},
		},
		{
			name:       "dirty worktree",
			wantDetail: "uncommitted",
			work: func(t *testing.T, sc *regressionScene) string {
				if err := os.WriteFile(filepath.Join(sc.fx.Worktree, "junk.txt"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				return strings.TrimSpace(gitkit.Git(t, sc.fx.Worktree, "rev-parse", "HEAD"))
			},
		},
		{
			name:       "HEAD moved past head_sha by a non-merge commit",
			wantDetail: "does not match the worktree's actual HEAD",
			work: func(t *testing.T, sc *regressionScene) string {
				commitFix(t, sc.fx.Worktree)
				return sc.shas[2]
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := newRegressionScene(t, tc.planInWorktree)
			sc.suite.fixer = fixerReporting(t, sc.fx.Worktree, func(t *testing.T) string { return tc.work(t, sc) })

			result, err := runFailedSuite(t, sc.fx, sc.suite)
			if err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}
			if result.Outcome != "stuck" {
				t.Fatalf("Outcome = %q; want stuck", result.Outcome)
			}
			report := integrationReportOf(t, sc.fx)
			if report.Fix == nil || report.Fix.Result != websterengine.FixResultRefused || !strings.Contains(report.Fix.Detail, tc.wantDetail) {
				t.Errorf("report fix = %+v; want result refused with a detail naming %q", report.Fix, tc.wantDetail)
			}
			if !hasEscalationRecord(t, sc.fx) {
				t.Errorf("no -1 escalation record after a refused fix")
			}
		})
	}
}

// TestIntegrationStage_FixAttempt_InFlightAttemptEndsStuck proves a state whose attempt the run's end interrupted ends stuck even over an OK integration report:
// summary.md names the commits after PreFixHead and the attempt's Result is set.
func TestIntegrationStage_FixAttempt_InFlightAttemptEndsStuck(t *testing.T) {
	sc := newRegressionScene(t, false)
	fixer := newFakeFixStarter(t, sc.fx.Worktree)
	sc.suite.fixer = fixer
	sc.suite.reportStatus = websterengine.ReportStatusOK
	sc.suite.integrationFix = &websterengine.IntegrationFixState{PreFixHead: sc.shas[0], SpawnedAt: "2026-10-02T00:00:00Z", StrandGUID: "gone"}

	result, err := runFailedSuite(t, sc.fx, sc.suite)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "stuck" {
		t.Fatalf("Outcome = %q; want stuck", result.Outcome)
	}
	assertFixWayForward(t, result.StuckReason, sc.shas[0], sc.shas[1], sc.shas[2])
	summary := summaryOf(t, sc.fx)
	for _, want := range []string{"## Integration suite fix", sc.shas[1], sc.shas[2]} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary.md does not contain %q; got:\n%s", want, summary)
		}
	}
	if fix := stateOf(t, sc.fx).IntegrationFix; fix == nil || fix.Result != websterengine.FixResultFailed {
		t.Errorf("state IntegrationFix = %+v; want Result set to failed", fix)
	}
	if fixer.calls != 0 {
		t.Errorf("StartFix calls = %d; want 0", fixer.calls)
	}
}

// TestIntegrationStage_FixAttempt_NilFixStarterIsWiringError proves a regression with no FixStarter returns the wiring error and records no attempt, so the fault never spends it.
func TestIntegrationStage_FixAttempt_NilFixStarterIsWiringError(t *testing.T) {
	sc := newRegressionScene(t, false)
	sc.suite.noFixStarter = true

	_, err := runFailedSuite(t, sc.fx, sc.suite)
	if err == nil || !strings.Contains(err.Error(), "FixStarter") {
		t.Fatalf("Run() error = %v; want the FixStarter wiring error", err)
	}
	if fix := stateOf(t, sc.fx).IntegrationFix; fix != nil {
		t.Errorf("state IntegrationFix = %+v; want unrecorded", fix)
	}
}

// TestIntegrationStage_FixAttempt_UnrecordedStrandIsRemoved proves a started fix strand whose GUID cannot be recorded is removed before the stage returns its error,
// since run entry's reclaim could never find it.
func TestIntegrationStage_FixAttempt_UnrecordedStrandIsRemoved(t *testing.T) {
	sc := newRegressionScene(t, false)
	fixer := newFakeFixStarter(t, sc.fx.Worktree)
	fixer.onStart = func(t *testing.T) {
		st := stateOf(t, sc.fx)
		st.IntegrationFix = nil
		if err := websterengine.SaveState(sc.fx.Deps.Geom.WebsterDir, sc.fx.Deps.Geom.ScratchDir, st); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}
		sc.fx.Reed.Strands = []reedengine.StrandStatus{{GUID: "fix-strand", Live: true}}
	}
	sc.suite.fixer = fixer

	_, err := runFailedSuite(t, sc.fx, sc.suite)
	if err == nil || !strings.Contains(err.Error(), "fix-strand") {
		t.Fatalf("Run() error = %v; want the record error naming the fix strand", err)
	}
	if len(sc.fx.Reed.RemovedGUIDs) != 1 || sc.fx.Reed.RemovedGUIDs[0] != "fix-strand" {
		t.Errorf("RemoveStrand calls = %v; want exactly [fix-strand]", sc.fx.Reed.RemovedGUIDs)
	}
}

// TestIntegrationStage_FixAttempt_PreSpawnFailureLeavesAttemptUnspent proves a failure before the spawn returns its error with no attempt recorded,
// so the next run still has its one attempt.
func TestIntegrationStage_FixAttempt_PreSpawnFailureLeavesAttemptUnspent(t *testing.T) {
	sc := newRegressionScene(t, false)
	fixer := newFakeFixStarter(t, sc.fx.Worktree)
	sc.suite.fixer = fixer
	blocker := filepath.Join(websterengine.IntegrationFixReportPath(sc.fx.Deps.Geom.ReportsDir), "occupied")
	if err := os.MkdirAll(blocker, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := runFailedSuite(t, sc.fx, sc.suite); err == nil || !strings.Contains(err.Error(), "stale integration fix report") {
		t.Fatalf("Run() error = %v; want the stale-report removal error", err)
	}
	if fix := stateOf(t, sc.fx).IntegrationFix; fix != nil {
		t.Errorf("state IntegrationFix = %+v; want unrecorded", fix)
	}
	if fixer.calls != 0 {
		t.Errorf("StartFix calls = %d; want 0", fixer.calls)
	}
}

// TestIntegrationStage_FixAttempt_NonRegressionVerdictsMakeNoAttempt proves flaky-only and pre-existing-only verdicts never spawn a fix strand.
func TestIntegrationStage_FixAttempt_NonRegressionVerdictsMakeNoAttempt(t *testing.T) {
	t.Run("flaky", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		appendIntegrationVerify(t, fx.PlanDir, "true")
		start := gitkit.RevParse(t, fx.Worktree, "HEAD")
		sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")
		fixer := newFakeFixStarter(t, fx.Worktree)

		if _, err := runFailedSuite(t, fx, failedSuite{batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "done", forkLog: flakyForkLog, fixer: fixer}); err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if fixer.calls != 0 {
			t.Errorf("StartFix calls = %d; want 0 for a flaky verdict", fixer.calls)
		}
	})
	t.Run("pre-existing", func(t *testing.T) {
		fx := newRunFixture(t, 1)
		appendIntegrationVerify(t, fx.PlanDir, "sh always.sh")
		seedVerifyScripts(t, fx.Worktree)
		start := gitkit.RevParse(t, fx.Worktree, "HEAD")
		sha1 := gitkit.CommitFile(t, fx.Worktree, "card1.txt", "one", "card1")
		fixer := newFakeFixStarter(t, fx.Worktree)

		if _, err := runFailedSuite(t, fx, failedSuite{
			batches: [][]string{{sha1}}, startSHA: start, masterOutcome: "done", fixer: fixer,
			forkLog: "--- FAIL: TestAlways (0.00s)\n    x_test.go:1: TestAlways failed\nFAIL\nFAIL\texample/pkg\t0.01s\n",
		}); err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if fixer.calls != 0 {
			t.Errorf("StartFix calls = %d; want 0 for a pre-existing verdict", fixer.calls)
		}
	})
}

// TestIntegrationStage_FixAttempt_MasterStuckMakesNoAttempt proves a non-done Master outcome over a regression escalates as before with no StartFix call.
func TestIntegrationStage_FixAttempt_MasterStuckMakesNoAttempt(t *testing.T) {
	sc := newRegressionScene(t, false)
	fixer := newFakeFixStarter(t, sc.fx.Worktree)
	sc.suite.fixer = fixer
	sc.suite.masterOutcome = "stuck"

	result, err := runFailedSuite(t, sc.fx, sc.suite)
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "stuck" || result.StuckReason != "master says stuck" {
		t.Errorf("Outcome = %q, StuckReason = %q; want Master's own stuck and reason", result.Outcome, result.StuckReason)
	}
	if fixer.calls != 0 {
		t.Errorf("StartFix calls = %d; want 0", fixer.calls)
	}
}
