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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/planparser"
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
			head := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
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

	originalBranch := strings.TrimSpace(mustGit(t, fx.Worktree, "symbolic-ref", "--short", "HEAD"))

	sha1 := commitFile(t, fx.Worktree, "card1.txt", "one", "card1")
	sha2 := commitFile(t, fx.Worktree, "card2.txt", "two", "card2")
	sha3 := commitFile(t, fx.Worktree, "bad.marker", "bad", "card3 introduces the bug")

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha1}},
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

	branch := strings.TrimSpace(mustGit(t, fx.Worktree, "symbolic-ref", "--short", "HEAD"))
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
	if err := websterengine.BisectAndEscalate(nil, nil, nil, "true", "/unused", websterDir, st); err != nil {
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

// TestIntegrationStage_FailedSuite_DoneOutcomeFailsLoud proves the FAILED integration report under
// a done outcome is fail-loud, SYMMETRIC with the missing-report-under-done case above: a done
// outcome CLAIMS a passing integration suite, so a Master that wrote outcome: done while the
// plan-level verify actually FAILED is a genuine inconsistency the run must not report as done.
// It also proves the escalation still persists despite the loud return — the reserved -1 record and
// summary.md's SHA-bisect- localized card are written BEFORE the fail-loud error, so a resume or a
// human still sees the offending card. (Before this was fixed the run returned Master's done
// verbatim, contradicting its own escalated summary.md.)
func TestIntegrationStage_FailedSuite_DoneOutcomeFailsLoud(t *testing.T) {
	fx := newRunFixture(t, 3)
	appendIntegrationVerify(t, fx.PlanDir, "test ! -f bad.marker")

	sha1 := commitFile(t, fx.Worktree, "card1.txt", "one", "card1")
	sha2 := commitFile(t, fx.Worktree, "card2.txt", "two", "card2")
	sha3 := commitFile(t, fx.Worktree, "bad.marker", "bad", "card3 introduces the bug")

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha1}},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha2}},
			3: {Slug: "batch3", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha3}},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-intfaildone",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-intfaildone",
			RunDir:    "/run/dir/intfaildone",
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
					{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
					{TranscriptPath: "/transcripts/fork3.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			// The integration report lands FAILED, but Master (wrongly) claims
			// outcome: done — the exact template-violating contradiction this
			// test exists to catch.
			reportPath := websterengine.IntegrationReportPath(fx.Deps.Geom.ReportsDir)
			if err := os.WriteFile(reportPath, []byte("status: FAILED\nhead_sha: "+sha3+"\ndeviations: []\n"), 0o644); err != nil {
				t.Fatalf("write integration report: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 3\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Batches shipped\n\nAll three batches landed.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-intfaildone", "master-session-intfaildone")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatal("Run() = nil error for outcome: done over a FAILED integration suite; want the fail-loud inconsistency error")
	}
	if !strings.Contains(err.Error(), "a done outcome requires a passing integration suite") {
		t.Errorf("Run() error = %q; want the done-over-FAILED-suite inconsistency named", err.Error())
	}

	// The escalation must persist despite the loud return: the reserved -1
	// record and summary.md's localized card are written before the error.
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	escalated, ok := st.Batches[-1]
	if !ok || escalated == nil {
		t.Fatalf("state.json carries no integration escalation record after the loud return; want one at the reserved key")
	}
	if escalated.Slug != "03-batch3" {
		t.Errorf("escalated record Slug = %q; want %q (the localized offending card)", escalated.Slug, "03-batch3")
	}

	summaryData, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if !strings.Contains(string(summaryData), "03-batch3") {
		t.Errorf("summary.md does not name the localized offending card after the loud return; got:\n%s", summaryData)
	}
}
