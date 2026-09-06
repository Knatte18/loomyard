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
		},
		RefMatcher:  websterengine.NeverMatches{},
		OutcomePath: filepath.Join(contractDir, "outcome.yaml"),
		SummaryPath: filepath.Join(contractDir, "summary.md"),
		Sleeper:     sleeper,
		Plan:        plan,
	}

	return &recordFixture{Deps: deps, Engine: engine, Sleeper: sleeper, Worktree: worktree, ReportsDir: reportsDir, StartSHA: startSHA, HeadSHA: headSHA}
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
// BatchState entry,
// or one already Terminal, is refused with ErrNoBeginRecord before the audit is ever consulted.
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

// TestRecordBatch_ZeroNewTranscriptsHardErrorsEvenWithReport proves the unfakeable-report rule:
// zero new transcripts through the whole settle window is a hard error REGARDLESS of a batch-report
// file already sitting on disk — a report with no fork behind it means Master wrote it itself.
func TestRecordBatch_ZeroNewTranscriptsHardErrorsEvenWithReport(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{{}})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrNoForkTranscripts) {
		t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrNoForkTranscripts)", err)
	}
	if len(fx.Sleeper.slept) == 0 {
		t.Errorf("Sleeper.slept is empty; want the settle window's retry ticks recorded")
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

// TestRecordBatch_ParentWriteOutsideContractFilesErrors proves CheckParent's write-policy violation
// is surfaced as a hard error naming the offending write, even when the fork-transcript count and
// the report itself are both otherwise clean.
func TestRecordBatch_ParentWriteOutsideContractFilesErrors(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{
			Forks:        []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
			ParentWrites: []string{"/some/other/hand-written-file.go"},
		},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a hard error for a parent write outside the two contract files")
	}
	if !strings.Contains(err.Error(), "hand-written-file.go") {
		t.Errorf("RecordBatch() error = %q; want it to name the offending parent write", err.Error())
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

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a hard error for a head_sha mismatch")
	}
	if !strings.Contains(err.Error(), fx.HeadSHA) {
		t.Errorf("RecordBatch() error = %q; want it to name the worktree's actual HEAD %q", err.Error(), fx.HeadSHA)
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
}

// TestRecordBatch_MissingSessionTranscriptNamesRecourse proves the TRUE cross-machine resume
// failure — the bracket-opening session's transcript file does not exist on this machine at all, so
// the audit read itself fails with fs.ErrNotExist — is wrapped with the machine-local-transcripts
// explanation and the move-the-report-aside operator recourse, instead of surfacing a bare "no such
// file or directory" (found live in crucible round fable-r3).
// errors.Is must still see the underlying fs.ErrNotExist.
func TestRecordBatch_MissingSessionTranscriptNamesRecourse(t *testing.T) {
	fx := newRecordFixture(t, nil)
	fx.Engine.auditErr = fmt.Errorf("claudeengine: read parent transcript %q: %w", "/nope/session.jsonl", fs.ErrNotExist)
	writeReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+fx.HeadSHA+"\n")

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want the wrapped missing-transcript error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("RecordBatch() error = %v; want errors.Is(err, fs.ErrNotExist) preserved through the wrap", err)
	}
	for _, needle := range []string{"machine-local", "moving the batch's report file", "session-1"} {
		if !strings.Contains(err.Error(), needle) {
			t.Errorf("RecordBatch() error = %q; want it to contain %q", err.Error(), needle)
		}
	}
}

// TestRecordBatch_DoneChecksBlockOnUnresolvedCreate proves card 33's wiring: a Create target that
// still does not resolve against the worktree's actual post-card tree returns ErrCardNotDone and
// persists no terminal digest.
func TestRecordBatch_DoneChecksBlockOnUnresolvedCreate(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	fx.Deps.Plan.Cards[0].TargetGroups = []planparser.TargetGroup{
		{Type: planparser.CardTypeCreate, Refs: []string{"internal/foo#NeverLanded"}},
	}

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrCardNotDone) {
		t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrCardNotDone)", err)
	}
	if bs := fx.Deps.State.Batches[1]; bs.Terminal {
		t.Error("BatchState.Terminal = true; want false — a not-done card must not persist a terminal digest")
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
