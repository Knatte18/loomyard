// attachprobe_test.go covers the live-agent probe BurlerProducer and Bouncer run before they archive
// or spawn -- the seam that stops a resumed run from starting a second agent over one that is still
// alive. The probe spans both producers, so its cases live in one file rather than being duplicated
// into burler_test.go, bouncer_seed_test.go, and bouncer_judge_test.go; all three of those files'
// own fakes and fixtures are reused here.
//
// Two properties are asserted in every case, not one. That the probe RAN is not enough: archiving
// renames the very files a live agent is about to write, so a probe that ran after the archive would
// pass a "did we attach" assertion while still having destroyed the attached run's file contract.
// Each case therefore also asserts that the pre-existing artifacts survived untouched.

package shedadapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// stampedSiblingCount reports how many entries in dir share base's stem but not its exact name --
// the archive helper's stamped-sibling shape. Zero proves nothing was archived.
func stampedSiblingCount(t *testing.T, dir, base string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) = %v; want nil", dir, err)
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	count := 0
	for _, e := range entries {
		if e.Name() != base && strings.HasPrefix(e.Name(), stem+"-") {
			count++
		}
	}
	return count
}

// --- BurlerProducer ---

func TestBurlerProducer_AttachesToLiveRoundInsteadOfRespawning(t *testing.T) {
	runDir := t.TempDir()
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	attach := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-session"},
	}
	p := newTestBurlerProducerWithAttach(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, attach, fixedClock(time.Now()))

	// The live agent's own in-progress review, already on disk. It must still be there afterwards.
	writeRoundFile(t, roundReviewPath(runDir, 1))

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q (a completed round hands off to its Bouncer)", outcome, shedengine.Stuck)
	}
	if want := roundReviewPath(runDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if !attach.attachCalled {
		t.Error("Attach was not called; want the probe to run before anything else")
	}
	if runner.calls != 0 {
		t.Errorf("runner.Run calls = %d; want 0 -- a live round must be attached to, never respawned over", runner.calls)
	}
	if _, err := os.Stat(roundReviewPath(runDir, 1)); err != nil {
		t.Errorf("the live round's review file was moved (stat = %v); want it untouched -- archiving renames the file the attached agent is still writing", err)
	}
	if n := stampedSiblingCount(t, runDir, filepath.Base(roundReviewPath(runDir, 1))); n != 0 {
		t.Errorf("stamped archive siblings = %d; want 0 -- the attach branch must not archive", n)
	}
}

func TestBurlerProducer_AttachSpecNamesTheRoundsOwnArtifacts(t *testing.T) {
	runDir := t.TempDir()
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	attach := &fakeShuttle{}
	opts := burlerengine.RunOpts{Timeout: 90 * time.Minute}
	p := newTestBurlerProducerWithAttach(t, runDir, simpleBurlerProfile(), opts, runner, attach, fixedClock(time.Now()))
	writeRoundPair(t, runDir, 1)

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	// Round 1 is complete on disk, so this Call is round 2 -- the probe must name round 2's own
	// pair, since shuttleengine.Attach set-matches a persisted run.json on exactly these paths.
	want := []string{roundReviewPath(runDir, 2), roundFixerReportPath(runDir, 2)}
	got := attach.gotAttachSpec.OutputFiles
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("attach spec OutputFiles = %v; want %v", got, want)
	}
	if attach.gotAttachSpec.Timeout != opts.Timeout {
		t.Errorf("attach spec Timeout = %s; want %s -- an attached run's deadline is the round's, not shuttle's shorter default", attach.gotAttachSpec.Timeout, opts.Timeout)
	}
}

func TestBurlerProducer_NoLiveRunSpawnsExactlyAsBefore(t *testing.T) {
	runDir := t.TempDir()
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	attach := &fakeShuttle{attachFound: false}
	p := newTestBurlerProducerWithAttach(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, attach, fixedClock(time.Now()))

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if runner.calls != 1 {
		t.Errorf("runner.Run calls = %d; want 1 -- a not-found probe must fall through to the unchanged spawn path", runner.calls)
	}
}

func TestBurlerProducer_AttachedRunAlreadyDiedRespawnsFromAttemptOne(t *testing.T) {
	runDir := t.TempDir()
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	attach := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied},
	}
	p := newTestBurlerProducerWithAttach(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, attach, fixedClock(time.Now()))

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if runner.calls != 1 {
		t.Fatalf("runner.Run calls = %d; want 1 -- a dead attached run leaves nothing to attach to, so a fresh spawn is correct", runner.calls)
	}
	// The attached run was not this producer's own attempt, so the retry budget must start fresh.
	if got := runner.gotOpts[0].Round; got != "1" {
		t.Errorf("first spawn's RunOpts.Round = %q; want \"1\" -- counting the dead attached run as attempt 1 would halve every resumed round's retry budget", got)
	}
}

func TestBurlerProducer_AttachErrorNeitherArchivesNorSpawns(t *testing.T) {
	sentinel := errors.New("reed state unreadable")
	runDir := t.TempDir()
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	attach := &fakeShuttle{attachErr: sentinel}
	p := newTestBurlerProducerWithAttach(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, attach, fixedClock(time.Now()))
	writeRoundFile(t, roundReviewPath(runDir, 1))

	outcome, _, err := p.Call(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("Call() error = %v; want errors.Is(err, sentinel)", err)
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
	}
	if runner.calls != 0 {
		t.Errorf("runner.Run calls = %d; want 0", runner.calls)
	}
	if n := stampedSiblingCount(t, runDir, filepath.Base(roundReviewPath(runDir, 1))); n != 0 {
		t.Errorf("stamped archive siblings = %d; want 0 -- an undeterminable probe is exactly when archiving is most dangerous", n)
	}
}

// --- Bouncer, judge pass ---

func TestBouncer_JudgeCall_AttachesToLiveJudgeInsteadOfRespawning(t *testing.T) {
	attach := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-judge"},
	}
	b, cfg := newTestBouncer(t, attach)
	// Only round 1's report exists at Call entry -- a verdict already on disk would settle (or, if
	// APPROVED, clear) before judgeCall is ever reached, so the judge branch would go unexercised.
	// The attached judge writes its verdict and ledger while Call waits on it, which duringAttach
	// stands in for.
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
	attach.duringAttach = func() {
		_ = os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("APPROVED")), 0o644)
		_ = os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644)
	}
	// The round's report is the file the archive step would NOT touch; the focus file for round 2
	// is one it would. Pre-writing it proves the archive did not run on the attach branch.
	staleNextFocus := focusPath(cfg.RunDir, 2)
	if err := os.WriteFile(staleNextFocus, []byte("---\nround: 2\nexclude_lenses: []\nfocus: []\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale next focus) = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if !attach.attachCalled {
		t.Error("Attach was not called; want the judge pass to probe first")
	}
	if attach.called {
		t.Error("Run was called; want a live judge attached to, never respawned over")
	}
	if n := stampedSiblingCount(t, cfg.RunDir, filepath.Base(staleNextFocus)); n != 0 {
		t.Errorf("stamped archive siblings of %s = %d; want 0 -- the attach branch must not archive the judge's declared outputs out from under the live agent", filepath.Base(staleNextFocus), n)
	}
}

func TestBouncer_JudgeCall_AttachErrorDegradesWithoutSpawning(t *testing.T) {
	attach := &fakeShuttle{attachErr: errors.New("reed state unreadable")}
	b, cfg := newTestBouncer(t, attach)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil (an infrastructure fault degrades, it does not abort the run)", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if attach.called {
		t.Error("Run was called after a failed probe; want no spawn when liveness could not be determined")
	}
}

// --- Bouncer, entry-time pass over a verdict already on disk ---
//
// These four cases cover the window the judge spec's own shape opens: a recorded verdict needs the
// verdict and ledger files, while the judge spawn declares those two plus the next round's focus
// file. A crash in between leaves a live judge behind an apparently-final verdict, and Call's clear
// and replay branches both act on that verdict without spawning anything -- so without a probe of
// their own, one archives the run directory out from under the live judge and the other writes a
// synthetic file at a path it declared as an output.

func TestBouncer_EntryProbe_AttachedJudgeSettlesInsteadOfClearing(t *testing.T) {
	attach := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-judge"},
	}
	b, cfg := newClearTestBouncer(t, attach)
	layoutApprovedGeneration(t, cfg, 1)
	// What the live judge still had left to write when the driver died: its third declared output.
	attach.duringAttach = func() {
		_ = os.WriteFile(focusPath(cfg.RunDir, 2), []byte("---\nround: 2\nexclude_lenses: []\nfocus: []\n---\n"), 0o644)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q -- the judgment landed inside this call, so this call harvests it", outcome, shedengine.Done)
	}
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if !attach.attachCalled {
		t.Error("Attach was not called; want the probe to run before the clear branch acts")
	}
	if attach.called {
		t.Error("Run was called; want a live judge attached to, never respawned over")
	}
	if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); err != nil {
		t.Errorf("round 1's verdict file was moved (stat = %v); want it untouched -- the clear would archive it out from under the live judge", err)
	}
	assertNoArchivedRunDirSibling(t, cfg.RunDir)
}

func TestBouncer_EntryProbe_AttachedJudgeSettlesInsteadOfReplaying(t *testing.T) {
	attach := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-judge"},
	}
	b, cfg := newTestBouncer(t, attach)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("BLOCKING"), ledger: bouncerLedgerContent(1),
	}})
	// The live judge's real targeting for round 2, written while Call waits on it. The replay branch
	// would have synthesized two empty lists over this path before it ever landed.
	realFocus := "---\nround: 2\nexclude_lenses: []\nfocus: [\"the finding the judge actually targeted\"]\n---\n"
	attach.duringAttach = func() {
		_ = os.WriteFile(focusPath(cfg.RunDir, 2), []byte(realFocus), 0o644)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if attach.called {
		t.Error("Run was called; want a live judge attached to, never respawned over")
	}
	got, err := os.ReadFile(focusPath(cfg.RunDir, 2))
	if err != nil {
		t.Fatalf("ReadFile(round-2-focus.md) = %v; want nil", err)
	}
	if string(got) != realFocus {
		t.Errorf("round-2-focus.md = %q; want the live judge's own targeting %q -- a synthetic file written over it drops the judge's targeting for the next round", got, realFocus)
	}
	if n := stampedSiblingCount(t, cfg.RunDir, filepath.Base(focusPath(cfg.RunDir, 2))); n != 0 {
		t.Errorf("stamped archive siblings = %d; want 0 -- the attached branch must not archive the live judge's own output", n)
	}
}

func TestBouncer_EntryProbe_AttachErrorNeitherClearsNorSettles(t *testing.T) {
	logBuf := captureBouncerWarnings(t)
	attach := &fakeShuttle{attachErr: errors.New("reed state unreadable")}
	b, cfg := newClearTestBouncer(t, attach)
	layoutApprovedGeneration(t, cfg, 1)

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil (an infrastructure fault degrades, it does not abort the run)", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if attach.called {
		t.Error("Run was called after a failed probe; want no spawn when liveness could not be determined")
	}
	if logBuf.Len() == 0 {
		t.Error("Call() did not log a warning on the failed probe")
	}
	if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); err != nil {
		t.Errorf("round 1's verdict file was moved (stat = %v); want it untouched", err)
	}
	assertNoArchivedRunDirSibling(t, cfg.RunDir)
}

// TestBouncer_EntryProbe_NothingLiveClearsExactlyAsBefore is the not-found half of the pair: the
// probe must change nothing for the ordinary case, where the judge that wrote the verdict is long
// gone and re-entry genuinely means a settled generation is being re-judged.
func TestBouncer_EntryProbe_NothingLiveClearsExactlyAsBefore(t *testing.T) {
	attach := &fakeShuttle{attachFound: false, result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newClearTestBouncer(t, attach)
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
	if !attach.attachCalled {
		t.Error("Attach was not called; want the probe to run even when nothing is live")
	}
	archived := archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")
	if _, err := os.Stat(archived); err != nil {
		t.Errorf("expected the archived generation %s to exist: %v", archived, err)
	}
}

// TestBouncer_EntryProbe_SpecNamesTheJudgesOwnOutputFiles pins what the probe matches on.
// shuttleengine.Attach set-matches a persisted run.json's OutputFiles and nothing else, so a probe
// naming any other set -- the seed pass's single focus file, or the Burler row's review/fixer-report
// pair -- silently matches nothing and is indistinguishable from no agent being alive at all.
func TestBouncer_EntryProbe_SpecNamesTheJudgesOwnOutputFiles(t *testing.T) {
	attach := &fakeShuttle{attachFound: false, result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newTestBouncer(t, attach)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("BLOCKING"), ledger: bouncerLedgerContent(1),
	}})

	if _, _, err := b.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	want := []string{verdictPath(cfg.RunDir, 1), ledgerPath(cfg.RunDir, 1), focusPath(cfg.RunDir, 2)}
	got := attach.gotAttachSpec.OutputFiles
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("attach spec OutputFiles = %v; want %v", got, want)
	}
}

// --- Bouncer, seed pass ---

func TestBouncer_SeedCall_AttachesToLiveSeedInsteadOfRespawning(t *testing.T) {
	attach := &fakeShuttle{
		attachFound:  true,
		attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-seed"},
	}
	b, cfg := newTestBouncer(t, attach)
	// A parseable round-1 focus file would make Call take its re-bounce branch instead of seeding,
	// so the live seed's in-progress file is deliberately unparseable here -- which is exactly what
	// a half-written focus file looks like.
	focus := focusPath(cfg.RunDir, 1)
	if err := os.WriteFile(focus, []byte("---\nround: 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(partial focus) = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q (a seed call always hands off)", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if !attach.attachCalled {
		t.Error("Attach was not called; want the seed pass to probe first")
	}
	if attach.called {
		t.Error("Run was called; want a live seed attached to, never respawned over")
	}
}

func TestBouncer_SeedCall_NoLiveRunArchivesThenSpawns(t *testing.T) {
	attach := &fakeShuttle{attachFound: false, result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newTestBouncer(t, attach)
	focus := focusPath(cfg.RunDir, 1)
	if err := os.WriteFile(focus, []byte("---\nround: 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(partial focus) = %v; want nil", err)
	}

	if _, _, err := b.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if !attach.called {
		t.Error("Run was not called; want a not-found probe to fall through to the unchanged spawn path")
	}
	if n := stampedSiblingCount(t, cfg.RunDir, filepath.Base(focus)); n != 1 {
		t.Errorf("stamped archive siblings = %d; want 1 -- the respawn branch still archives the stale focus file", n)
	}
}
