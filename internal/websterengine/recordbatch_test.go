//go:build integration

// recordbatch_test.go exercises RecordBatch end to end (Tier 2 — see
// docs/benchmarks/running-tests.md): a real scratch git repo backs
// WorktreeRoot for the genuine changedFiles/dirty/headSHA drift computation,
// while the incremental fork audit
// (shuttleengine.Engine.AuditForksIncremental) is a local, call-scripted
// fake and SettleRetry's clock seam is a recording fake Sleeper that never
// actually blocks, mirroring audit_test.go's own SettleRetry fixture
// pattern (package-local — the internal and external test packages
// deliberately do not share a test-helper package).

package websterengine_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/planparser"
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
	// auditErr, when non-nil, is returned by every AuditForksIncremental call
	// instead of the script — the missing-transcript failure double for the
	// cross-machine resume path.
	auditErr error
}

func (e *recordFakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	e.callCount++
	e.sessions = append(e.sessions, sessionID)
	if e.auditErr != nil {
		return shuttleengine.ForkAudit{}, e.auditErr
	}
	idx := e.callCount - 1
	if idx >= len(e.scripted) {
		idx = len(e.scripted) - 1
	}
	return e.scripted[idx], nil
}

func (e *recordFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	return shuttleengine.Launch{}, nil
}
func (e *recordFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) { return nil, nil }
func (e *recordFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}
func (e *recordFakeEngine) InterruptSequence() []shuttleengine.PaneInput          { return nil }
func (e *recordFakeEngine) TrustDismissSequence(string) []shuttleengine.PaneInput { return nil }
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
// WorktreeRoot, a literal one-batch execution-batch list with an
// already-open BatchState (the begin-batch record RecordBatch's
// bracket-discipline check requires), and a scripted fake engine plus
// recording Sleeper. HeadSHA is the worktree's actual current HEAD (the
// work commit) — every valid report fixture must self-report exactly this
// SHA, since RecordBatch cross-checks report.HeadSHA against the worktree's
// real HEAD.
type recordFixture struct {
	Deps       websterengine.RecordDeps
	Engine     *recordFakeEngine
	Sleeper    *recordFakeSleeper
	Worktree   string
	ReportsDir string
	StartSHA   string
	HeadSHA    string
}

// newRecordFixture builds a fresh recordFixture whose engine is scripted with
// scripted — the caller supplies the AuditForksIncremental sequence its own
// test needs.
func newRecordFixture(t *testing.T, scripted []shuttleengine.ForkAudit) *recordFixture {
	t.Helper()

	worktree := newScratchRepo(t)
	startSHA := commitFile(t, worktree, "base.txt", "base", "base commit")
	headSHA := commitFile(t, worktree, "internal/foo/impl.go", "package foo\n", "01.1: add impl")

	cards := []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}
	batches := []batcher.Batch{{Cards: cards}}
	plan := &planparser.Plan{Format: 5, Cards: cards}

	reportsDir := t.TempDir()
	contractDir := t.TempDir()
	// A real (empty) plan directory: RecordBatch re-baselines the plan fingerprint over it after its
	// own BindHandles and DetectDrift rewrites, so this is a genuine read rather than an invented
	// path. No card here declares a handle, so nothing is ever written into it.
	planDir := t.TempDir()

	engine := &recordFakeEngine{scripted: scripted}
	sleeper := &recordFakeSleeper{}

	state := &websterengine.State{
		MasterSessionID: "session-1",
		CurrentBatch:    1,
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "json-flag", StartSHA: startSHA, Kind: "fork", SessionID: "session-1"},
		},
	}

	deps := websterengine.RecordDeps{
		Batches: batches,
		State:   state,
		Config:  websterengine.Config{},
		Engine:  engine,
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			ReportsDir:   reportsDir,
			PlanDir:      planDir,
		},
		RefMatcher:  websterengine.NeverMatches{},
		OutcomePath: filepath.Join(contractDir, "outcome.yaml"),
		SummaryPath: filepath.Join(contractDir, "summary.md"),
		Sleeper:     sleeper,
		Plan:        plan,
	}

	return &recordFixture{Deps: deps, Engine: engine, Sleeper: sleeper, Worktree: worktree, ReportsDir: reportsDir, StartSHA: startSHA, HeadSHA: headSHA}
}

// addPendingCard appends a second card to fx's plan, in its own second batch that has no BatchState
// and is therefore not terminal, referencing uses.
//
// Drift is defined against the REMAINING plan, and the batch being recorded is not remaining — its
// work is exactly what the delta reports. A drift fixture therefore needs a genuinely pending card
// to hold the reference; putting it on batch 1's own card tests a card drifting against itself,
// which is never drift.
func addPendingCard(fx *recordFixture, uses []string) {
	pending := planparser.Card{Number: 2, Slug: "pending", Title: "pending", Intent: "the not-yet-built card", Uses: uses}
	fx.Deps.Plan.Cards = append(fx.Deps.Plan.Cards, pending)
	fx.Deps.Batches = append(fx.Deps.Batches, batcher.Batch{Cards: []planparser.Card{pending}})
}

// writeReport seeds fx's reportsDir with a batch-report YAML file for batch
// 1 at its plan-format-pinned filename, using content verbatim.
func writeReport(t *testing.T, reportsDir, content string) {
	t.Helper()
	path := filepath.Join(reportsDir, websterengine.ReportFileName(1, "json-flag"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// validReport returns a well-formed minimal report YAML self-reporting
// headSHA — the caller must pass fx.HeadSHA (the worktree's actual current
// HEAD) so RecordBatch's head_sha cross-check passes.
func validReport(headSHA string) string {
	return "status: OK\nhead_sha: " + headSHA + "\n"
}

// TestRecordBatch_NoBeginRecord proves the bracket-discipline check: a record call with no matching
// BatchState entry never consults the audit.
// A report present is archived and the batch re-driven through begin-batch, so begin-batch's
// pre-existing-report refusal no longer fires; with no report the error still names begin-batch.
func TestRecordBatch_NoBeginRecord(t *testing.T) {
	t.Run("report present is archived", func(t *testing.T) {
		fx := newRecordFixture(t, nil)
		fx.Deps.State.Batches = map[int]*websterengine.BatchState{}
		writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if !errors.Is(err, websterengine.ErrReportArchived) || !errors.Is(err, websterengine.ErrNoBeginRecord) {
			t.Fatalf("RecordBatch() error = %v; want ErrReportArchived wrapping ErrNoBeginRecord", err)
		}
		if !strings.Contains(err.Error(), "lyx webster begin-batch 01") {
			t.Errorf("RecordBatch() error = %q; want it to name `lyx webster begin-batch 01`", err.Error())
		}
		if fx.Engine.callCount != 0 {
			t.Errorf("Engine was reached (%d calls) with no begin record; want zero", fx.Engine.callCount)
		}
		archived := archivedReports(t, fx.ReportsDir)
		if len(archived) != 1 || result == nil || result.ArchivedReport == "" {
			t.Errorf("archived reports = %v, result = %+v; want one archived report surfaced on the result", archived, result)
		}
		if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
			t.Errorf("report path still occupied (stat err = %v); want it free for begin-batch", statErr)
		}
	})

	t.Run("no report names begin-batch", func(t *testing.T) {
		fx := newRecordFixture(t, nil)
		fx.Deps.State.Batches = map[int]*websterengine.BatchState{}

		_, err := websterengine.RecordBatch(fx.Deps, 1)
		if !errors.Is(err, websterengine.ErrNoBeginRecord) {
			t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrNoBeginRecord)", err)
		}
		if !strings.Contains(err.Error(), "lyx webster begin-batch 01") {
			t.Errorf("RecordBatch() error = %q; want it to name `lyx webster begin-batch 01`", err.Error())
		}
	})

	t.Run("terminal done refuses and leaves the report", func(t *testing.T) {
		fx := newRecordFixture(t, nil)
		fx.Deps.State.Batches[1].Terminal = true
		fx.Deps.State.Batches[1].Status = websterengine.DigestStatusDone
		writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

		_, err := websterengine.RecordBatch(fx.Deps, 1)
		if err == nil || !strings.Contains(err.Error(), "next batch") {
			t.Fatalf("RecordBatch() error = %v; want a refusal naming the next batch", err)
		}
		if errors.Is(err, websterengine.ErrNoBeginRecord) {
			t.Errorf("RecordBatch() error = %v; a terminal batch must not share ErrNoBeginRecord", err)
		}
		if got := archivedReports(t, fx.ReportsDir); len(got) != 0 {
			t.Errorf("archived reports = %v; want none — the report stays in place", got)
		}
	})

	t.Run("terminal failed names recover-batch", func(t *testing.T) {
		fx := newRecordFixture(t, nil)
		fx.Deps.State.Batches[1].Terminal = true
		fx.Deps.State.Batches[1].Status = websterengine.DigestStatusFailed

		_, err := websterengine.RecordBatch(fx.Deps, 1)
		if err == nil || !strings.Contains(err.Error(), "lyx webster recover-batch 01") {
			t.Fatalf("RecordBatch() error = %v; want a refusal naming recover-batch", err)
		}
	})
}

// TestRecordBatch_RecoveryBatchRefusedLoud proves the kind guard: a recovery batch's report is
// recover-batch's to classify, never record-batch's — the refusal names the correct verb instead of
// dying later in the fork audit with a misleading "never forked" error (round fable-r3 live).
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

// TestRecordBatch_AuditsBracketOpeningSession proves the crash/resume seam: the fork audit keys on
// the session recorded in the batch's begin record (bs.SessionID), never blindly on the CURRENT
// Master session — a resumed run's fresh Master must be able to consume a report whose fork
// transcript lives under the crashed session's own subagents directory (round fable-r3 live:
// auditing the current session instead wedged that resume across all three verbs).
func TestRecordBatch_AuditsBracketOpeningSession(t *testing.T) {
	audit := shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/crashed-session/subagents/f1.jsonl", ReportReturned: true}},
	}
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{audit})
	fx.Deps.State.MasterSessionID = "session-resumed"
	fx.Deps.State.Batches[1].SessionID = "session-crashed"
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
	for i, session := range fx.Engine.sessions {
		if session != "session-crashed" {
			t.Errorf("AuditForksIncremental call %d keyed on session %q; want the bracket-opening \"session-crashed\"", i, session)
		}
	}
}

// TestRecordBatch_ZeroNewTranscriptsArchivesReport proves the unfakeable-report rule:
// zero new transcripts through the whole settle window never records the report, REGARDLESS of a
// batch-report file already sitting on disk — a report with no fork behind it means Master wrote it itself.
// The report is archived, the batch record stays begun, and begin-batch re-drives it.
func TestRecordBatch_ZeroNewTranscriptsArchivesReport(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{{}})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrReportArchived) || !errors.Is(err, websterengine.ErrNoForkTranscripts) {
		t.Fatalf("RecordBatch() error = %v; want ErrReportArchived wrapping ErrNoForkTranscripts", err)
	}
	if !strings.Contains(err.Error(), "lyx webster begin-batch 01") {
		t.Errorf("RecordBatch() error = %q; want it to name `lyx webster begin-batch 01`", err.Error())
	}
	if len(fx.Sleeper.slept) == 0 {
		t.Errorf("Sleeper.slept is empty; want the settle window's retry ticks recorded")
	}
	if got := archivedReports(t, fx.ReportsDir); len(got) != 1 || result == nil || result.ArchivedReport == "" {
		t.Errorf("archived reports = %v, result = %+v; want one archived report surfaced on the result", got, result)
	}
	bs := fx.Deps.State.Batches[1]
	if bs.Terminal || bs.StartSHA != fx.StartSHA {
		t.Errorf("BatchState = %+v; want it still begun, non-terminal, with StartSHA %q kept", bs, fx.StartSHA)
	}
	if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
		t.Errorf("report path still occupied (stat err = %v); want it free for begin-batch", statErr)
	}
}

// TestRecordBatch_TranscriptAppearsOnLaterTick proves a fork transcript that only appears on the
// fetch AFTER the first miss still resolves clean — SettleRetry's own "first miss is inconclusive"
// de-risk applied through the whole RecordBatch call.
func TestRecordBatch_TranscriptAppearsOnLaterTick(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{},
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/late.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

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

// TestRecordBatch_OneNewTranscriptWithReport_TerminalDigestPersisted proves the normal happy path:
// one new transcript plus a valid, matching report distills and persists the digest, marks the
// batch Terminal, clears CurrentBatch, and attributes the transcript to both SeenForkTranscripts
// and the batch's own ForkTranscripts.
func TestRecordBatch_OneNewTranscriptWithReport_TerminalDigestPersisted(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.NoReport {
		t.Fatal("RecordResult.NoReport = true; want false (a valid report was present)")
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordResult.Digest = %+v; want a done digest", result.Digest)
	}
	if result.Digest.Batch != "01-json-flag" {
		t.Errorf("Digest.Batch = %q; want %q", result.Digest.Batch, "01-json-flag")
	}
	if result.Digest.HeadSHA != fx.HeadSHA {
		t.Errorf("Digest.HeadSHA = %q; want %q", result.Digest.HeadSHA, fx.HeadSHA)
	}

	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal {
		t.Error("BatchState.Terminal = false; want true")
	}
	if bs.Status != websterengine.DigestStatusDone {
		t.Errorf("BatchState.Status = %q; want %q", bs.Status, websterengine.DigestStatusDone)
	}
	if bs.Digest == nil {
		t.Error("BatchState.Digest = nil; want the persisted digest")
	}
	if len(bs.CardSHAs) != 1 || bs.CardSHAs[0] != fx.HeadSHA {
		t.Errorf("BatchState.CardSHAs = %v; want [%q]", bs.CardSHAs, fx.HeadSHA)
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

// TestRecordBatch_OneNewTranscriptNoReport_RetrySeesExactlyOneNew proves the no_report ladder: a
// fork transcript with no report yet returns NoReport true, leaves the batch non-terminal, and
// STILL advances attribution — so a second record-batch call (after Master's re-fork) sees exactly
// its own new transcript and resolves clean, never re-counting the first one.
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
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
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

// TestRecordBatch_ReportPresentDropsNeverReturnedWarning proves the report file is the fork's
// contract: a fork whose transcript never ended on a final report but whose report file landed
// draws no "never returned a final report" warning.
func TestRecordBatch_ReportPresentDropsNeverReturnedWarning(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: false}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.NoReport {
		t.Fatal("RecordResult.NoReport = true; want false (report file present)")
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "never returned a final report") {
			t.Errorf("Warnings = %v; want no \"never returned a final report\" warning with the report present", result.Warnings)
		}
	}
}

// TestRecordBatch_NoReportKeepsNeverReturnedWarning proves the warning still fires on the
// no-report path, where it explains why Master must re-fork.
func TestRecordBatch_NoReportKeepsNeverReturnedWarning(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: false}}},
	})

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if !result.NoReport {
		t.Fatal("RecordResult.NoReport = false; want true (no report file)")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "never returned a final report") {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings = %v; want the \"never returned a final report\" warning on the no-report path", result.Warnings)
	}
}

// TestRecordBatch_MultipleNewTranscriptsWarnsNeverErrors proves more than one new transcript in a
// single call is a warning only, never a hard error — legitimate retry behavior (a fork's Agent
// call errored mid-flight followed by a direct re-fork, with no intervening record-batch call).
func TestRecordBatch_MultipleNewTranscriptsWarnsNeverErrors(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{
			{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true},
			{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true},
		}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

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

// TestRecordBatch_ParentWriteOutsideWorktreeWarns proves a parent write outside the worktree is a
// policy finding: the batch records done with one recorded warning naming the write.
func TestRecordBatch_ParentWriteOutsideWorktreeWarns(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{
			Forks:        []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
			ParentWrites: []string{"/some/other/hand-written-file.go"},
		},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil (a write outside the worktree is policy)", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("Digest = %+v; want done", result.Digest)
	}
	if !warningsContain(result.Warnings, "audit warning (parent-write)", "hand-written-file.go") {
		t.Errorf("Warnings = %v; want a parent-write audit warning naming the write", result.Warnings)
	}
}

// fabricMatcher is a RefMatcher that matches any command containing "FABRICREF".
type fabricMatcher struct{}

func (fabricMatcher) Matches(cmd string) bool { return strings.Contains(cmd, "FABRICREF") }

// warningsContain reports whether any warning contains every one of subs.
func warningsContain(warnings []string, subs ...string) bool {
	for _, w := range warnings {
		ok := true
		for _, s := range subs {
			if !strings.Contains(w, s) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// setCardVerify gives fx's only card the verify command cmd, in the batch list RecordBatch reads.
func setCardVerify(fx *recordFixture, cmd string) {
	fx.Deps.Batches[0].Cards[0].Verify = cmd
	fx.Deps.Batches[0].Cards[0].HasVerify = true
}

// forkFabricRefAudit scripts one fork transcript whose Bash ran a fabric-referencing command.
func forkFabricRefAudit() shuttleengine.ForkAudit {
	return shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{
		TranscriptPath: "subagents/f1.jsonl",
		ReportReturned: true,
		BashCommands:   []string{"lyx FABRICREF sync"},
	}}}
}

// archivedReports lists the archived copies of batch 1's report.
func archivedReports(t *testing.T, reportsDir string) []string {
	t.Helper()
	archived, err := filepath.Glob(filepath.Join(reportsDir, "01-json-flag-*.yaml"))
	if err != nil {
		t.Fatalf("glob archived reports: %v", err)
	}
	return archived
}

// TestRecordBatch_ForkFabricReferenceWarnsWhenVerifyPasses proves a policy finding with an OK report
// and a passing card verify records the batch done with exactly one recorded warning.
func TestRecordBatch_ForkFabricReferenceWarnsWhenVerifyPasses(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{forkFabricRefAudit()})
	fx.Deps.RefMatcher = fabricMatcher{}
	setCardVerify(fx, "exit 0")
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("Digest = %+v; want done", result.Digest)
	}
	n := 0
	for _, w := range result.Warnings {
		if strings.Contains(w, "audit warning (fabric-reference)") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Warnings = %v; want exactly one audit warning (fabric-reference)", result.Warnings)
	}
	if got := fx.Deps.State.Batches[1].AuditWarnings; len(got) != 1 {
		t.Errorf("AuditWarnings = %v; want one entry", got)
	}
}

// TestRecordBatch_RetryNeverDuplicatesWarning proves a finding first seen on a no-report call is
// warned once: the later OK report re-runs the verify and records done without a second warning.
func TestRecordBatch_RetryNeverDuplicatesWarning(t *testing.T) {
	audit := shuttleengine.ForkAudit{
		Forks:              []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
		ParentBashCommands: []string{"lyx FABRICREF sync"},
	}
	audit2 := audit
	audit2.Forks = []shuttleengine.ForkReport{
		{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true},
		{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true},
	}
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{audit, audit2})
	fx.Deps.RefMatcher = fabricMatcher{}
	marker := filepath.Join(fx.Worktree, "verify-ran.marker")
	setCardVerify(fx, "touch verify-ran.marker")

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("first RecordBatch() error = %v; want nil", err)
	}
	if !result.NoReport {
		t.Fatal("first call NoReport = false; want true")
	}
	if got := fx.Deps.State.Batches[1].AuditWarnings; len(got) != 1 {
		t.Fatalf("AuditWarnings after first call = %v; want one", got)
	}

	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	result, err = websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("second RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("Digest = %+v; want done", result.Digest)
	}
	if got := fx.Deps.State.Batches[1].AuditWarnings; len(got) != 1 {
		t.Errorf("AuditWarnings after second call = %v; want still exactly one", got)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("verify marker missing: %v; want the card verify re-run on the OK report", statErr)
	}
}

// TestRecordBatch_RetryFailingVerifyFailsNamingEarlierWarning proves the same flow with a failing
// verify fails the batch, and the earlier recorded warning is among the reasons.
func TestRecordBatch_RetryFailingVerifyFailsNamingEarlierWarning(t *testing.T) {
	audit := shuttleengine.ForkAudit{
		Forks:              []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
		ParentBashCommands: []string{"lyx FABRICREF sync"},
	}
	audit2 := audit
	audit2.Forks = []shuttleengine.ForkReport{
		{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true},
		{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true},
	}
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{audit, audit2})
	fx.Deps.RefMatcher = fabricMatcher{}
	setCardVerify(fx, "exit 1")

	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Fatalf("first RecordBatch() error = %v; want nil", err)
	}
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("second RecordBatch() error = %v; want ErrBatchFailed", err)
	}
	if !result.Failed || result.Digest.Status != websterengine.DigestStatusFailed {
		t.Fatalf("result = %+v; want a failed digest", result)
	}
	if !warningsContain(result.Digest.Reasons, "audit warning (fabric-reference)") {
		t.Errorf("Reasons = %v; want the earlier warning among them", result.Digest.Reasons)
	}
	if !warningsContain(result.Digest.Reasons, "verify exit 1 exited 1") {
		t.Errorf("Reasons = %v; want the verify failure among them", result.Digest.Reasons)
	}
}

// TestRecordBatch_CorrectnessParentFindingNoReportFailsBatch proves a parent write to a tracked file
// fails the batch with no report at all, archiving nothing and naming the path in the reasons.
func TestRecordBatch_CorrectnessParentFindingNoReportFailsBatch(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
	}})
	tracked := filepath.Join(fx.Worktree, "internal", "foo", "impl.go")
	fx.Engine.scripted[0].ParentWrites = []string{tracked}

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecordBatch() error = %v; want ErrBatchFailed", err)
	}
	if !strings.Contains(err.Error(), "lyx webster recover-batch") {
		t.Errorf("error = %q; want it to name recover-batch", err.Error())
	}
	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed || !result.Failed {
		t.Errorf("batch = %+v, result = %+v; want terminal failed", bs, result)
	}
	if !warningsContain(result.Digest.Reasons, tracked) {
		t.Errorf("Reasons = %v; want the written path named", result.Digest.Reasons)
	}
	if got := archivedReports(t, fx.ReportsDir); len(got) != 0 {
		t.Errorf("archived reports = %v; want none (no report existed)", got)
	}
}

// TestRecordBatch_CorrectnessFindingFailedReportFailsBatch proves a correctness finding fails the
// batch even over a FAILED report, and archives that report.
func TestRecordBatch_CorrectnessFindingFailedReportFailsBatch(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
	}})
	fx.Engine.scripted[0].ParentWrites = []string{filepath.Join(fx.Worktree, "internal", "foo", "impl.go")}
	writeReport(t, fx.ReportsDir, "status: FAILED\nhead_sha: "+fx.HeadSHA+"\n")

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecordBatch() error = %v; want ErrBatchFailed", err)
	}
	if got := archivedReports(t, fx.ReportsDir); len(got) != 1 {
		t.Errorf("archived reports = %v; want exactly one", got)
	}
}

// TestRecordBatch_PolicyFindingFailingVerifySameCallFailsBatch proves step 7's first trigger: a fork
// fabric-reference on an OK report whose card verify exits 1 fails the batch on the same call.
func TestRecordBatch_PolicyFindingFailingVerifySameCallFailsBatch(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{forkFabricRefAudit()})
	fx.Deps.RefMatcher = fabricMatcher{}
	setCardVerify(fx, "exit 1")
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecordBatch() error = %v; want ErrBatchFailed", err)
	}
	if !strings.Contains(err.Error(), "lyx webster recover-batch") {
		t.Errorf("error = %q; want it to name recover-batch", err.Error())
	}
	if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
		t.Errorf("live report stat = %v; want it archived away", statErr)
	}
	if got := archivedReports(t, fx.ReportsDir); len(got) != 1 {
		t.Errorf("archived reports = %v; want exactly one", got)
	}
}

// TestRecordBatch_NamedSpawnWarnsOnceAcrossBatches proves a whole-session parent finding is
// dispositioned by the first record-batch that reports it: the next batch in the same session
// records done with no refusal and no repeated warning.
func TestRecordBatch_NamedSpawnWarnsOnceAcrossBatches(t *testing.T) {
	f1 := shuttleengine.ForkReport{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}
	f2 := shuttleengine.ForkReport{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true}
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{f1}, NamedSpawns: 1},
		{Forks: []shuttleengine.ForkReport{f1, f2}, NamedSpawns: 1},
	})
	addPendingCard(fx, nil)
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("batch 1 RecordBatch() error = %v; want nil", err)
	}
	if !warningsContain(result.Warnings, "audit warning (named-spawn)") {
		t.Fatalf("batch 1 Warnings = %v; want a named-spawn audit warning", result.Warnings)
	}

	fx.Deps.State.Batches[2] = &websterengine.BatchState{Slug: "pending", StartSHA: fx.StartSHA, Kind: "fork", SessionID: "session-1"}
	fx.Deps.State.CurrentBatch = 2
	path := filepath.Join(fx.ReportsDir, websterengine.ReportFileName(2, "pending"))
	if err := os.WriteFile(path, []byte(validReport(fx.HeadSHA)), 0o644); err != nil {
		t.Fatalf("write batch 2 report: %v", err)
	}
	result, err = websterengine.RecordBatch(fx.Deps, 2)
	if err != nil {
		t.Fatalf("batch 2 RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("batch 2 Digest = %+v; want done", result.Digest)
	}
	if warningsContain(result.Warnings, "named-spawn") {
		t.Errorf("batch 2 Warnings = %v; want no repeated named-spawn warning", result.Warnings)
	}
}

// TestRecordBatch_ParentWriteToRunStateFailsBatch proves a Master write to the run's state.json is a
// correctness finding that fails the batch, naming the path.
func TestRecordBatch_ParentWriteToRunStateFailsBatch(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
	}})
	fx.Deps.Geom.WebsterDir = t.TempDir()
	state := filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")
	fx.Engine.scripted[0].ParentWrites = []string{state}
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecordBatch() error = %v; want ErrBatchFailed", err)
	}
	if !warningsContain(result.Digest.Reasons, state) {
		t.Errorf("Reasons = %v; want the state.json path named", result.Digest.Reasons)
	}
}

// TestRecordBatch_ForkContractWriteFailsBatch proves a fork writing a Master contract file fails the
// batch and archives its report.
func TestRecordBatch_ForkContractWriteFailsBatch(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
	}})
	fx.Engine.scripted[0].Forks[0].WritePaths = []string{fx.Deps.OutcomePath}
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecordBatch() error = %v; want ErrBatchFailed", err)
	}
	if got := archivedReports(t, fx.ReportsDir); len(got) != 1 {
		t.Errorf("archived reports = %v; want exactly one", got)
	}
}

// TestRecordBatch_Regression20260930_ForkAuditFalsePositive pins the 2026-09-30 incident: a
// fabric-reference finding on an otherwise clean batch records done with a warning instead of
// refusing every retry.
func TestRecordBatch_Regression20260930_ForkAuditFalsePositive(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{forkFabricRefAudit()})
	fx.Deps.RefMatcher = fabricMatcher{}
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	for attempt := 1; attempt <= 2; attempt++ {
		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if attempt == 2 {
			if err == nil || !strings.Contains(err.Error(), "already terminal") {
				t.Fatalf("second RecordBatch() error = %v; want a refusal naming the batch already terminal", err)
			}
			break
		}
		if err != nil {
			t.Fatalf("RecordBatch() error = %v; want nil", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("Digest = %+v; want done", result.Digest)
		}
		if !warningsContain(result.Warnings, "audit warning (fabric-reference)") {
			t.Errorf("Warnings = %v; want the fabric-reference warning", result.Warnings)
		}
	}
}

// TestRecordBatch_HeadSHAMismatchErrors proves a batch-report whose own self-reported head_sha
// disagrees with the worktree's actual current HEAD is a hard error, naming both — the fork's
// report and the repo it left behind must never be trusted to agree silently.
func TestRecordBatch_HeadSHAMismatchErrors(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, "status: OK\nhead_sha: 0000000000000000000000000000000000000000000000000000000000000000\n")
	restore := snapshotRecordState(fx)

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a hard error for a head_sha mismatch")
	}
	if !strings.Contains(err.Error(), fx.HeadSHA) {
		t.Errorf("RecordBatch() error = %q; want it to name the worktree's actual HEAD %q", err.Error(), fx.HeadSHA)
	}

	// Taking the way forward: the report names the worktree's actual HEAD, and the same call records.
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	restore()
	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Fatalf("RecordBatch() with a corrected head_sha error = %v; want nil", err)
	}
}

// TestRecordBatch_MalformedReportYAMLErrors proves an unparseable batch-report (here, an
// unrecognized status value) is a hard error, never a guessed digest.
func TestRecordBatch_MalformedReportYAMLErrors(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, "status: bogus\nhead_sha: "+fx.HeadSHA+"\n")

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a hard error for an unrecognized status value")
	}
	if !strings.Contains(err.Error(), "way forward: `lyx webster recover-batch 1` archives the malformed report") {
		t.Errorf("RecordBatch() error = %q; want the recover-batch way forward", err.Error())
	}
}

// TestRecordBatch_WayForward_UnknownBatch proves a batch number outside the plan names `lyx webster status`,
// and that naming a batch the run does have then reaches the ordinary begin-record refusal instead.
func TestRecordBatch_WayForward_UnknownBatch(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	_, err := websterengine.RecordBatch(fx.Deps, 99)
	if err == nil || !strings.Contains(err.Error(), "way forward: `lyx webster status` lists the run's batches") {
		t.Fatalf("RecordBatch(99) error = %v; want the status way forward", err)
	}

	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Errorf("RecordBatch(1) error = %v; want the named batch to record", err)
	}
}

// TestRecordBatch_MissingSessionTranscriptArchivesReport proves the TRUE cross-machine resume
// failure — the bracket-opening session's transcript file does not exist on this machine at all, so
// the audit read itself fails with fs.ErrNotExist — archives the report, keeps the batch begun, and
// explains the machine-local transcripts with the begin-batch way forward (found live in crucible
// round fable-r3).
// errors.Is must still see the underlying fs.ErrNotExist.
func TestRecordBatch_MissingSessionTranscriptArchivesReport(t *testing.T) {
	fx := newRecordFixture(t, nil)
	fx.Engine.auditErr = fmt.Errorf("claudeengine: read parent transcript %q: %w", "/nope/session.jsonl", fs.ErrNotExist)
	writeReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+fx.HeadSHA+"\n")

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrReportArchived) {
		t.Fatalf("RecordBatch() error = %v; want ErrReportArchived", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("RecordBatch() error = %v; want errors.Is(err, fs.ErrNotExist) preserved through the wrap", err)
	}
	for _, needle := range []string{"machine-local", "lyx webster begin-batch 01", "session-1"} {
		if !strings.Contains(err.Error(), needle) {
			t.Errorf("RecordBatch() error = %q; want it to contain %q", err.Error(), needle)
		}
	}
	if got := archivedReports(t, fx.ReportsDir); len(got) != 1 || result == nil || result.ArchivedReport == "" {
		t.Errorf("archived reports = %v, result = %+v; want one archived report surfaced on the result", got, result)
	}
	if bs := fx.Deps.State.Batches[1]; bs.Terminal || bs.StartSHA != fx.StartSHA {
		t.Errorf("BatchState = %+v; want it still begun, non-terminal, with StartSHA %q kept", bs, fx.StartSHA)
	}
}

// TestRecordBatch_DoneChecksBlockOnUnresolvedCreate proves card 33's wiring as card 10 reshaped it:
// a Create target that still does not resolve against the worktree's actual post-card tree fails the
// batch terminally with its findings as reasons and the report archived.
func TestRecordBatch_DoneChecksBlockOnUnresolvedCreate(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	fx.Deps.Plan.Cards[0].TargetGroups = []planparser.TargetGroup{
		{Type: planparser.CardTypeCreate, Refs: []string{"internal/foo#NeverLanded"}},
	}

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrBatchFailed)", err)
	}
	if result == nil || !result.Failed {
		t.Fatalf("RecordBatch() result = %+v; want Failed", result)
	}
	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed {
		t.Errorf("BatchState = terminal %v status %q; want terminal failed", bs.Terminal, bs.Status)
	}
	if bs.Digest == nil || !strings.Contains(strings.Join(bs.Digest.Reasons, "; "), "NeverLanded") {
		t.Errorf("digest = %+v; want the done-check finding in its reasons", bs.Digest)
	}
	if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
		t.Errorf("report still at its live path (stat err %v); want it archived", statErr)
	}
}

// writeRecordPlanDir writes a minimal, valid on-disk plan directory holding one card whose body is
// cardBody, returning the directory and its freshly parsed *planparser.Plan — the record-batch
// wiring test's own plan-fixture builder, package-local to this file since planglyph's own
// writePlanFixture is unexported to its package.
func writeRecordPlanDir(t *testing.T, cardBody string) (string, *planparser.Plan) {
	t.Helper()
	dir := t.TempDir()

	content := "# Card 1 — json-flag\n\n" + cardBody + "\n"
	if err := os.WriteFile(filepath.Join(dir, "01-json-flag.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	overview := "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n# Plan: test\n\nframing\n\n## Card Index\n\n1 — json-flag — summary\n"
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) returned error: %v", dir, err)
	}
	return dir, plan
}

// TestRecordBatch_BindsHandleFromDeltaEndToEnd proves card 34's wiring end to end: a Create
// declaration whose symbol actually landed in the batch's own work commit is bound from the
// record-batch delta and rewritten on disk to its plain glyph, with the batch still terminating
// cleanly.
func TestRecordBatch_BindsHandleFromDeltaEndToEnd(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})

	planDir, plan := writeRecordPlanDir(t, "**Create:**\n- `plan:internal/foo#Bar` -> `func Bar() {}`\n\n**Intent:** add Bar\n")
	fx.Deps.Geom.PlanDir = planDir
	fx.Deps.Plan = plan
	fx.Deps.Batches[0].Cards = plan.Cards

	headSHA := commitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Bar() {}\n", "01.1: add Bar")
	writeReport(t, fx.ReportsDir, validReport(headSHA))

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}

	data, readErr := os.ReadFile(filepath.Join(planDir, "01-json-flag.md"))
	if readErr != nil {
		t.Fatalf("read card file: %v", readErr)
	}
	if strings.Contains(string(data), "plan:internal/foo#Bar") {
		t.Errorf("card file still carries the unbound handle: %s", data)
	}
	if !strings.Contains(string(data), "internal/foo#Bar") {
		t.Errorf("card file does not carry the bound plain glyph: %s", data)
	}
}

// TestRecordBatch_ScopeGuardFindingsLandInWarnings proves card 35's wiring: a symbol touched
// outside the completed batch's own target glyphs lands as an informational finding in
// RecordResult.Warnings, and the batch still terminates.
func TestRecordBatch_ScopeGuardFindingsLandInWarnings(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	// A symbol added outside the plan's own declared targets.
	headSHA := commitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Surprise() {}\n", "01.2: add Surprise")
	writeReport(t, fx.ReportsDir, validReport(headSHA))
	fx.Deps.Plan.Cards[0].Targets = []string{"unrelated/thing#Nothing"}

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "scope-outside-plan") {
			found = true
		}
	}
	if !found {
		t.Errorf("RecordResult.Warnings = %v; want a scope-outside-plan finding", result.Warnings)
	}
}

// TestRecordBatch_ScopeGuardDegradesOnDeltaUnavailable proves the guard's own degradation path: a
// DeltaGit infrastructure error (an unresolvable start SHA) records the guard-could-not-run notice
// and never panics — the batch still terminates, since neither the done-checks above (their own
// Resolve, unaffected) nor BindHandles (this card's card carries no Create declaration) has
// anything to block on.
func TestRecordBatch_ScopeGuardDegradesOnDeltaUnavailable(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	fx.Deps.State.Batches[1].StartSHA = "does-not-exist-rev"

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil (the scope guard alone must degrade, not fail the run)", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "could not run") {
			found = true
		}
	}
	if !found {
		t.Errorf("RecordResult.Warnings = %v; want the guard-could-not-run notice", result.Warnings)
	}
}

// TestRecordBatch_DriftBlocksOnDeletedStillReferenced proves card 36's wiring: a symbol the delta
// reports deleted, with no corresponding rename, that the plan still references returns
// ErrCardNotDone and persists no terminal digest.
func TestRecordBatch_DriftBlocksOnDeletedStillReferenced(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	// A symbol must exist at the delta's START side to be reported deleted — the fixture's own
	// StartSHA (base.txt only) predates internal/foo entirely, so the batch's own start boundary is
	// moved to a commit that already carries the symbol, and a later commit removes it.
	withSymbol := commitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc WillGoAway() {}\n", "01.2: add WillGoAway")
	fx.Deps.State.Batches[1].StartSHA = withSymbol
	if err := os.Remove(filepath.Join(fx.Worktree, "internal/foo/impl.go")); err != nil {
		t.Fatalf("remove impl.go: %v", err)
	}
	mustGit(t, fx.Worktree, "add", "-A")
	mustGit(t, fx.Worktree, "commit", "-m", "01.3: remove WillGoAway")
	headSHA := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
	writeReport(t, fx.ReportsDir, validReport(headSHA))
	addPendingCard(fx, []string{"internal/foo#WillGoAway"})

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil — drift about a later card warns, it does not fail this batch", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
	var inWarnings int
	for _, w := range result.Warnings {
		if strings.Contains(w, "later card:") && strings.Contains(w, "WillGoAway") {
			inWarnings++
		}
	}
	if inWarnings != 1 {
		t.Errorf("RecordResult.Warnings = %v; want exactly one later card: warning", result.Warnings)
	}
	var recorded int
	for _, w := range fx.Deps.State.Batches[1].AuditWarnings {
		if w.Class == "later-card-drift" {
			recorded++
		}
	}
	if recorded != 1 {
		t.Errorf("BatchState.AuditWarnings = %+v; want exactly one later-card-drift entry", fx.Deps.State.Batches[1].AuditWarnings)
	}
}

// TestRecordBatch_EvidenceTierDriftWarnsAndDoesNotBlock proves DetectDrift's mixed severity set is
// split rather than blanket-blocked: an inexact rename quarry classifies as an evidence-tier
// candidate rather than an exact pair produces an informational rename-candidate finding, which
// must ride out on Warnings and let the batch terminate — a finding that kills the batch never
// reaches the reviewer whose decision the tier exists to inform.
func TestRecordBatch_EvidenceTierDriftWarnsAndDoesNotBlock(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	// The symbol must exist at the delta's start side to be reported deleted, so the batch's own
	// start boundary moves to a commit that already carries it.
	withSymbol := commitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc WillMove() int { return 1 }\n", "01.2: add WillMove")
	fx.Deps.State.Batches[1].StartSHA = withSymbol
	// Renamed AND rewritten: the token streams differ in length, so quarry's exact tier declines it
	// and offers it as a candidate instead.
	headSHA := commitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Moved() int {\n\ttotal := 1\n\ttotal += 0\n\treturn total\n}\n", "01.3: rename and rewrite")
	writeReport(t, fx.ReportsDir, validReport(headSHA))
	addPendingCard(fx, []string{"internal/foo#WillMove"})

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil — an informational rename-candidate must never fail the batch", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
	var surfaced bool
	for _, w := range result.Warnings {
		if strings.Contains(w, "rename-candidate") {
			surfaced = true
		}
	}
	if !surfaced {
		t.Errorf("RecordResult.Warnings = %v; want the informational rename-candidate finding surfaced there", result.Warnings)
	}
}

// TestRecordBatch_DeleteCardDeletingItsOwnTargetIsNotDrift proves the batch being recorded is
// excluded from drift detection. Its work is exactly what the delta reports, so a Delete card
// referencing the symbol it just deleted was reporting its own success as
// plan-references-deleted-symbol — and no Delete card could ever be recorded at all.
func TestRecordBatch_DeleteCardDeletingItsOwnTargetIsNotDrift(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	withSymbol := commitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc WillGoAway() {}\n", "01.2: add WillGoAway")
	fx.Deps.State.Batches[1].StartSHA = withSymbol
	if err := os.Remove(filepath.Join(fx.Worktree, "internal/foo/impl.go")); err != nil {
		t.Fatalf("remove impl.go: %v", err)
	}
	mustGit(t, fx.Worktree, "add", "-A")
	mustGit(t, fx.Worktree, "commit", "-m", "01.3: remove WillGoAway")
	headSHA := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
	writeReport(t, fx.ReportsDir, validReport(headSHA))

	// The card being recorded IS the Delete card, and it names the symbol its own batch removed.
	fx.Deps.Plan.Cards[0].Targets = []string{"internal/foo#WillGoAway"}
	fx.Deps.Plan.Cards[0].TargetGroups = []planparser.TargetGroup{
		{Type: planparser.CardTypeDelete, Refs: []string{"internal/foo#WillGoAway"}},
	}
	fx.Deps.Batches[0].Cards = fx.Deps.Plan.Cards[:1]

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil — a Delete card's own deletion is its success, never drift", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
}

// TestRecordBatch_DoneChecksPassOnLandedCreate proves the happy path: a Create target whose
// symbol actually landed in the batch's own work commit passes the done-checks and the batch
// still terminates cleanly.
func TestRecordBatch_DoneChecksPassOnLandedCreate(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	// newRecordFixture's own work commit adds internal/foo/impl.go with no declared symbol; add one
	// the fixture's own Create target can resolve against.
	commitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Bar() {}\n", "01.2: add Bar")
	headSHA := commitFile(t, fx.Worktree, "base.txt", "base updated", "01.3: bump base")
	writeReport(t, fx.ReportsDir, validReport(headSHA))
	fx.Deps.Plan.Cards[0].TargetGroups = []planparser.TargetGroup{
		{Type: planparser.CardTypeCreate, Refs: []string{"internal/foo#Bar"}},
	}

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
}

// seedDriftPlanDir writes a real, parseable two-card plan directory whose second card's Uses field
// names every ref in uses. RecordBatch's own drift repair calls planparser.RewriteRefs against this
// directory, which re-parses it from disk rather than reading RecordDeps.Plan, so a repair scenario
// needs genuine card files here and not only the in-memory plan newRecordFixture builds.
func seedDriftPlanDir(t *testing.T, uses []string) string {
	t.Helper()
	dir := t.TempDir()

	overview := "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n" +
		"# Plan: drift fixture\n\nA two-card fixture whose second card is still pending.\n\n" +
		"## Card Index\n\n1 — json-flag — the recorded batch's own card\n2 — pending — the not-yet-built card\n"
	card1 := "# Card 1 — json-flag\n\n**Prosa:**\n- `//base.txt`\n\n**Intent:** The recorded batch's own card.\n"

	var usesBullets string
	for _, u := range uses {
		usesBullets += "- `" + u + "`\n"
	}
	card2 := "# Card 2 — pending\n\n**Prosa:**\n- `//base.txt`\n\n**Uses:**\n" + usesBullets +
		"\n**Intent:** The not-yet-built card that still references what batch 1 moved out from under it.\n"

	for name, content := range map[string]string{
		"00-overview.md":  overview,
		"01-json-flag.md": card1,
		"02-pending.md":   card2,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("seed drift plan dir %s: %v", name, err)
		}
	}
	return dir
}

// TestRecordBatch_RestampsFingerprintEvenWhenDriftBlocks is the regression test for the round-4
// review's R4-01. BindHandles and DetectDrift both rewrite the plan on disk BEFORE they report a
// finding, and a single call routinely does both: here the delta carries an exact-tier rename the
// pending card references (repaired, so planparser.RewriteRefs rewrites 02-pending.md) alongside a
// deletion the same card references (blocking, so RecordBatch returns ErrCardNotDone).
//
// With the re-baseline positioned after the refusal, state.json kept the pre-rewrite fingerprint
// while the plan on disk carried webster's own sanctioned edit, so every later begin-batch refused
// it as a foreign edit — and `--fresh`, the advised recourse, then refused the run outright over the
// cards that had already landed. The run was unrecoverable without hand-editing state.json.
func TestRecordBatch_RestampsFingerprintEvenWhenDriftBlocks(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})

	planDir := seedDriftPlanDir(t, []string{"internal/foo#WillMove", "internal/foo#WillGoAway"})
	fx.Deps.Geom.PlanDir = planDir
	seeded := mustFingerprint(t, planDir)
	fx.Deps.State.PlanFingerprint = seeded

	// Both symbols must exist at the delta's start side, so the batch's own start boundary moves to
	// a commit that already carries them.
	withSymbols := commitFile(t, fx.Worktree, "internal/foo/impl.go",
		"package foo\n\nfunc WillMove() int { return 1 }\n\nfunc WillGoAway() {}\n", "01.2: add both symbols")
	fx.Deps.State.Batches[1].StartSHA = withSymbols
	// One exact-tier rename (identical body, so quarry asserts the pair) plus one genuine deletion.
	headSHA := commitFile(t, fx.Worktree, "internal/foo/impl.go",
		"package foo\n\nfunc Moved() int { return 1 }\n", "01.3: rename one, delete the other")
	writeReport(t, fx.ReportsDir, validReport(headSHA))
	addPendingCard(fx, []string{"internal/foo#WillMove", "internal/foo#WillGoAway"})

	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil — the deleted-and-still-referenced symbol warns about a later card", err)
	}

	repaired, readErr := os.ReadFile(filepath.Join(planDir, "02-pending.md"))
	if readErr != nil {
		t.Fatalf("read 02-pending.md: %v", readErr)
	}
	if !strings.Contains(string(repaired), "internal/foo#Moved") {
		t.Fatalf("02-pending.md = %q; want the exact-tier repair to have rewritten the renamed glyph — the fixture is not exercising a rewrite at all", repaired)
	}

	if fx.Deps.State.PlanFingerprint == seeded {
		t.Error("State.PlanFingerprint still carries its pre-call value after a call that rewrote the plan on disk; every later begin-batch would refuse webster's own sanctioned rewrite as a foreign edit")
	}
	if fx.Deps.State.PlanFingerprint == "" {
		t.Error("State.PlanFingerprint was cleared rather than re-baselined")
	}
}

// TestRecordBatch_NilStateIsRefusedNotPanicked is R6-21's regression test for the record-batch half.
func TestRecordBatch_NilStateIsRefusedNotPanicked(t *testing.T) {
	_, err := websterengine.RecordBatch(websterengine.RecordDeps{Plan: &planparser.Plan{}}, 1)
	if err == nil {
		t.Fatal("websterengine.RecordBatch(nil State) error = nil; want a refusal naming the missing field")
	}
	if !strings.Contains(err.Error(), "State is nil") {
		t.Errorf("websterengine.RecordBatch(nil State) error = %v; want it to name RecordDeps.State", err)
	}
}

// recordParentBranch is the parent branch parentMergeFixture's run merges from.
const recordParentBranch = "parent1"

// parentMerge simulates a parent merge-in: it checks out side (branching it off startSHA when it does not exist yet), commits one file there (name=content),
// returns to the original branch and merges side with --no-ff, returning the merge commit's SHA.
func parentMerge(t *testing.T, fx *recordFixture, side, name, content string) string {
	t.Helper()
	base := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "--abbrev-ref", "HEAD"))
	if _, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + side}, fx.Worktree); err == nil && exitCode == 0 {
		mustGit(t, fx.Worktree, "checkout", side)
	} else {
		mustGit(t, fx.Worktree, "checkout", "-b", side, fx.StartSHA)
	}
	commitFile(t, fx.Worktree, name, content, side+" commit")
	mustGit(t, fx.Worktree, "checkout", base)
	mustGit(t, fx.Worktree, "merge", "--no-ff", "-m", "merge "+side, side)
	return strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
}

// snapshotRecordState captures the fork-transcript bookkeeping RecordBatch mutates before it can refuse, and returns a restore func:
// the CLI never persists a refused call's state, so a retry runs against the state as it stood before that call.
func snapshotRecordState(fx *recordFixture) (restore func()) {
	seen := append([]string(nil), fx.Deps.State.SeenForkTranscripts...)
	forks := append([]string(nil), fx.Deps.State.Batches[1].ForkTranscripts...)
	return func() {
		fx.Deps.State.SeenForkTranscripts = append([]string(nil), seen...)
		fx.Deps.State.Batches[1].ForkTranscripts = append([]string(nil), forks...)
	}
}

func parentMergeFixture(t *testing.T) *recordFixture {
	t.Helper()
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	fx.Deps.ParentBranch = func() (string, error) { return recordParentBranch, nil }
	return fx
}

func assertBatchOpen(t *testing.T, fx *recordFixture) {
	t.Helper()
	if fx.Deps.State.Batches[1].Terminal {
		t.Error("batch is terminal; want it left non-terminal")
	}
	if fx.Deps.State.CurrentBatch != 1 {
		t.Errorf("CurrentBatch = %d; want 1 (unchanged)", fx.Deps.State.CurrentBatch)
	}
}

// TestRecordBatch_ParentMergeAfterForkCommit proves a parent merge-in landing after the fork's commit no longer wedges record-batch:
// the batch is recorded at the report's own head_sha.
func TestRecordBatch_ParentMergeAfterForkCommit(t *testing.T) {
	fx := parentMergeFixture(t)
	// The merge commit is both the new HEAD and the one walked merge.
	merge := parentMerge(t, fx, "parent1", "parent1.txt", "p1")

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || !fx.Deps.State.Batches[1].Terminal {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal batch", result.Digest)
	}
	if got := fx.Deps.State.Batches[1].CardSHAs; len(got) != 1 || got[0] != fx.HeadSHA {
		t.Errorf("CardSHAs = %v; want [%s]", got, fx.HeadSHA)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %v; want exactly one", result.Warnings)
	}
	for _, want := range []string{fx.HeadSHA, "HEAD \"" + merge + "\"", "(" + merge + ")"} {
		if !strings.Contains(result.Warnings[0], want) {
			t.Errorf("warning %q missing %q", result.Warnings[0], want)
		}
	}
}

func TestRecordBatch_TwoParentMergesAfterForkCommit(t *testing.T) {
	fx := parentMergeFixture(t)
	m1 := parentMerge(t, fx, "parent1", "parent1.txt", "p1")
	m2 := parentMerge(t, fx, "parent1", "parent2.txt", "p2")

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if !fx.Deps.State.Batches[1].Terminal {
		t.Fatal("batch is not terminal")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %v; want exactly one", result.Warnings)
	}
	for _, want := range []string{m1, m2} {
		if !strings.Contains(result.Warnings[0], want) {
			t.Errorf("warning %q missing merge %q", result.Warnings[0], want)
		}
	}
}

// TestRecordBatch_ParentMergeSymbolsAreNotTheBatchsOwn proves the delta is the fork's own StartSHA..head_sha range:
// a symbol the parent side brought in raises no scope, drift or bind finding.
func TestRecordBatch_ParentMergeSymbolsAreNotTheBatchsOwn(t *testing.T) {
	fx := parentMergeFixture(t)
	fx.Deps.Plan.Cards[0].Targets = []string{"unrelated/thing#Nothing"}
	parentMerge(t, fx, "parent1", "internal/parent/p.go", "package parent\n\nfunc FromParent() {}\n")

	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want nil", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "only merge commits") {
		t.Errorf("Warnings = %v; want only the moved-HEAD warning", result.Warnings)
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "FromParent") || strings.Contains(w, "scope-outside-plan") {
			t.Errorf("warning %q names the parent side's symbol", w)
		}
	}
}

func TestRecordBatch_NonMergeMovementRefused(t *testing.T) {
	cases := map[string]func(t *testing.T, fx *recordFixture){
		"non-merge commit alone": func(t *testing.T, fx *recordFixture) {
			commitFile(t, fx.Worktree, "extra.txt", "x", "extra commit")
		},
		"non-merge commit after a merge": func(t *testing.T, fx *recordFixture) {
			parentMerge(t, fx, "parent1", "parent1.txt", "p1")
			commitFile(t, fx.Worktree, "extra.txt", "x", "extra commit")
		},
		"fast-forward onto non-merge commits": func(t *testing.T, fx *recordFixture) {
			base := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "--abbrev-ref", "HEAD"))
			mustGit(t, fx.Worktree, "checkout", "-b", "ffside")
			commitFile(t, fx.Worktree, "ff.txt", "ff", "ff commit")
			mustGit(t, fx.Worktree, "checkout", base)
			mustGit(t, fx.Worktree, "merge", "--ff-only", "ffside")
		},
	}
	for name, move := range cases {
		t.Run(name, func(t *testing.T) {
			fx := parentMergeFixture(t)
			restore := snapshotRecordState(fx)
			move(t, fx)
			newHead := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))

			_, err := websterengine.RecordBatch(fx.Deps, 1)
			if err == nil {
				t.Fatal("RecordBatch() error = nil; want a refusal")
			}
			for _, want := range []string{fx.HeadSHA, newHead, "only merge commits"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err.Error(), want)
				}
			}
			assertBatchOpen(t, fx)

			// Taking the way forward: HEAD goes back to the report's head_sha and the same call records.
			mustGit(t, fx.Worktree, "reset", "--hard", fx.HeadSHA)
			restore()
			if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
				t.Fatalf("retry RecordBatch() error = %v; want nil", err)
			}
		})
	}
}

// TestRecordBatch_EvilParentMergeRefused proves a parent merge carrying an extra edit is refused, leaving the batch open,
// so content outside the audited StartSHA..head_sha delta can never ride in on a merge commit.
func TestRecordBatch_EvilParentMergeRefused(t *testing.T) {
	fx := parentMergeFixture(t)
	restore := snapshotRecordState(fx)
	base := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "--abbrev-ref", "HEAD"))
	mustGit(t, fx.Worktree, "checkout", "-b", recordParentBranch, fx.StartSHA)
	commitFile(t, fx.Worktree, "parent1.txt", "p1", "parent1 commit")
	mustGit(t, fx.Worktree, "checkout", base)
	mustGit(t, fx.Worktree, "merge", "--no-ff", "--no-commit", recordParentBranch)
	if err := os.WriteFile(filepath.Join(fx.Worktree, "smuggled.txt"), []byte("unaudited"), 0o644); err != nil {
		t.Fatalf("write smuggled file: %v", err)
	}
	mustGit(t, fx.Worktree, "add", "smuggled.txt")
	mustGit(t, fx.Worktree, "commit", "--no-edit")

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a refusal")
	}
	for _, want := range []string{fx.HeadSHA, "carries changes beyond a clean merge", "remedy:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
	assertBatchOpen(t, fx)

	// Taking the way forward: HEAD goes back to the report's head_sha and the same call records.
	mustGit(t, fx.Worktree, "reset", "--hard", fx.HeadSHA)
	restore()
	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Fatalf("retry RecordBatch() error = %v; want nil", err)
	}
}

// TestRecordBatch_MergeInProgressRefusedThenSucceeds proves a conflicting parent merge left in progress refuses record-batch,
// that concluding it by hand is still refused because a conflict resolution is not a clean parent merge,
// and that the same call succeeds once HEAD is moved back to the report's head_sha as the refusal's remedy says.
func TestRecordBatch_MergeInProgressRefusedThenSucceeds(t *testing.T) {
	fx := parentMergeFixture(t)
	restore := snapshotRecordState(fx)

	base := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "--abbrev-ref", "HEAD"))
	mustGit(t, fx.Worktree, "checkout", "-b", "parent1", fx.StartSHA)
	commitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\n// parent side\n", "parent side impl")
	mustGit(t, fx.Worktree, "checkout", base)
	if _, _, exitCode, err := gitexec.RunGit([]string{"merge", "--no-ff", "-m", "merge parent1", "parent1"}, fx.Worktree); err != nil || exitCode == 0 {
		t.Fatalf("conflicting merge: exit=%d err=%v; want a conflict", exitCode, err)
	}

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a refusal while a merge is in progress")
	}
	if !strings.Contains(err.Error(), "merge --continue") || !strings.Contains(err.Error(), "merge --abort") {
		t.Errorf("error %q; want the merge --continue/--abort pointer", err.Error())
	}
	assertBatchOpen(t, fx)

	if err := os.WriteFile(filepath.Join(fx.Worktree, "internal/foo/impl.go"), []byte("package foo\n"), 0o644); err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	mustGit(t, fx.Worktree, "add", "internal/foo/impl.go")
	mustGit(t, fx.Worktree, "commit", "--no-edit")

	restore()
	_, err = websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() after a hand-resolved merge: error = nil; want a refusal")
	}
	for _, want := range []string{"do not merge cleanly", "remedy:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
	assertBatchOpen(t, fx)

	mustGit(t, fx.Worktree, "reset", "--hard", fx.HeadSHA)
	restore()
	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Fatalf("retry RecordBatch() error = %v; want nil", err)
	}
	if got := fx.Deps.State.Batches[1].CardSHAs; len(got) != 1 || got[0] != fx.HeadSHA {
		t.Errorf("CardSHAs = %v; want [%s]", got, fx.HeadSHA)
	}
}
