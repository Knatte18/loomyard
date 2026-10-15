// attachprobe_test.go covers the live-agent probe the Bouncer runs before it archives or spawns -- the seam that stops a resumed run from starting a second agent over one that is still alive.
// The Bouncer probes through its Shuttle; the Burler rows' probes are covered beside their producers in burlerreview_test.go and burlerfix_test.go.
//
// Two properties are asserted in every case, not one. That the probe RAN is not enough: archiving
// renames the very files a live agent is about to write, so a probe that ran after the archive would
// pass a "did we attach" assertion while still having destroyed the attached run's file contract.
// Each case therefore also asserts that the pre-existing artifacts survived untouched.

package shedadapters

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

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

// stopLog records the strand guid of every Stop call on the handles built over it, failing each call with err when set.
type stopLog struct {
	mu      sync.Mutex
	err     error
	stopped []string
}

// probedHandle is a burlerengine.Handle for a half a probe reports live; a producer reads its strand guid and stops it.
// A nil log makes Stop a no-op.
type probedHandle struct {
	guid string
	log  *stopLog
}

func (h probedHandle) StrandGUID() string { return h.guid }

func (h probedHandle) RunDir() string { return "/kept/" + h.guid }

func (h probedHandle) Wait() (shuttleengine.Result, error) { return shuttleengine.Result{}, nil }

func (h probedHandle) Stop() error {
	if h.log == nil {
		return nil
	}
	h.log.mu.Lock()
	defer h.log.mu.Unlock()
	h.log.stopped = append(h.log.stopped, h.guid)
	return h.log.err
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
// shuttleengine.Attach set-matches a persisted run.json's OutputFiles and nothing else, so a probe naming any other set -- the seed pass's single focus file, or a Burler row's review or fixer-report file -- silently matches nothing and is indistinguishable from no agent being alive at all.
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
			if !reflect.DeepEqual(ptr, shedengine.OutputPointer{}) {
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
