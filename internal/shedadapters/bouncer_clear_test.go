// bouncer_clear_test.go covers Bouncer.Call's clear-and-re-seed step: the trigger (an already-
// judged, APPROVED round at Call entry), its archive-naming and collision behaviour, every
// non-triggering case the trigger's fire set must exclude, the harvest path's immunity, the
// clear's own failure degradation, and the cross-invocation and post-commit-failure cases the
// trigger is intended to reach.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// newClearTestBouncerConfig builds a BouncerConfig exactly like testBouncerConfig, except RunDir is
// a subdirectory the test creates inside t.TempDir() rather than t.TempDir() itself, so an archived
// sibling this file's clear-triggering tests produce lands inside the temp tree and is cleaned up
// with it.
func newClearTestBouncerConfig(t *testing.T) BouncerConfig {
	t.Helper()

	parent := t.TempDir()
	runDir := filepath.Join(parent, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(runDir) = %v; want nil", err)
	}
	stencilsDir := shippedBouncerStencilsFixture(t, "bouncer-template-rubric", "# Rubric\n\nBe thorough and cite evidence.\n")
	return BouncerConfig{
		Name:          "gate",
		RunDir:        runDir,
		ArtifactPaths: []string{filepath.Join(parent, "artifact.md")},
		ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
		StencilsDir:   stencilsDir,
		RubricStencil: "bouncer-template-rubric",
		Model:         "claude-x",
		Effort:        "high",
		Version:       "v1",
		Now:           fixedClock(bouncerJudgeTestClock),
	}
}

// newClearTestBouncer builds a *Bouncer over newClearTestBouncerConfig's run dir, driven by shuttle.
func newClearTestBouncer(t *testing.T, shuttle Shuttle) (*Bouncer, BouncerConfig) {
	t.Helper()

	cfg := newClearTestBouncerConfig(t)
	cfg.Shuttle = shuttle
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	return b, cfg
}

// layoutApprovedGeneration writes a full, already-settled generation for round into cfg.RunDir: the
// round producer's own review and fixer-report pair (BurlerProducer's artifacts) alongside the
// Bouncer's report, APPROVED verdict, and ledger -- the complete on-disk state a clear must move as
// one unit.
func layoutApprovedGeneration(t *testing.T, cfg BouncerConfig, round int) {
	t.Helper()

	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   round,
		report:  bouncerReport(round),
		verdict: bouncerVerdictContent("APPROVED"),
		ledger:  bouncerLedgerContent(round),
	}})
	if err := os.WriteFile(roundReviewPath(cfg.RunDir, round), []byte(fmt.Sprintf("review for round %d", round)), 0o644); err != nil {
		t.Fatalf("WriteFile(review round %d) = %v; want nil", round, err)
	}
	if err := os.WriteFile(roundFixerReportPath(cfg.RunDir, round), []byte(fmt.Sprintf("fixer report for round %d", round)), 0o644); err != nil {
		t.Fatalf("WriteFile(fixer report round %d) = %v; want nil", round, err)
	}
}

// archivedRunDirPath returns the path archiveRunDir would rename runDir to when now resolves to
// instant and suffix names the same-second collision slot ("" for the first, "-1" for the second,
// and so on), mirroring archive.go's own directory-naming scheme.
func archivedRunDirPath(runDir string, instant time.Time, suffix string) string {
	dir := filepath.Dir(runDir)
	base := filepath.Base(runDir)
	stamp := instant.UTC().Format(archiveTimestampFormat)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, stamp, suffix))
}

// assertNoArchivedRunDirSibling fails t if runDir's parent contains any entry named runDir's own
// base name plus a "-" suffix, which is the shape every archived run-dir sibling this package
// writes takes -- the assertion a non-triggering case's test uses to prove the clear did not fire.
func assertNoArchivedRunDirSibling(t *testing.T, runDir string) {
	t.Helper()

	parent := filepath.Dir(runDir)
	base := filepath.Base(runDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v; want nil", parent, err)
	}
	for _, e := range entries {
		if e.Name() != base && strings.HasPrefix(e.Name(), base+"-") {
			t.Errorf("unexpected archived run-dir sibling %q; the clear must not have fired", e.Name())
		}
	}
}

func TestBouncer_Clear_ApprovedRunDirClearsAndReseeds(t *testing.T) {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newClearTestBouncer(t, shuttle)
	layoutApprovedGeneration(t, cfg, 1)

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q (the clear falls through to the seed path)", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if !shuttle.called {
		t.Error("Call() did not invoke the shuttle seam; want the seed spawn the clear falls through to")
	}

	archived := archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")
	entries, err := os.ReadDir(archived)
	if err != nil {
		t.Fatalf("ReadDir(archived sibling %q) = %v; want nil", archived, err)
	}
	wantNames := map[string]bool{
		cfg.ReportName(1):            false,
		"round-1-bouncer-verdict.md": false,
		"round-1-bouncer-ledger.md":  false,
		"round-1-review.md":          false,
		"round-1-fixer-report.md":    false,
	}
	for _, e := range entries {
		if _, ok := wantNames[e.Name()]; ok {
			wantNames[e.Name()] = true
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("archived sibling %q is missing entry %q; the whole generation must move together", archived, name)
		}
	}

	freshEntries, err := os.ReadDir(cfg.RunDir)
	if err != nil {
		t.Fatalf("ReadDir(recreated run dir) = %v; want nil", err)
	}
	if len(freshEntries) != 1 || freshEntries[0].Name() != "round-1-focus.md" {
		names := make([]string, len(freshEntries))
		for i, e := range freshEntries {
			names[i] = e.Name()
		}
		t.Errorf("recreated run dir entries = %v; want exactly [round-1-focus.md] (only what seedCall writes)", names)
	}
}

func TestBouncer_Clear_CollisionTakesNumericSuffix(t *testing.T) {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newClearTestBouncer(t, shuttle)
	layoutApprovedGeneration(t, cfg, 1)

	if _, _, err := b.Call(context.Background()); err != nil {
		t.Fatalf("Call() (first clear) error = %v; want nil", err)
	}

	// Re-approve a second generation in the freshly recreated run dir, under the same injected
	// clock second, so the second clear's archive target collides with the first.
	layoutApprovedGeneration(t, cfg, 1)

	if _, _, err := b.Call(context.Background()); err != nil {
		t.Fatalf("Call() (second clear) error = %v; want nil", err)
	}

	first := archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")
	if _, err := os.Stat(first); err != nil {
		t.Errorf("expected the first archived sibling %s to still exist: %v", first, err)
	}
	second := archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "-1")
	if _, err := os.Stat(second); err != nil {
		t.Errorf("expected the collision-suffixed archived sibling %s to exist: %v", second, err)
	}
}

func TestBouncer_Clear_NonTriggeringCasesLeaveRunDirUntouched(t *testing.T) {
	t.Run("InSegmentBlockingReplay", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newClearTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
			round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("BLOCKING"), ledger: bouncerLedgerContent(1),
		}})

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		wantPointer := ledgerPath(cfg.RunDir, 1)
		if ptr.Path != wantPointer {
			t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
		}
		if shuttle.called {
			t.Error("Call() invoked the shuttle seam on a BLOCKING replay; want it never called")
		}
		assertNoArchivedRunDirSibling(t, cfg.RunDir)
	})

	t.Run("MidSegmentResumeUnjudgedRound", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newClearTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
			{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("APPROVED"), ledger: bouncerLedgerContent(1)},
			{round: 2, report: bouncerReport(2)},
		})

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if ptr != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", ptr)
		}
		if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); err != nil {
			t.Errorf("round 1's verdict file was removed even though round 2 -- not round 1 -- is the resolved round: %v", err)
		}
		assertNoArchivedRunDirSibling(t, cfg.RunDir)
	})

	t.Run("ReBounce_FocusSeededNoReport", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newClearTestBouncer(t, shuttle)
		seeded := "---\nround: 1\nexclude_lenses: []\nfocus: [\"already seeded\"]\n---\n"
		if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte(seeded), 0o644); err != nil {
			t.Fatalf("WriteFile(...) = %v; want nil", err)
		}

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if ptr != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", ptr)
		}
		if shuttle.called {
			t.Error("Call() invoked the shuttle seam on a re-bounce; want it never called")
		}
		assertNoArchivedRunDirSibling(t, cfg.RunDir)
	})

	t.Run("VerdictWithNoParsableLedger", func(t *testing.T) {
		logBuf := captureBouncerWarnings(t)
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newClearTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
			round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("APPROVED"),
		}})

		outcome, ptr, err := b.Call(context.Background())
		assertJudgeDegraded(t, outcome, ptr, err)
		if logBuf.Len() == 0 {
			t.Error("Call() did not log a warning on a degraded path")
		}
		assertNoArchivedRunDirSibling(t, cfg.RunDir)
	})
}

func TestBouncer_Clear_HarvestApprovedDoesNotClear(t *testing.T) {
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
	b, cfg := newClearTestBouncer(t, shuttle)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q (the call that earns Done must not clear)", outcome, shedengine.Done)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}
	if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); err != nil {
		t.Errorf("round 1's verdict file was removed on the very call that produced it: %v", err)
	}
	assertNoArchivedRunDirSibling(t, cfg.RunDir)
}

// TestBouncer_Clear_ArchiveFailureDegradesToStuck proves a failed clear degrades to Stuck with an
// empty pointer and a nil error, matching seedCall's own stale-focus-archive degrade beside it.
//
// It exercises this via a rename failure -- the parent directory carries no write permission -- the
// one reliable, portable trigger available here. A rename failure and a recreate failure both
// surface through archiveRunDir's own wrapped error and are handled identically by this call site,
// so this one trigger proves the wiring for both: under POSIX permission semantics, recreate's
// write requirement on runDir's parent is a strict subset of rename's (rename both removes runDir's
// old entry and adds the archived sibling's entry there; recreate only adds runDir's own entry back),
// so any parent permission that lets the rename proceed necessarily lets the recreate proceed too --
// the two failure causes are mechanically inseparable at this call site. archiveRunDir's own
// rename-failure contract is unit-tested directly in archive_test.go.
func TestBouncer_Clear_ArchiveFailureDegradesToStuck(t *testing.T) {
	logBuf := captureBouncerWarnings(t)
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newClearTestBouncer(t, shuttle)
	layoutApprovedGeneration(t, cfg, 1)

	parent := filepath.Dir(cfg.RunDir)
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatalf("Chmod(parent) = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if shuttle.called {
		t.Error("Call() invoked the shuttle seam after a failed clear; want it never reached")
	}
	if logBuf.Len() == 0 {
		t.Error("Call() did not log a warning on the failed clear")
	}
}

// TestBouncer_Clear_FreshBouncerOverPreviouslyApprovedRunDir is the cross-invocation case: a
// Bouncer value constructed fresh over a run directory a previous process already settled clears
// and re-seeds on its very first Call, exactly as an in-process re-entry does. Nothing about the
// trigger depends on in-memory state, since it reads only what a previous settle wrote to disk.
func TestBouncer_Clear_FreshBouncerOverPreviouslyApprovedRunDir(t *testing.T) {
	cfg := newClearTestBouncerConfig(t)
	cfg.Shuttle = &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	layoutApprovedGeneration(t, cfg, 1)

	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	archived := archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")
	if _, err := os.Stat(archived); err != nil {
		t.Errorf("expected archived sibling %s from the previous process's generation to exist: %v", archived, err)
	}
}

// TestBouncer_Clear_AfterCommitFailureSubsequentCallClears is the accepted-regression case: a
// Commit failure surfaces from settle unchanged (the run directory is left APPROVED, since settle
// never archives on that path), and the next Call over that same still-APPROVED directory clears
// and re-seeds instead of retrying the commit -- both halves asserted in one test so the sequence is
// the subject.
func TestBouncer_Clear_AfterCommitFailureSubsequentCallClears(t *testing.T) {
	sentinel := errors.New("commit failed")
	commitCalls := 0
	cfg := newClearTestBouncerConfig(t)
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
	cfg.Shuttle = shuttle
	cfg.Commit = func() error {
		commitCalls++
		if commitCalls == 1 {
			return sentinel
		}
		return nil
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	outcome, ptr, err := b.Call(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("Call() (first) error = %v; want errors.Is(err, sentinel)", err)
	}
	if outcome != "" {
		t.Errorf("Call() (first) outcome = %q; want empty alongside a non-nil error", outcome)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() (first) pointer = %+v; want empty", ptr)
	}
	if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); err != nil {
		t.Fatalf("round 1's verdict file must survive a Commit failure (settle never archives on that path): %v", err)
	}

	outcome, ptr, err = b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() (second) error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() (second) outcome = %q; want %q (the still-APPROVED directory clears and re-seeds)", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() (second) pointer = %+v; want empty", ptr)
	}
	if commitCalls != 1 {
		t.Errorf("Commit call count = %d; want 1 (the second Call clears and re-seeds rather than retrying the commit)", commitCalls)
	}
	archived := archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")
	if _, err := os.Stat(archived); err != nil {
		t.Errorf("expected archived sibling %s to exist after the second Call: %v", archived, err)
	}
}

// TestBouncer_Clear_EndToEndSequence runs a full within-package sequence -- seed, judge BLOCKING,
// judge APPROVED with Done, re-enter -- and asserts the re-entering Call is itself a seed call that
// writes round-1-focus.md into a fresh run directory with the prior generation preserved beside it.
func TestBouncer_Clear_EndToEndSequence(t *testing.T) {
	// Round 1: seed.
	seedShuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	seedShuttle.duringRun = func() {
		path := seedShuttle.gotSpec.OutputFiles[0]
		content := "---\nround: 1\nexclude_lenses: []\nfocus: [\"check the thing\"]\n---\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	b, cfg := newClearTestBouncer(t, seedShuttle)

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() (seed) error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() (seed) outcome = %q; want %q", outcome, shedengine.Stuck)
	}

	// Round 1: judge BLOCKING. The round producer writes its own report first.
	if err := os.WriteFile(filepath.Join(cfg.RunDir, cfg.ReportName(1)), []byte(bouncerReport(1)), 0o644); err != nil {
		t.Fatalf("WriteFile(round-1 report) = %v; want nil", err)
	}
	judgeShuttle := judgeFakeShuttle(1, bouncerVerdictContent("BLOCKING"), bouncerLedgerContent(1), true)
	b.cfg.Shuttle = judgeShuttle

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() (judge BLOCKING) error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() (judge BLOCKING) outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr.Path != ledgerPath(cfg.RunDir, 1) {
		t.Fatalf("Call() (judge BLOCKING) pointer = %q; want round 1's ledger", ptr.Path)
	}

	// Round 2: judge APPROVED, earning Done.
	if err := os.WriteFile(filepath.Join(cfg.RunDir, cfg.ReportName(2)), []byte(bouncerReport(2)), 0o644); err != nil {
		t.Fatalf("WriteFile(round-2 report) = %v; want nil", err)
	}
	approveShuttle := judgeFakeShuttle(2, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(2), true)
	b.cfg.Shuttle = approveShuttle

	outcome, ptr, err = b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() (judge APPROVED) error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Fatalf("Call() (judge APPROVED) outcome = %q; want %q", outcome, shedengine.Done)
	}
	if ptr.Path != ledgerPath(cfg.RunDir, 2) {
		t.Fatalf("Call() (judge APPROVED) pointer = %q; want round 2's ledger", ptr.Path)
	}

	// Re-entry: the segment is called again with the same, now-APPROVED run directory.
	reentrySeedShuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b.cfg.Shuttle = reentrySeedShuttle

	outcome, ptr, err = b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() (re-entry) error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() (re-entry) outcome = %q; want %q (a seed call)", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() (re-entry) pointer = %+v; want empty", ptr)
	}
	if !reentrySeedShuttle.called {
		t.Error("Call() (re-entry) did not invoke the shuttle seam; want the seed spawn")
	}

	freshRaw, err := os.ReadFile(focusPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(round-1-focus.md in the fresh run dir) = %v; want nil", err)
	}
	if _, err := parseFocus(freshRaw); err != nil {
		t.Fatalf("parseFocus(fresh round-1-focus.md) error = %v; want nil", err)
	}

	archived := archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")
	entries, err := os.ReadDir(archived)
	if err != nil {
		t.Fatalf("ReadDir(archived sibling %q) = %v; want nil (the prior generation preserved beside the fresh dir)", archived, err)
	}
	if len(entries) == 0 {
		t.Error("archived sibling is empty; want the prior two-round generation preserved")
	}
}
