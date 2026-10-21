// bouncer_reentry_test.go covers the Bouncer's settle record and its re-entry over a settled verdict: the records an approved settle writes, and the cases that re-settle, harvest or clear.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// reentry is a Bouncer over a nested run directory whose Subject, clock, Commit and Bounces seams the test steers through its fields.
// Review reports are named as the recipe names them, so the settle record's liveness reads them.
type reentry struct {
	t       *testing.T
	b       *Bouncer
	cfg     BouncerConfig
	shuttle *shedfake.Shuttle

	subject    Subject
	subjectOK  bool
	subjectErr error
	now        time.Time
	commitErr  error
	skip       bool
	log        seamLog
	recorder   carryOverRecorder

	// noSpawn fails the test when the Bounces seam is read, which only a path that judges reads.
	noSpawn      bool
	spawnsBefore int
}

type reentryOption func(h *reentry, cfg *BouncerConfig)

var errReentryCommit = errors.New("commit failed")

// convergingShuttle returns a shuttle whose judge run writes a CONVERGED verdict, the ledger ledger renders for the round and the next round's focus file.
func convergingShuttle(ledger func(round int) string) *shedfake.Shuttle {
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	shuttle.DuringRun = func() {
		outputs := shuttle.GotSpec.OutputFiles
		if len(outputs) != 3 {
			return
		}
		round, _ := OutputRound(outputs[0])
		ledgerBody := bouncerLedgerContent(round)
		if ledger != nil {
			ledgerBody = ledger(round)
		}
		_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent("CONVERGED")), 0o644)
		_ = os.WriteFile(outputs[1], []byte(ledgerBody), 0o644)
		_ = os.WriteFile(outputs[2], []byte(fmt.Sprintf("---\nround: %d\nexclude_lenses: []\nfocus: []\n---\n", round+1)), 0o644)
	}
	return shuttle
}

func newReentry(t *testing.T, shuttle *shedfake.Shuttle, opts ...reentryOption) *reentry {
	t.Helper()
	h := &reentry{
		t:         t,
		shuttle:   shuttle,
		subject:   Subject{Kind: SubjectDocument, Hash: "hash-1", Base: "base-1"},
		subjectOK: true,
		now:       bouncerJudgeTestClock,
	}
	cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle), withClock(func() time.Time { return h.now })).Config
	cfg.ReportName = func(round int) string { return fmt.Sprintf("round-%d-review.md", round) }
	cfg.Subject = func() (Subject, bool, error) { return h.subject, h.subjectOK, h.subjectErr }
	cfg.Approve = h.log.approve(nil)
	cfg.Commit = func() error {
		h.log.calls = append(h.log.calls, "commit")
		return h.commitErr
	}
	cfg.Bounces = func() (int, int, bool, error) {
		if h.noSpawn {
			t.Error("the Bounces seam was read on a path that must not judge")
		}
		return 0, 0, false, nil
	}
	for _, opt := range opts {
		opt(h, &cfg)
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	h.b, h.cfg = b, cfg
	return h
}

func (h *reentry) call() (shedengine.Outcome, shedengine.OutputPointer, error) {
	return h.b.Call(context.Background())
}

// settleRoundOne lays out round 1's review and runs one Call, which judges and settles it with Commit answering commitErr.
func (h *reentry) settleRoundOne(commitErr error) error {
	h.t.Helper()
	layoutBouncerRun(h.t, h.cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
	h.commitErr = commitErr
	_, _, err := h.call()
	return err
}

// resume advances the clock, lets Commit succeed and forbids a spawn or a judged round from now on.
func (h *reentry) resume() {
	h.now = h.now.Add(time.Hour)
	h.commitErr = nil
	h.noSpawn = true
	h.spawnsBefore = len(h.shuttle.Specs)
}

func (h *reentry) requireNoSpawn() {
	h.t.Helper()
	if got := len(h.shuttle.Specs); got != h.spawnsBefore {
		h.t.Errorf("shuttle runs = %d; want %d, no spawn since the resume", got, h.spawnsBefore)
	}
}

func (h *reentry) settleRecord() settleRecord {
	h.t.Helper()
	record, found, err := readSettleRecord(h.cfg.RunDir)
	if err != nil || !found {
		h.t.Fatalf("readSettleRecord() = %+v, %v, %v; want a record", record, found, err)
	}
	return record
}

func (h *reentry) requireNoRecords() {
	h.t.Helper()
	for _, name := range []string{settleRecordFile, generationRecordFile} {
		if _, err := os.Stat(filepath.Join(h.cfg.RunDir, name)); !errors.Is(err, os.ErrNotExist) {
			h.t.Errorf("%s exists in the run directory (stat error %v); want none", name, err)
		}
	}
}

func countCalls(log seamLog, name string) int {
	count := 0
	for _, call := range log.calls {
		if call == name {
			count++
		}
	}
	return count
}

func openFindingLedger(t *testing.T) func(round int) string {
	return func(round int) string {
		return evidenceLedgerContent(t, round, evidenceEntry{"alpha", "open", "design", "MEDIUM"})
	}
}

//testtiming:keep pins the settle record's writes and the re-entry cases over a settled verdict, whose spawn-free and clearing paths no other test reaches
func TestBouncer_Reentry(t *testing.T) {
	t.Parallel()
	codeSubject := Subject{Kind: SubjectCode, MergeBase: "base-1", Head: "head-1", FullHash: "full-1", PatchID: "patch-1", Patch: []byte("diff")}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"ConvergedSettleWhoseCommitFailedResumesWithNoSpawn", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil))
			if err := h.settleRoundOne(errReentryCommit); !errors.Is(err, errReentryCommit) {
				t.Fatalf("first Call() error = %v; want the commit failure", err)
			}
			first := h.settleRecord()
			if first.Settled {
				t.Error("record settled = true after a failed Commit; want false")
			}

			h.resume()
			ptr := shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if want := ledgerPath(h.cfg.RunDir, 1); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			h.requireNoSpawn()
			assertNoArchivedRunDirSibling(t, h.cfg.RunDir)
			second := h.settleRecord()
			if !second.Settled || second.SettledAt != first.SettledAt {
				t.Errorf("record = settled %t at %q; want settled at the first write's %q", second.Settled, second.SettledAt, first.SettledAt)
			}
			if got := countCalls(h.log, "commit"); got != 2 {
				t.Errorf("Commit ran %d times; want 2", got)
			}
		}},
		{"OpenFindingsRerunTheCarryOver", func(t *testing.T) {
			var recorder carryOverRecorder
			h := newReentry(t, convergingShuttle(openFindingLedger(t)), func(h *reentry, cfg *BouncerConfig) { recorder.install(cfg) })
			if err := h.settleRoundOne(errReentryCommit); !errors.Is(err, errReentryCommit) {
				t.Fatalf("first Call() error = %v; want the commit failure", err)
			}

			h.resume()
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if len(recorder.entries) != 2 {
				t.Fatalf("carry-over entries = %d; want 2, one per settle", len(recorder.entries))
			}
			if first, second := recorder.entries[0], recorder.entries[1]; first.Closing != second.Closing || len(second.Findings) != 1 || first.Findings[0].Key != second.Findings[0].Key {
				t.Errorf("carry-over entries = %+v, %+v; want the same converged entry twice", first, second)
			}
			h.requireNoSpawn()
		}},
		{"CirclingAcceptWhoseCommitFailedReSettlesAsAccepted", func(t *testing.T) {
			var recorder carryOverRecorder
			h := newReentry(t, convergingShuttle(nil), func(h *reentry, cfg *BouncerConfig) { recorder.install(cfg) })
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{
				round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CIRCLING"), ledger: bouncerLedgerContent(1),
			}})
			if err := recordAutoAccept(h.cfg.RunDir, 1); err != nil {
				t.Fatalf("recordAutoAccept() = %v; want nil", err)
			}
			h.commitErr = errReentryCommit
			if _, _, err := h.call(); !errors.Is(err, errReentryCommit) {
				t.Fatalf("first Call() error = %v; want the commit failure", err)
			}

			h.resume()
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			var closings []string
			for _, entry := range recorder.entries {
				closings = append(closings, string(entry.Closing))
			}
			if !slices.Equal(closings, []string{"accepted", "accepted"}) {
				t.Errorf("carry-over closings = %v; want accepted twice", closings)
			}
			h.requireNoSpawn()
			assertNoArchivedRunDirSibling(t, h.cfg.RunDir)
		}},
		{"MalformedCirclingDecisionOnASettledRoundFailsTheReSettle", func(t *testing.T) {
			var recorder carryOverRecorder
			h := newReentry(t, convergingShuttle(nil), func(h *reentry, cfg *BouncerConfig) { recorder.install(cfg) })
			if err := h.settleRoundOne(errReentryCommit); !errors.Is(err, errReentryCommit) {
				t.Fatalf("first Call() error = %v; want the commit failure", err)
			}
			decisionPath := circlingDecisionPath(h.cfg.RunDir, 1)
			if err := os.WriteFile(decisionPath, []byte("not a decision file"), 0o644); err != nil {
				t.Fatalf("write malformed decision: %v", err)
			}

			h.resume()
			if _, _, err := h.call(); err == nil {
				t.Fatal("Call() error = nil; want the malformed decision reported")
			}
			if len(recorder.entries) != 1 {
				t.Errorf("carry-over entries = %d; want 1, the first settle's alone", len(recorder.entries))
			}
			h.requireNoSpawn()
		}},
		{"SkipSeamApprovalReentersWithNoCarryOver", func(t *testing.T) {
			var recorder carryOverRecorder
			h := newReentry(t, convergingShuttle(nil), func(h *reentry, cfg *BouncerConfig) {
				recorder.install(cfg)
				cfg.Skip = func() (bool, error) { return h.skip, nil }
			})
			h.skip = true
			ptr := shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if ptr.Path != "" {
				t.Errorf("Call() pointer = %q; want empty", ptr.Path)
			}
			generation, found, err := readGenerationRecord(h.cfg.RunDir)
			if err != nil || !found || generation.Number != 1 {
				t.Errorf("generation record = %+v, %t, %v; want generation 1", generation, found, err)
			}
			first := h.settleRecord()
			if first.Round != 0 || !first.Settled {
				t.Errorf("record = round %d settled %t; want a settled round-0 record", first.Round, first.Settled)
			}

			h.skip = false
			h.resume()
			ptr = shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if ptr.Path != "" {
				t.Errorf("re-entry pointer = %q; want empty", ptr.Path)
			}
			if len(recorder.entries) != 0 {
				t.Errorf("carry-over entries = %d; want none", len(recorder.entries))
			}
			h.requireNoSpawn()
			if second := h.settleRecord(); !second.Settled || second.SettledAt != first.SettledAt {
				t.Errorf("record = settled %t at %q; want settled at the first write's %q", second.Settled, second.SettledAt, first.SettledAt)
			}
		}},
		{"SkipSeamStillApprovingNeverCarriesOver", func(t *testing.T) {
			var recorder carryOverRecorder
			h := newReentry(t, convergingShuttle(nil), func(h *reentry, cfg *BouncerConfig) {
				recorder.install(cfg)
				cfg.Skip = func() (bool, error) { return true, nil }
			})
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if len(recorder.entries) != 0 {
				t.Errorf("carry-over entries = %d; want none", len(recorder.entries))
			}
			if got := countCalls(h.log, "commit"); got != 2 {
				t.Errorf("Commit ran %d times; want 2", got)
			}
		}},
		{"DocumentSettleMarksTheBaseItReadsAfterCommit", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil), func(h *reentry, cfg *BouncerConfig) {
				cfg.SettledBase = func() (string, error) { return "base-after-commit", nil }
			})
			if err := h.settleRoundOne(errReentryCommit); !errors.Is(err, errReentryCommit) {
				t.Fatalf("first Call() error = %v; want the commit failure", err)
			}
			if record := h.settleRecord(); record.Base != "base-1" || record.Settled {
				t.Errorf("interrupted record = base %q settled %t; want the first base base-1, unsettled", record.Base, record.Settled)
			}

			h.resume()
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if record := h.settleRecord(); record.Base != "base-after-commit" || !record.Settled {
				t.Errorf("settled record = base %q settled %t; want the base read after Commit, settled", record.Base, record.Settled)
			}
		}},
		{"CodeSubjectWithMovedMergeBaseAndMergeInOnlyHistoryReSettles", func(t *testing.T) {
			var sinces []string
			h := newReentry(t, convergingShuttle(nil), func(h *reentry, cfg *BouncerConfig) {
				cfg.MergeInOnly = func(since string) (bool, error) {
					sinces = append(sinces, since)
					return true, nil
				}
			})
			h.subject = codeSubject
			if err := h.settleRoundOne(errReentryCommit); !errors.Is(err, errReentryCommit) {
				t.Fatalf("first Call() error = %v; want the commit failure", err)
			}
			if patch, err := os.ReadFile(filepath.Join(h.cfg.RunDir, settlePatchFile)); err != nil || string(patch) != "diff" {
				t.Errorf("settle patch = %q, %v; want the full diff", patch, err)
			}

			h.subject = Subject{Kind: SubjectCode, MergeBase: "base-2", Head: "head-2", FullHash: "full-2", PatchID: "patch-1"}
			h.resume()
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			h.requireNoSpawn()
			if !slices.Equal(sinces, []string{"head-1"}) {
				t.Errorf("MergeInOnly asked since %v; want the settled head head-1", sinces)
			}
		}},
		{"EqualJudgeSubjectHarvestsTheSettledVerdict", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil))
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{
				round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONVERGED"), ledger: bouncerLedgerContent(1),
			}})
			if err := writeGenerationRecord(h.cfg.RunDir, generationRecord{Number: 1, JudgeSubject: h.subject}); err != nil {
				t.Fatalf("writeGenerationRecord() = %v; want nil", err)
			}
			h.resume()

			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			h.requireNoSpawn()
			assertNoArchivedRunDirSibling(t, h.cfg.RunDir)
			if record := h.settleRecord(); record.Round != 1 || !record.Settled {
				t.Errorf("record = round %d settled %t; want a settled round-1 record", record.Round, record.Settled)
			}
		}},
		{"LaterRoundJudgedAndSettledAgain", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil))
			if err := h.settleRoundOne(nil); err != nil {
				t.Fatalf("first Call() error = %v; want nil", err)
			}
			first := h.settleRecord()
			h.now = h.now.Add(time.Hour)
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: 2, report: bouncerReport(2)}})

			ptr := shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if want := ledgerPath(h.cfg.RunDir, 2); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			second := h.settleRecord()
			if second.Round != 2 || second.SettledAt == first.SettledAt {
				t.Errorf("record = round %d at %q; want round 2 settled later than %q", second.Round, second.SettledAt, first.SettledAt)
			}
		}},
		{"UnharvestedConvergedLaterRoundSettlesWithNoArchiveAndNoSpawn", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil))
			if err := h.settleRoundOne(nil); err != nil {
				t.Fatalf("first Call() error = %v; want nil", err)
			}
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{
				round: 2, report: bouncerReport(2), verdict: bouncerVerdictContent("CONVERGED"), ledger: bouncerLedgerContent(2),
			}})
			h.resume()

			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			h.requireNoSpawn()
			assertNoArchivedRunDirSibling(t, h.cfg.RunDir)
			if record := h.settleRecord(); record.Round != 2 {
				t.Errorf("record round = %d; want 2", record.Round)
			}
		}},
		{"SubjectSeamAnsweringNoSubjectWritesNoRecordAndClears", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil))
			h.subjectOK = false
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{
				round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONVERGED"), ledger: bouncerLedgerContent(1),
			}})

			shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			if _, err := os.Stat(archivedRunDirPath(h.cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
				t.Errorf("archived run directory missing: %v; want the clear to have fired", err)
			}
			h.requireNoRecords()
		}},
		{"NilSubjectLeavesNoRecordAfterASeedAndAJudge", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil), func(h *reentry, cfg *BouncerConfig) { cfg.Subject = nil })
			shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			h.requireNoRecords()
		}},
		{"ToldSubjectSeedAndJudgeWriteTheGenerationRecord", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil))
			shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			generation, found, err := readGenerationRecord(h.cfg.RunDir)
			if err != nil || !found || generation.Number != 1 || generation.JudgeSubject.Kind != "" {
				t.Fatalf("generation record after the seed = %+v, %t, %v; want generation 1 with no judge subject", generation, found, err)
			}

			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			generation, _, err = readGenerationRecord(h.cfg.RunDir)
			wantAt := bouncerJudgeTestClock.Format(time.RFC3339)
			if err != nil || generation.JudgeSubject.Kind != SubjectDocument || generation.JudgeSubject.Hash != "hash-1" || generation.JudgeBase != "base-1" || generation.JudgeAt != wantAt {
				t.Errorf("generation record after the judge = %+v, %v; want the live subject, base base-1 and time %s", generation, err, wantAt)
			}
		}},
		{"SubjectReadErrorIsReturned", func(t *testing.T) {
			sentinel := errors.New("subject unreadable")
			h := newReentry(t, convergingShuttle(nil))
			h.subjectErr = sentinel
			if _, _, err := h.call(); !errors.Is(err, sentinel) {
				t.Errorf("Call() error = %v; want it to wrap %v", err, sentinel)
			}
			if len(h.shuttle.Specs) != 0 {
				t.Errorf("shuttle runs = %d; want none", len(h.shuttle.Specs))
			}
		}},
		{"CommitFailingAgainInsideTheReSettleLeavesTheRecordUnsettled", func(t *testing.T) {
			h := newReentry(t, convergingShuttle(nil))
			if err := h.settleRoundOne(errReentryCommit); !errors.Is(err, errReentryCommit) {
				t.Fatalf("first Call() error = %v; want the commit failure", err)
			}

			h.resume()
			h.commitErr = errReentryCommit
			if _, _, err := h.call(); !errors.Is(err, errReentryCommit) {
				t.Errorf("Call() error = %v; want the commit failure", err)
			}
			if record := h.settleRecord(); record.Settled {
				t.Error("record settled = true after Commit failed again; want false")
			}
			h.requireNoSpawn()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
