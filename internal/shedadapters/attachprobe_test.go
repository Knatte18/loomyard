// attachprobe_test.go covers the live-agent probe BurlerProducer and Bouncer run before they archive
// or spawn -- the seam that stops a resumed run from starting a second agent over one that is still
// alive. BurlerProducer probes the round's two halves through its runner and resumes, stops or respawns by what is live;
// the Bouncer probes through its Shuttle. The probe spans both producers, so its cases live in one file rather than being duplicated
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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
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

// probedHandle is a burlerengine.Handle for a half the runner's probe reports live; the producer reads only its strand guid.
type probedHandle struct{ guid string }

func (h probedHandle) StrandGUID() string { return h.guid }

func (h probedHandle) RunDir() string { return "/kept/" + h.guid }

func (h probedHandle) Wait() (shuttleengine.Result, error) { return shuttleengine.Result{}, nil }

const (
	reviewGUID = "review-guid"
	fixGUID    = "fix-guid"
)

// roundWith builds the LiveRound a probe reports for the given states, a live half carrying its guid's handle.
func roundWith(review, fix burlerengine.HalfState) burlerengine.LiveRound {
	half := func(state burlerengine.HalfState, guid string) burlerengine.LiveHalf {
		if state != burlerengine.HalfLive {
			return burlerengine.LiveHalf{State: state}
		}
		return burlerengine.LiveHalf{State: state, Handle: probedHandle{guid: guid}}
	}
	return burlerengine.LiveRound{Review: half(review, reviewGUID), Fix: half(fix, fixGUID)}
}

func TestBurlerProducer_ResumesBothLiveHalvesInsteadOfRespawning(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	runner := &shedfake.BurlerRunner{
		LiveRounds:    []burlerengine.LiveRound{roundWith(burlerengine.HalfLive, burlerengine.HalfLive)},
		ResumeResults: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}},
	}
	remover := &shedfake.StrandRemover{}
	p := newBurlerProducer(t, runDir, runner, withRemover(remover), withBurlerClock(fixedClock(time.Now())))

	// The live reviewer's own in-progress review, already on disk.
	// It must still be there afterwards.
	if err := os.WriteFile(roundReviewPath(runDir, 1), []byte("---\nverdict: APPROVED\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(review) = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if want := roundReviewPath(runDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if runner.ProbeCalls != 1 || runner.ResumeCalls != 1 {
		t.Errorf("ProbeRound calls %d, Resume calls %d; want 1 each, the probe before anything else", runner.ProbeCalls, runner.ResumeCalls)
	}
	if runner.GotLive[0] != roundWith(burlerengine.HalfLive, burlerengine.HalfLive) {
		t.Errorf("Resume was handed %+v; want the probed round", runner.GotLive[0])
	}
	if runner.Calls != 0 {
		t.Errorf("runner.Run calls = %d; want 0 -- a live round must be resumed, never respawned over", runner.Calls)
	}
	if len(remover.Removed) != 0 {
		t.Errorf("removed strands = %v; want none for a resumed round", remover.Removed)
	}
	if _, err := os.Stat(roundReviewPath(runDir, 1)); err != nil {
		t.Errorf("the live round's review file was moved (stat = %v); want it untouched -- archiving renames the file the live reviewer is still writing", err)
	}
	if n := stampedSiblingCount(t, runDir, filepath.Base(roundReviewPath(runDir, 1))); n != 0 {
		t.Errorf("stamped archive siblings = %d; want 0 -- the resume branch must not archive", n)
	}
}

// TestBurlerProducer_NoLiveHalfSpawnsAttemptOne also pins what the probe is told: the round's own timeout, so a resumed round's deadline is the round's and not the shuttle config's shorter default.
//
//testtiming:keep pins the round and timeout the probe is handed, which no other test reads off the recorded probe options
func TestBurlerProducer_NoLiveHalfSpawnsAttemptOne(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	opts := burlerengine.RunOpts{Timeout: 90 * time.Minute}
	p := newBurlerProducer(t, runDir, runner, withBurlerRunOpts(opts), withBurlerClock(fixedClock(time.Now())))
	// Round 1 is complete on disk, so this Call is round 2.
	writeJudgedRound(t, runDir, 1)

	shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if runner.Calls != 1 || runner.ResumeCalls != 0 {
		t.Errorf("runner.Run calls %d, Resume calls %d; want 1 and 0 -- a probe finding nothing live falls through to the spawn path", runner.Calls, runner.ResumeCalls)
	}
	if got := runner.GotProbeOpts[0]; got.Timeout != opts.Timeout || got.Round != "2" {
		t.Errorf("probe options Timeout %s, Round %q; want %s and %q", got.Timeout, got.Round, opts.Timeout, "2")
	}
}

// TestBurlerProducer_ResumedRoundThatEndedDeadRespawnsFromAttemptOne covers a resumed round whose halves died or timed out.
func TestBurlerProducer_ResumedRoundThatEndedDeadRespawnsFromAttemptOne(t *testing.T) {
	t.Parallel()

	for _, outcome := range []shuttleengine.Outcome{shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			runner := &shedfake.BurlerRunner{
				Results:       []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}},
				LiveRounds:    []burlerengine.LiveRound{roundWith(burlerengine.HalfLive, burlerengine.HalfLive)},
				ResumeResults: []burlerengine.Result{{Outcome: outcome}},
			}
			p := newBurlerProducer(t, runDir, runner, withBurlerClock(fixedClock(time.Now())))

			shedfake.RequireOutcome(t, p, shedengine.Stuck)
			if runner.Calls != 1 {
				t.Fatalf("runner.Run calls = %d; want 1 -- a dead resumed round leaves nothing to attach to, so a fresh spawn is correct", runner.Calls)
			}
			// The resumed round was not this producer's own attempt, so the retry budget must start fresh.
			if got := runner.GotOpts[0].Round; got != "1" {
				t.Errorf("first spawn's RunOpts.Round = %q; want \"1\" -- counting the dead resumed round as attempt 1 would halve every resumed round's retry budget", got)
			}
		})
	}
}

// TestBurlerProducer_LiveRoundErrorNeitherArchivesNorSpawns covers the three errors a live-round probe can end in.
// Each leaves the round's files in place, since a half that may still be live may be writing them.
func TestBurlerProducer_LiveRoundErrorNeitherArchivesNorSpawns(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("reed state unreadable")
	tests := []struct {
		name       string
		runner     *shedfake.BurlerRunner
		remover    *shedfake.StrandRemover
		wantErr    error
		wantSuffix string
	}{
		{
			name:    "probe error",
			runner:  &shedfake.BurlerRunner{ProbeErrs: []error{sentinel}},
			remover: &shedfake.StrandRemover{},
			wantErr: sentinel,
		},
		{
			name:       "failed removal of the one live half",
			runner:     &shedfake.BurlerRunner{LiveRounds: []burlerengine.LiveRound{roundWith(burlerengine.HalfDone, burlerengine.HalfLive)}},
			remover:    &shedfake.StrandRemover{Err: sentinel},
			wantErr:    burlerengine.ErrHalfNotStopped,
			wantSuffix: `way forward: run "lyx reed remove ` + fixGUID + `", then re-step the row`,
		},
		{
			name: "resume that could not stop a half",
			runner: &shedfake.BurlerRunner{
				LiveRounds: []burlerengine.LiveRound{roundWith(burlerengine.HalfLive, burlerengine.HalfLive)},
				ResumeErrs: []error{fmt.Errorf("%w: strand g", burlerengine.ErrHalfNotStopped)},
			},
			remover: &shedfake.StrandRemover{},
			wantErr: burlerengine.ErrHalfNotStopped,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			p := newBurlerProducer(t, runDir, tt.runner, withRemover(tt.remover), withBurlerClock(fixedClock(time.Now())))
			writeRoundFile(t, roundReviewPath(runDir, 1))

			outcome, _, err := p.Call(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Call() error = %v; want errors.Is(err, %v)", err, tt.wantErr)
			}
			if tt.wantSuffix != "" && !strings.HasSuffix(err.Error(), tt.wantSuffix) {
				t.Errorf("Call() error = %q; want it to end with %q", err, tt.wantSuffix)
			}
			if outcome != "" {
				t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
			}
			if tt.runner.Calls != 0 {
				t.Errorf("runner.Run calls = %d; want 0", tt.runner.Calls)
			}
			if n := stampedSiblingCount(t, runDir, filepath.Base(roundReviewPath(runDir, 1))); n != 0 {
				t.Errorf("stamped archive siblings = %d; want 0 -- an undeterminable or unstoppable half is exactly when archiving is most dangerous", n)
			}
		})
	}
}

// TestBurlerProducer_PartiallyLiveRoundIsStoppedAndRespawned covers the pairs a round cannot be resumed from:
// one live half is removed by its strand guid, and a done/gone pair removes nothing, and both spawn attempt 1.
func TestBurlerProducer_PartiallyLiveRoundIsStoppedAndRespawned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		live        burlerengine.LiveRound
		wantRemoved []string
	}{
		{"live reviewer beside a gone fixer", roundWith(burlerengine.HalfLive, burlerengine.HalfGone), []string{reviewGUID}},
		{"live fixer beside a finished review", roundWith(burlerengine.HalfDone, burlerengine.HalfLive), []string{fixGUID}},
		{"finished review and a gone fixer", roundWith(burlerengine.HalfDone, burlerengine.HalfGone), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			runner := &shedfake.BurlerRunner{
				Results:    []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}},
				LiveRounds: []burlerengine.LiveRound{tt.live},
			}
			remover := &shedfake.StrandRemover{}
			p := newBurlerProducer(t, runDir, runner, withRemover(remover), withBurlerClock(fixedClock(time.Now())))

			shedfake.RequireOutcome(t, p, shedengine.Stuck)
			if !slices.Equal(remover.Removed, tt.wantRemoved) {
				t.Errorf("removed strands = %v; want %v", remover.Removed, tt.wantRemoved)
			}
			if runner.ResumeCalls != 0 || runner.Calls != 1 {
				t.Errorf("Resume calls %d, runner.Run calls %d; want 0 and 1", runner.ResumeCalls, runner.Calls)
			}
			if got := runner.GotOpts[0].Round; got != "1" {
				t.Errorf("spawn's RunOpts.Round = %q; want \"1\"", got)
			}
		})
	}
}

// --- Bouncer, judge pass ---

//testtiming:keep pins that the judge pass attaches to a live judge without archiving its declared outputs out from under it
func TestBouncer_JudgeCall_AttachesToLiveJudgeInsteadOfRespawning(t *testing.T) {
	attach := &shedfake.Shuttle{
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-judge"},
	}
	b, cfg := newBouncerFixture(t, withShuttle(attach)).Build()
	// Only round 1's report exists at Call entry -- a verdict already on disk would settle (or, if CONVERGED, clear) before judgeCall is ever reached,
	// so the judge branch would go unexercised.
	// The attached judge writes its verdict and ledger while Call waits on it, which duringAttach
	// stands in for.
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
	attach.DuringAttach = func() {
		_ = os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("CONVERGED")), 0o644)
		_ = os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644)
	}
	// The round's report is the file the archive step would NOT touch; the focus file for round 2
	// is one it would. Pre-writing it proves the archive did not run on the attach branch.
	staleNextFocus := focusPath(cfg.RunDir, 2)
	if err := os.WriteFile(staleNextFocus, []byte("---\nround: 2\nexclude_lenses: []\nfocus: []\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale next focus) = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if !attach.AttachCalled {
		t.Error("Attach was not called; want the judge pass to probe first")
	}
	if attach.Called {
		t.Error("Run was called; want a live judge attached to, never respawned over")
	}
	if n := stampedSiblingCount(t, cfg.RunDir, filepath.Base(staleNextFocus)); n != 0 {
		t.Errorf("stamped archive siblings of %s = %d; want 0 -- the attach branch must not archive the judge's declared outputs out from under the live agent", filepath.Base(staleNextFocus), n)
	}
}

func TestBouncer_JudgeCall_AttachErrorDegradesWithoutSpawning(t *testing.T) {
	attach := &shedfake.Shuttle{AttachErr: errors.New("reed state unreadable")}
	b, cfg := newBouncerFixture(t, withShuttle(attach)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if ptr.Path != "" || ptr.GateAttempts != nil {
		t.Errorf("Call() pointer = %+v; want empty Path and no GateAttempts", ptr)
	}
	if attach.Called {
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

//testtiming:keep pins that a live judge behind an already-written verdict is attached to before the clear or replay branch acts, and its own output is neither archived nor overwritten
func TestBouncer_EntryProbe_AttachedJudgeSettlesInsteadOfClearingOrReplaying(t *testing.T) {
	tests := []struct {
		name string
		// approved lays out an already-approved generation, which the clear branch would archive;
		// otherwise a CONTINUE verdict, which the replay branch would synthesize a focus file for.
		approved    bool
		wantOutcome shedengine.Outcome
	}{
		{"instead of clearing an approved generation", true, shedengine.Done},
		{"instead of replaying a CONTINUE verdict", false, shedengine.Stuck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attach := &shedfake.Shuttle{
				AttachFound:  true,
				AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-judge"},
			}
			b, cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(attach)).Build()
			if tt.approved {
				layoutApprovedGeneration(t, cfg, 1)
			} else {
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
					round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: bouncerLedgerContent(1),
				}})
			}
			// The live judge's real targeting for round 2, its third declared output, written while
			// Call waits on it. A clear would archive the run dir out from under it and a replay would
			// have synthesized two empty lists over this path before it ever landed.
			realFocus := "---\nround: 2\nexclude_lenses: []\nfocus: [\"the finding the judge actually targeted\"]\n---\n"
			attach.DuringAttach = func() {
				_ = os.WriteFile(focusPath(cfg.RunDir, 2), []byte(realFocus), 0o644)
			}

			ptr := shedfake.RequireOutcome(t, b, tt.wantOutcome)
			if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			if !attach.AttachCalled {
				t.Error("Attach was not called; want the probe to run before the clear or replay branch acts")
			}
			if attach.Called {
				t.Error("Run was called; want a live judge attached to, never respawned over")
			}
			if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); err != nil {
				t.Errorf("round 1's verdict file was moved (stat = %v); want it untouched -- the clear would archive it out from under the live judge", err)
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
			assertNoArchivedRunDirSibling(t, cfg.RunDir)
		})
	}
}

func TestBouncer_EntryProbe_AttachErrorNeitherClearsNorSettles(t *testing.T) {
	logBuf := logcapture.Capture(t)
	attach := &shedfake.Shuttle{AttachErr: errors.New("reed state unreadable")}
	b, cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(attach)).Build()
	layoutApprovedGeneration(t, cfg, 1)

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if ptr.Path != "" || ptr.GateAttempts != nil {
		t.Errorf("Call() pointer = %+v; want empty Path and no GateAttempts", ptr)
	}
	if attach.Called {
		t.Error("Run was called after a failed probe; want no spawn when liveness could not be determined")
	}
	if logBuf.String() == "" {
		t.Error("Call() did not log a warning on the failed probe")
	}
	if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); err != nil {
		t.Errorf("round 1's verdict file was moved (stat = %v); want it untouched", err)
	}
	assertNoArchivedRunDirSibling(t, cfg.RunDir)
}

// TestBouncer_EntryProbe_SpecNamesTheJudgesOwnOutputFiles pins what the probe matches on.
// shuttleengine.Attach set-matches a persisted run.json's OutputFiles and nothing else, so a probe
// naming any other set -- the seed pass's single focus file, or the Burler row's review/fixer-report
// pair -- silently matches nothing and is indistinguishable from no agent being alive at all.
//
//testtiming:keep pins the OutputFiles set the judge's entry probe names, which no other test reads off the recorded attach spec
func TestBouncer_EntryProbe_SpecNamesTheJudgesOwnOutputFiles(t *testing.T) {
	attach := &shedfake.Shuttle{AttachFound: false, Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newBouncerFixture(t, withShuttle(attach)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: bouncerLedgerContent(1),
	}})

	shedfake.CallOK(t, b)

	want := []string{verdictPath(cfg.RunDir, 1), ledgerPath(cfg.RunDir, 1), focusPath(cfg.RunDir, 2)}
	got := attach.GotAttachSpec.OutputFiles
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("attach spec OutputFiles = %v; want %v", got, want)
	}
}

// --- Bouncer, seed pass ---

func TestBouncer_SeedCall_ProbesForALiveSeed(t *testing.T) {
	tests := []struct {
		name        string
		attachFound bool
	}{
		{"a live seed is attached to, never respawned over", true},
		{"no live seed archives the stale focus file then spawns", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attach := &shedfake.Shuttle{
				AttachFound:  tt.attachFound,
				AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-seed"},
				Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}
			b, cfg := newBouncerFixture(t, withShuttle(attach)).Build()
			// A parseable round-1 focus file would make Call take its re-bounce branch instead of seeding,
			// so the in-progress file is deliberately unparseable here -- which is exactly what
			// a half-written focus file looks like.
			focus := focusPath(cfg.RunDir, 1)
			if err := os.WriteFile(focus, []byte("---\nround: 1\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(partial focus) = %v; want nil", err)
			}

			ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if ptr != (shedengine.OutputPointer{}) {
				t.Errorf("Call() pointer = %+v; want empty", ptr)
			}
			if !attach.AttachCalled {
				t.Error("Attach was not called; want the seed pass to probe first")
			}
			if attach.Called == tt.attachFound {
				t.Errorf("Run called = %v; want %v -- a live seed is attached to, a not-found probe falls through to the spawn", attach.Called, !tt.attachFound)
			}
			if n := stampedSiblingCount(t, cfg.RunDir, filepath.Base(focus)); !tt.attachFound && n != 1 {
				t.Errorf("stamped archive siblings = %d; want 1 -- the respawn branch still archives the stale focus file", n)
			}
		})
	}
}
