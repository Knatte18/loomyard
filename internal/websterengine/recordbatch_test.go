//go:build integration

// recordbatch_test.go exercises RecordBatch end to end (Tier 2 — see
// docs/benchmarks/running-tests.md): a real scratch git repo backs
// WorktreeRoot for the genuine ChangedFiles/Dirty drift computation, while
// the incremental fork audit (shuttleengine.Engine.AuditForksIncremental)
// is a local, call-scripted fake and SettleRetry's clock seam is a
// recording fake Sleeper that never actually blocks, mirroring
// audit_test.go's own SettleRetry fixture pattern (package-local — the
// internal and external test packages deliberately do not share a
// test-helper package).

package websterengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recordFakeSleeper is a Sleeper that never actually blocks — it only
// records each requested duration, so SettleRetry's retry loop runs a
// scripted sequence of "attempts" at zero real wall-clock cost.
type recordFakeSleeper struct {
	slept []time.Duration
}

func (s *recordFakeSleeper) Sleep(d time.Duration) {
	s.slept = append(s.slept, d)
}

var _ websterengine.Sleeper = (*recordFakeSleeper)(nil)

// recordFakeEngine is a hermetic shuttleengine.Engine double: AuditForksIncremental
// returns scripted[callCount] on each call (clamped to the last entry once the
// script is exhausted), so a test can drive a settle-retry sequence (an empty
// miss followed by a hit, or a stable audit across repeated calls) without any
// real transcript files. Every other method is unreached by RecordBatch's own
// path and returns a fixed, inert value.
type recordFakeEngine struct {
	scripted  []shuttleengine.ForkAudit
	callCount int
	// sessions records the sessionID of every AuditForksIncremental call, so a
	// test can assert WHICH session the audit was keyed on (the
	// bracket-opening session, never blindly the current Master session).
	sessions []string
}

func (e *recordFakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	idx := e.callCount
	if idx >= len(e.scripted) {
		idx = len(e.scripted) - 1
	}
	e.callCount++
	e.sessions = append(e.sessions, sessionID)
	return e.scripted[idx], nil
}

func (e *recordFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	return shuttleengine.Launch{}, nil
}
func (e *recordFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) { return nil, nil }
func (e *recordFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}
func (e *recordFakeEngine) InterruptSequence() []shuttleengine.PaneInput    { return nil }
func (e *recordFakeEngine) TrustDismissSequence() []shuttleengine.PaneInput { return nil }
func (e *recordFakeEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return nil
}
func (e *recordFakeEngine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *recordFakeEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return nil
}

var _ shuttleengine.Engine = (*recordFakeEngine)(nil)

// recordFixture is a fully-wired set of RecordBatch dependencies: a real
// scratch git repo (one base commit plus one in-scope work commit) as
// WorktreeRoot, a literal one-batch plan with an already-open BatchState
// (the begin-batch record RecordBatch's bracket-discipline check requires),
// and a scripted fake engine plus recording Sleeper.
type recordFixture struct {
	Deps       websterengine.RecordDeps
	Engine     *recordFakeEngine
	Sleeper    *recordFakeSleeper
	Worktree   string
	ReportsDir string
	StartSHA   string
}

// newRecordFixture builds a fresh recordFixture whose engine is scripted with
// scripted — the caller supplies the AuditForksIncremental sequence its own
// test needs.
func newRecordFixture(t *testing.T, scripted []shuttleengine.ForkAudit) *recordFixture {
	t.Helper()

	worktree := newScratchRepo(t)
	startSHA := commitFile(t, worktree, "base.txt", "base", "base commit")
	commitFile(t, worktree, "internal/foo/impl.go", "package foo\n", "01.1: add impl")

	plan := &builderengine.Plan{
		Batches: []builderengine.PlanBatch{
			{Number: 1, Slug: "json-flag", File: "01-json-flag.md", Scope: []string{"internal/foo"}},
		},
	}

	reportsDir := t.TempDir()
	contractDir := t.TempDir()

	engine := &recordFakeEngine{scripted: scripted}
	sleeper := &recordFakeSleeper{}
	layout := &hubgeometry.Layout{WorktreeRoot: worktree, Cwd: worktree}

	state := &websterengine.State{
		MasterSessionID: "session-1",
		CurrentBatch:    1,
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "json-flag", StartSHA: startSHA, Kind: "fork", SessionID: "session-1"},
		},
	}

	deps := websterengine.RecordDeps{
		Plan:         plan,
		State:        state,
		Config:       websterengine.Config{},
		Engine:       engine,
		Layout:       layout,
		WorktreeRoot: worktree,
		ReportsDir:   reportsDir,
		OutcomePath:  filepath.Join(contractDir, "outcome.yaml"),
		SummaryPath:  filepath.Join(contractDir, "summary.md"),
		Sleeper:      sleeper,
	}

	return &recordFixture{Deps: deps, Engine: engine, Sleeper: sleeper, Worktree: worktree, ReportsDir: reportsDir, StartSHA: startSHA}
}

// writeReport seeds fx's reportsDir with a batch-report YAML file for batch
// 1 at its plan-format-pinned filename, using content verbatim.
func writeReport(t *testing.T, reportsDir, content string) {
	t.Helper()
	path := filepath.Join(reportsDir, builderengine.BatchReportFileName(1, "json-flag"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

const validReportYAML = "batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n"

// TestRecordBatch_NoBeginRecord proves the bracket-discipline check: a
// record call with no matching BatchState entry, or one already Terminal, is
// refused with ErrNoBeginRecord before the audit is ever consulted.
func TestRecordBatch_NoBeginRecord(t *testing.T) {
	t.Run("absent BatchState", func(t *testing.T) {
		fx := newRecordFixture(t, nil)
		fx.Deps.State.Batches = map[int]*websterengine.BatchState{}

		_, err := websterengine.RecordBatch(fx.Deps, 1)
		if !errors.Is(err, websterengine.ErrNoBeginRecord) {
			t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrNoBeginRecord)", err)
		}
		if fx.Engine.callCount != 0 {
			t.Errorf("Engine was reached (%d calls) with no begin record; want zero", fx.Engine.callCount)
		}
	})

	t.Run("already Terminal BatchState", func(t *testing.T) {
		fx := newRecordFixture(t, nil)
		fx.Deps.State.Batches[1].Terminal = true

		_, err := websterengine.RecordBatch(fx.Deps, 1)
		if !errors.Is(err, websterengine.ErrNoBeginRecord) {
			t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrNoBeginRecord)", err)
		}
	})
}

// TestRecordBatch_RecoveryBatchRefusedLoud proves the kind guard: a recovery
// batch's report is recover-batch's to classify, never record-batch's — the
// refusal names the correct verb instead of dying later in the fork audit
// with a misleading "never forked" error (round fable-r3 live).
func TestRecordBatch_RecoveryBatchRefusedLoud(t *testing.T) {
	fx := newRecordFixture(t, nil)
	fx.Deps.State.Batches[1].Kind = "recovery"

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil || !strings.Contains(err.Error(), "recover-batch") {
		t.Fatalf("RecordBatch() error = %v; want a refusal naming recover-batch", err)
	}
	if fx.Engine.callCount != 0 {
		t.Errorf("Engine was reached (%d calls) for a recovery batch; want zero", fx.Engine.callCount)
	}
}

// TestRecordBatch_AuditsBracketOpeningSession proves the crash/resume seam:
// the fork audit keys on the session recorded in the batch's begin record
// (bs.SessionID), never blindly on the CURRENT Master session — a resumed
// run's fresh Master must be able to consume a report whose fork transcript
// lives under the crashed session's own subagents directory (round fable-r3
// live: auditing the current session instead wedged that resume across all
// three verbs).
func TestRecordBatch_AuditsBracketOpeningSession(t *testing.T) {
	audit := shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/crashed-session/subagents/f1.jsonl", ReportReturned: true}},
	}
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{audit})
	fx.Deps.State.MasterSessionID = "session-resumed"
	fx.Deps.State.Batches[1].SessionID = "session-crashed"
	writeReport(t, fx.ReportsDir, validReportYAML)

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != builderengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
	for i, session := range fx.Engine.sessions {
		if session != "session-crashed" {
			t.Errorf("AuditForksIncremental call %d keyed on session %q; want the bracket-opening \"session-crashed\"", i, session)
		}
	}
}

// TestRecordBatch_ZeroNewTranscriptsHardErrorsEvenWithReport proves the
// unfakeable-report rule: zero new transcripts through the whole settle
// window is a hard error REGARDLESS of a batch-report file already sitting
// on disk — a report with no fork behind it means Master wrote it itself.
func TestRecordBatch_ZeroNewTranscriptsHardErrorsEvenWithReport(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{{}})
	writeReport(t, fx.ReportsDir, validReportYAML)

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrNoForkTranscripts) {
		t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrNoForkTranscripts)", err)
	}
	if len(fx.Sleeper.slept) == 0 {
		t.Errorf("Sleeper.slept is empty; want the settle window's retry ticks recorded")
	}
}

// TestRecordBatch_TranscriptAppearsOnLaterTick proves a fork transcript that
// only appears on the fetch AFTER the first miss still resolves clean —
// SettleRetry's own "first miss is inconclusive" de-risk applied through the
// whole RecordBatch call.
func TestRecordBatch_TranscriptAppearsOnLaterTick(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{},
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/late.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReportYAML)

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil {
		t.Fatal("RecordResult.Digest = nil; want a distilled digest")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("RecordResult.Warnings = %v; want none", result.Warnings)
	}
	if len(fx.Sleeper.slept) != 1 {
		t.Errorf("Sleeper.slept = %v; want exactly one tick before the late transcript resolved", fx.Sleeper.slept)
	}
}

// TestRecordBatch_OneNewTranscriptWithReport_TerminalDigestPersisted proves
// the normal happy path: one new transcript plus a valid, matching report
// distills and persists the digest, marks the batch Terminal, clears
// CurrentBatch, and attributes the transcript to both SeenForkTranscripts
// and the batch's own ForkTranscripts.
func TestRecordBatch_OneNewTranscriptWithReport_TerminalDigestPersisted(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReportYAML)

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.NoReport {
		t.Fatal("RecordResult.NoReport = true; want false (a valid report was present)")
	}
	if result.Digest == nil || result.Digest.Status != builderengine.DigestStatusDone {
		t.Fatalf("RecordResult.Digest = %+v; want a done digest", result.Digest)
	}
	if result.Digest.FilesChanged != 1 {
		t.Errorf("Digest.FilesChanged = %d; want 1 (internal/foo/impl.go)", result.Digest.FilesChanged)
	}

	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal {
		t.Error("BatchState.Terminal = false; want true")
	}
	if bs.Status != builderengine.DigestStatusDone {
		t.Errorf("BatchState.Status = %q; want %q", bs.Status, builderengine.DigestStatusDone)
	}
	if bs.Digest == nil {
		t.Error("BatchState.Digest = nil; want the persisted digest")
	}
	if fx.Deps.State.CurrentBatch != 0 {
		t.Errorf("State.CurrentBatch = %d; want 0 (cleared)", fx.Deps.State.CurrentBatch)
	}

	wantTranscript := "subagents/f1.jsonl"
	if len(fx.Deps.State.SeenForkTranscripts) != 1 || fx.Deps.State.SeenForkTranscripts[0] != wantTranscript {
		t.Errorf("State.SeenForkTranscripts = %v; want [%q]", fx.Deps.State.SeenForkTranscripts, wantTranscript)
	}
	if len(bs.ForkTranscripts) != 1 || bs.ForkTranscripts[0] != wantTranscript {
		t.Errorf("BatchState.ForkTranscripts = %v; want [%q]", bs.ForkTranscripts, wantTranscript)
	}
}

// TestRecordBatch_OneNewTranscriptNoReport_RetrySeesExactlyOneNew proves the
// no_report ladder: a fork transcript with no report yet returns NoReport
// true, leaves the batch non-terminal, and STILL advances attribution — so a
// second record-batch call (after Master's re-fork) sees exactly its own new
// transcript and resolves clean, never re-counting the first one.
func TestRecordBatch_OneNewTranscriptNoReport_RetrySeesExactlyOneNew(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
		{Forks: []shuttleengine.ForkReport{
			{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true},
			{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true},
		}},
	})

	// First call: no report on disk yet.
	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() first call error = %v; want nil", err)
	}
	if !result.NoReport {
		t.Fatal("RecordResult.NoReport = false; want true (no report file yet)")
	}
	bs := fx.Deps.State.Batches[1]
	if bs.Terminal {
		t.Error("BatchState.Terminal = true after a no_report call; want false")
	}
	if len(fx.Deps.State.SeenForkTranscripts) != 1 || fx.Deps.State.SeenForkTranscripts[0] != "subagents/f1.jsonl" {
		t.Fatalf("State.SeenForkTranscripts after no_report call = %v; want attribution still advanced to [f1]", fx.Deps.State.SeenForkTranscripts)
	}

	// Second call: the re-fork's report has now landed. The fake engine's
	// second script entry still includes f1, proving SettleRetry's own
	// defensive re-filter (against the just-updated SeenForkTranscripts) is
	// what keeps this call's classification down to exactly one new
	// transcript (f2), not two.
	writeReport(t, fx.ReportsDir, validReportYAML)
	result, err = websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() second call error = %v; want nil (clean, exactly one new transcript)", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("second call Warnings = %v; want none (exactly one new transcript, not two)", result.Warnings)
	}
	if !bs.Terminal {
		t.Error("BatchState.Terminal = false after the second call's valid report; want true")
	}
	wantTranscripts := []string{"subagents/f1.jsonl", "subagents/f2.jsonl"}
	if len(bs.ForkTranscripts) != len(wantTranscripts) {
		t.Errorf("BatchState.ForkTranscripts = %v; want %v", bs.ForkTranscripts, wantTranscripts)
	}
}

// TestRecordBatch_MultipleNewTranscriptsWarnsNeverErrors proves more than one
// new transcript in a single call is a warning only, never a hard error —
// legitimate retry behavior (a fork's Agent call errored mid-flight followed
// by a direct re-fork, with no intervening record-batch call).
func TestRecordBatch_MultipleNewTranscriptsWarnsNeverErrors(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{
			{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true},
			{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true},
		}},
	})
	writeReport(t, fx.ReportsDir, validReportYAML)

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil (multi-new is a warning, never an error)", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("RecordResult.Warnings is empty; want the multi-new-transcript warning")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "2 new fork transcripts") {
			found = true
		}
	}
	if !found {
		t.Errorf("RecordResult.Warnings = %v; want one naming 2 new fork transcripts", result.Warnings)
	}
}

// TestRecordBatch_ParentWriteOutsideContractFilesErrors proves CheckParent's
// write-policy violation is surfaced as a hard error naming the offending
// write, even when the fork-transcript count and the report itself are both
// otherwise clean.
func TestRecordBatch_ParentWriteOutsideContractFilesErrors(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{
			Forks:        []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
			ParentWrites: []string{"/some/other/hand-written-file.go"},
		},
	})
	writeReport(t, fx.ReportsDir, validReportYAML)

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a hard error for a parent write outside the two contract files")
	}
	if !strings.Contains(err.Error(), "hand-written-file.go") {
		t.Errorf("RecordBatch() error = %q; want it to name the offending parent write", err.Error())
	}
}

// TestRecordBatch_ReportBatchFieldMismatchErrors proves a batch-report whose
// own batch: field disagrees with the polled batch ID is a hard error,
// naming both.
func TestRecordBatch_ReportBatchFieldMismatchErrors(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, "batch: 99-wrong-batch\nstatus: done\ntests: green\nstuck_reason: null\n")

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a hard error for a batch-field mismatch")
	}
	if !strings.Contains(err.Error(), "01-json-flag") || !strings.Contains(err.Error(), "99-wrong-batch") {
		t.Errorf("RecordBatch() error = %q; want it to name both the polled batch and the report's own field", err.Error())
	}
}

// TestRecordBatch_MalformedReportYAMLErrors proves an unparseable
// batch-report (here, an unrecognized status value) is a hard error, never a
// guessed digest.
func TestRecordBatch_MalformedReportYAMLErrors(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, "batch: 01-json-flag\nstatus: bogus\ntests: green\nstuck_reason: null\n")

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a hard error for an unrecognized status value")
	}
}
