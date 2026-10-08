// recordbatch_test.go exercises RecordBatch end to end (Tier 1 — see
// docs/benchmarks/running-tests.md): a temp directory backs
// WorktreeRoot over a fakeGit for the head, dirty and merge questions,
// while the incremental fork audit
// (shuttleengine.Engine.AuditForksIncremental) is a local, call-scripted
// fake and SettleRetry's clock seam is a recording fake Sleeper that never
// actually blocks, mirroring audit_test.go's own SettleRetry fixture
// pattern (package-local — the internal and external test packages
// deliberately do not share a test-helper package).

package websterengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
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

// recordAudit scripts a shuttlefake.Engine's AuditForksIncremental:
// it returns scripted[callCount] on each call (clamped to the last entry once the
// script is exhausted), so a test can drive a settle-retry sequence (an empty
// miss followed by a hit, or a stable audit across repeated calls) without any
// real transcript files.
type recordAudit struct {
	scripted  []shuttleengine.ForkAudit
	callCount int
	// sessions records the sessionID of every call, so a
	// test can assert WHICH session the audit was keyed on (the
	// bracket-opening session, never blindly the current Master session).
	sessions []string
	// auditErr, when non-nil, is returned by every call
	// instead of the script — the missing-transcript failure double for the
	// cross-machine resume path.
	auditErr error
	// seen records the seen set handed to every call.
	seen []map[string]bool
}

func (a *recordAudit) engine() *shuttlefake.Engine {
	return &shuttlefake.Engine{AuditForksIncrementalFn: a.audit}
}

func (a *recordAudit) audit(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	a.callCount++
	a.sessions = append(a.sessions, sessionID)
	a.seen = append(a.seen, maps.Clone(seenTranscripts))
	if a.auditErr != nil {
		return shuttleengine.ForkAudit{}, a.auditErr
	}
	if len(a.scripted) == 0 {
		return shuttleengine.ForkAudit{}, nil
	}
	idx := a.callCount - 1
	if idx >= len(a.scripted) {
		idx = len(a.scripted) - 1
	}
	return a.scripted[idx], nil
}

// recordFixture is a fully-wired set of RecordBatch dependencies: a temp
// directory holding the base and in-scope work files as WorktreeRoot over a
// fakeGit with one base commit plus one work commit, a literal one-batch
// execution-batch list with an
// already-open BatchState (the begin-batch record RecordBatch's
// bracket-discipline check requires), and a scripted fake engine plus
// recording Sleeper. HeadSHA is the fakeGit's current HEAD (the
// work commit) — every valid report fixture must self-report exactly this
// SHA, since RecordBatch cross-checks report.HeadSHA against the worktree's
// HEAD.
type recordFixture struct {
	Deps       websterengine.RecordDeps
	Audit      *recordAudit
	Sleeper    *recordFakeSleeper
	Git        *fakeGit
	Index      *fakeIndex
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

	worktree := t.TempDir()
	git := newFakeGit()
	writeWorktreeFile(t, worktree, "base.txt", "base")
	startSHA := git.head
	writeWorktreeFile(t, worktree, "internal/foo/impl.go", "package foo\n")
	headSHA := git.commit()

	fx := newRecordFixtureOver(t, worktree, git, startSHA, headSHA, scripted)
	fx.Git = git
	return fx
}

// newRecordFixtureOver builds the fixture over worktree, whose base commit is startSHA and work commit headSHA, answering git questions from git;
// a nil git means the real repository at worktree.
func newRecordFixtureOver(t *testing.T, worktree string, git websterengine.Git, startSHA, headSHA string, scripted []shuttleengine.ForkAudit) *recordFixture {
	t.Helper()

	fakeIdx, index := indexOver(git)
	cards := []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}
	batches := []batcher.Batch{{Cards: cards}}
	plan := &planparser.Plan{Format: 5, Cards: cards}

	reportsDir := t.TempDir()
	contractDir := t.TempDir()
	// A real (empty) plan directory: RecordBatch re-baselines the plan fingerprint over it after its
	// own BindHandles and DetectDrift rewrites, so this is a genuine read rather than an invented
	// path. No card here declares a handle, so nothing is ever written into it.
	planDir := t.TempDir()

	audit := &recordAudit{scripted: scripted}
	sleeper := &recordFakeSleeper{}

	state := &websterengine.State{
		MasterSessionID: "session-1",
		CurrentBatch:    1,
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "json-flag", StartSHA: startSHA, Kind: "fork", SessionID: "session-1"},
		},
	}

	// RecordBatch refuses a plan that differs from the recorded fingerprint, so the state records this one.
	websterDir := t.TempDir()
	if err := websterengine.RestampPlanBaseline(state, planDir, websterDir); err != nil {
		t.Fatalf("RestampPlanBaseline() error = %v", err)
	}

	deps := websterengine.RecordDeps{
		Batches: batches,
		State:   state,
		Config:  websterengine.Config{},
		Engine:  audit.engine(),
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   websterDir,
			ReportsDir:   reportsDir,
			PlanDir:      planDir,
			Git:          git,
			Index:        index,
		},
		RefMatcher:  websterengine.NeverMatches{},
		OutcomePath: filepath.Join(contractDir, "outcome.yaml"),
		SummaryPath: filepath.Join(contractDir, "summary.md"),
		Sleeper:     sleeper,
		Plan:        plan,
	}

	return &recordFixture{Deps: deps, Audit: audit, Sleeper: sleeper, Index: fakeIdx, Worktree: worktree, ReportsDir: reportsDir, StartSHA: startSHA, HeadSHA: headSHA}
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
// A report present is archived and the batch re-driven through begin-batch, so begin-batch's pre-existing-report refusal no longer fires;
// with no report the error still names begin-batch.
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
		if fx.Audit.callCount != 0 {
			t.Errorf("Engine was reached (%d calls) with no begin record; want zero", fx.Audit.callCount)
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
	if fx.Audit.callCount != 0 {
		t.Errorf("Engine was reached (%d calls) for a recovery batch; want zero", fx.Audit.callCount)
	}
}

// TestRecordBatch_ZeroNewTranscriptsArchivesReport proves the unfakeable-report rule:
// zero new transcripts through the whole settle window never records the report, REGARDLESS of a batch-report file already sitting on disk — a report with no fork behind it means Master wrote it itself.
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
	// A cross-machine resume reproduces this error with no forgery, so the message keeps that diagnosis.
	if !strings.Contains(err.Error(), "machine-local") {
		t.Errorf("RecordBatch() error = %q; want it to name the machine-local transcript caveat", err.Error())
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
		if strings.Contains(w, "2 fork transcripts count toward the batch") {
			found = true
		}
	}
	if !found {
		t.Errorf("RecordResult.Warnings = %v; want one naming 2 counted fork transcripts", result.Warnings)
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

// forkNestedAgentAudit scripts one fork transcript that attempted an Agent call, a policy finding.
func forkNestedAgentAudit() shuttleengine.ForkAudit {
	return shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{
		TranscriptPath: "subagents/f1.jsonl",
		ReportReturned: true,
		AgentCalls:     1,
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

// fabricPathMatcher is a RefMatcher that matches any command containing its fabric path.
type fabricPathMatcher string

func (m fabricPathMatcher) Matches(cmd string) bool { return strings.Contains(cmd, string(m)) }

// oneForkAudit is the clean audit of one fork whose report was returned.
func oneForkAudit() shuttleengine.ForkAudit {
	return shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}}
}

// TestRecordBatch_AuditOutcomes asserts how RecordBatch classifies one audit-and-report pair: a
// clean one records the batch done with the digest, attribution and warnings the audit implies, a
// policy finding warns and still records, and a correctness finding fails the batch terminally —
// naming recover-batch, archiving any report, and recording the Uncheckable entries recovery
// cannot check.
func TestRecordBatch_AuditOutcomes(t *testing.T) {
	t.Parallel()

	okReport := func(fx *recordFixture) string { return validReport(fx.HeadSHA) }
	failedReport := func(fx *recordFixture) string { return "status: FAILED\nhead_sha: " + fx.HeadSHA + "\n" }
	archived := func(t *testing.T, fx *recordFixture, want int) {
		t.Helper()
		if got := archivedReports(t, fx.ReportsDir); len(got) != want {
			t.Errorf("archived reports = %v; want %d", got, want)
		}
	}
	requireDone := func(t *testing.T, result *websterengine.RecordResult) {
		t.Helper()
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("RecordResult.Digest = %+v; want a done digest", result.Digest)
		}
	}
	requireUncheckableFabricReference := func(t *testing.T, fx *recordFixture) {
		t.Helper()
		got := fx.Deps.State.Batches[1].Uncheckable
		if len(got) != 1 || !strings.HasPrefix(got[0], string(websterengine.ClassFabricReference)+": ") {
			t.Errorf("Uncheckable = %v; want one fabric-reference entry", got)
		}
	}
	forkWithCommand := func(cmd string) func() []shuttleengine.ForkAudit {
		return func() []shuttleengine.ForkAudit {
			return []shuttleengine.ForkAudit{{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true, BashCommands: []string{cmd}}}}}
		}
	}
	parentWrite := func(path func(fx *recordFixture) string) func(t *testing.T, fx *recordFixture) {
		return func(t *testing.T, fx *recordFixture) { fx.Audit.scripted[0].ParentWrites = []string{path(fx)} }
	}
	requireFullHeadSHA := func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
		t.Helper()
		requireDone(t, result)
		if result.Digest.HeadSHA != fx.HeadSHA {
			t.Errorf("Digest.HeadSHA = %q; want the full SHA %q", result.Digest.HeadSHA, fx.HeadSHA)
		}
		if got := fx.Deps.State.Batches[1].CardSHAs; len(got) != 1 || got[0] != fx.HeadSHA {
			t.Errorf("CardSHAs = %v; want [%s]", got, fx.HeadSHA)
		}
	}
	trackedFile := func(fx *recordFixture) string { return filepath.Join(fx.Worktree, "internal", "foo", "impl.go") }
	oneFork := func() []shuttleengine.ForkAudit { return []shuttleengine.ForkAudit{oneForkAudit()} }

	cases := []struct {
		name    string
		audits  func() []shuttleengine.ForkAudit
		prepare func(t *testing.T, fx *recordFixture)
		// report is the batch-report file's content; nil leaves none on disk.
		report func(fx *recordFixture) string
		// wantFailed expects the batch to fail terminally with ErrBatchFailed.
		wantFailed bool
		check      func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error)
	}{
		{
			name:   "a lowercase abbreviation of HEAD records the full SHA",
			audits: oneFork,
			report: func(fx *recordFixture) string { return validReport(fx.HeadSHA[:9]) },
			check:  requireFullHeadSHA,
		},
		{
			name:   "an uppercase abbreviation of HEAD records the full lowercase SHA",
			audits: oneFork,
			report: func(fx *recordFixture) string { return validReport(strings.ToUpper(fx.HeadSHA[:9])) },
			check:  requireFullHeadSHA,
		},
		{
			name:       "a correctness finding fails with the full SHA when head_sha abbreviates HEAD",
			audits:     forkWithCommand("cat FABRICREF/webster/state.json"),
			prepare:    func(t *testing.T, fx *recordFixture) { fx.Deps.RefMatcher = fabricMatcher{} },
			report:     func(fx *recordFixture) string { return validReport(fx.HeadSHA[:9]) },
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if result.Digest.HeadSHA != fx.HeadSHA {
					t.Errorf("failed Digest.HeadSHA = %q; want the full SHA %q", result.Digest.HeadSHA, fx.HeadSHA)
				}
			},
		},
		{
			name:       "a correctness finding wins over a head_sha naming no commit",
			audits:     forkWithCommand("cat FABRICREF/webster/state.json"),
			prepare:    func(t *testing.T, fx *recordFixture) { fx.Deps.RefMatcher = fabricMatcher{} },
			report:     func(fx *recordFixture) string { return validReport("abcdef123") },
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if result.Digest.HeadSHA != "" {
					t.Errorf("failed Digest.HeadSHA = %q; want empty", result.Digest.HeadSHA)
				}
				if errors.Is(err, websterengine.ErrHeadSHAUnresolved) {
					t.Errorf("error = %v; want the correctness failure, not ErrHeadSHAUnresolved", err)
				}
			},
		},
		{
			// A resumed run's fresh Master must be able to consume a report whose fork transcript
			// lives under the crashed session's own subagents directory.
			name: "the audit is keyed on the bracket-opening session",
			audits: func() []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{{Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/crashed-session/subagents/f1.jsonl", ReportReturned: true}}}}
			},
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Deps.State.MasterSessionID = "session-resumed"
				fx.Deps.State.Batches[1].SessionID = "session-crashed"
			},
			report: okReport,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				requireDone(t, result)
				for i, session := range fx.Audit.sessions {
					if session != "session-crashed" {
						t.Errorf("AuditForksIncremental call %d keyed on session %q; want the bracket-opening \"session-crashed\"", i, session)
					}
				}
			},
		},
		{
			// SettleRetry's own "first miss is inconclusive" de-risk, applied through the whole call.
			name: "a transcript appearing on a later tick resolves clean",
			audits: func() []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{{}, {Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/late.jsonl", ReportReturned: true}}}}
			},
			report: okReport,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if result.Digest == nil {
					t.Fatal("RecordResult.Digest = nil; want a distilled digest")
				}
				if len(result.Warnings) != 0 {
					t.Errorf("RecordResult.Warnings = %v; want none", result.Warnings)
				}
				if len(fx.Sleeper.slept) != 1 {
					t.Errorf("Sleeper.slept = %v; want exactly one tick before the late transcript resolved", fx.Sleeper.slept)
				}
			},
		},
		{
			name:   "one new transcript with a valid report persists the terminal digest",
			audits: oneFork,
			report: okReport,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if result.NoReport {
					t.Fatal("RecordResult.NoReport = true; want false (a valid report was present)")
				}
				requireDone(t, result)
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
			},
		},
		{
			// The report file is the fork's contract: a transcript that never ended on a final
			// report but whose report file landed draws no "never returned" warning.
			name: "a report file present drops the never-returned warning",
			audits: func() []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: false}}}}
			},
			report: okReport,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if result.NoReport {
					t.Fatal("RecordResult.NoReport = true; want false (report file present)")
				}
				if warningsContain(result.Warnings, "never returned a final report") {
					t.Errorf("Warnings = %v; want no \"never returned a final report\" warning with the report present", result.Warnings)
				}
			},
		},
		{
			// On the no-report path the warning explains why Master must re-fork.
			name: "no report file keeps the never-returned warning",
			audits: func() []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: false}}}}
			},
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if !result.NoReport {
					t.Fatal("RecordResult.NoReport = false; want true (no report file)")
				}
				if !warningsContain(result.Warnings, `fork "subagents/f1.jsonl" never returned a final report`) {
					t.Errorf("Warnings = %v; want the \"never returned a final report\" warning naming the fork on the no-report path", result.Warnings)
				}
			},
		},
		{
			name: "a parent write outside the worktree is a policy warning",
			audits: func() []shuttleengine.ForkAudit {
				audit := oneForkAudit()
				audit.ParentWrites = []string{"/some/other/hand-written-file.go"}
				return []shuttleengine.ForkAudit{audit}
			},
			report: okReport,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				requireDone(t, result)
				if !warningsContain(result.Warnings, "audit warning (parent-write)", "hand-written-file.go") {
					t.Errorf("Warnings = %v; want a parent-write audit warning naming the write", result.Warnings)
				}
			},
		},
		{
			name:    "a nested agent warns once when the card verify passes",
			audits:  func() []shuttleengine.ForkAudit { return []shuttleengine.ForkAudit{forkNestedAgentAudit()} },
			prepare: func(t *testing.T, fx *recordFixture) { setCardVerify(fx, "exit 0") },
			report:  okReport,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				requireDone(t, result)
				n := 0
				for _, w := range result.Warnings {
					if strings.Contains(w, "audit warning (nested-agent)") {
						n++
					}
				}
				if n != 1 {
					t.Errorf("Warnings = %v; want exactly one audit warning (nested-agent)", result.Warnings)
				}
				if got := fx.Deps.State.Batches[1].AuditWarnings; len(got) != 1 {
					t.Errorf("AuditWarnings = %v; want one entry", got)
				}
			},
		},
		{
			// A fork that touched the fabric checkout through Bash is correctness whatever its
			// command; the failed record names the command and is the state recover-batch
			// proceeds from. A fabric reference has no path recovery could check.
			name:   "a read-only fabric reference fails the batch",
			audits: forkWithCommand("cat /fabric/sibling/webster/state.json"),
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Deps.RefMatcher = fabricPathMatcher("/fabric/sibling")
				setCardVerify(fx, "exit 0")
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				// The reason quotes the command with %q, so inner quotes come back escaped.
				if !warningsContain(result.Digest.Reasons, fmt.Sprintf("%q", "cat /fabric/sibling/webster/state.json")) {
					t.Errorf("Reasons = %v; want the command named", result.Digest.Reasons)
				}
				requireUncheckableFabricReference(t, fx)
				archived(t, fx, 1)
			},
		},
		{
			name:   "a mutating fabric reference fails the batch",
			audits: forkWithCommand("git -C /fabric/sibling checkout HEAD~1 -- webster/state.json"),
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Deps.RefMatcher = fabricPathMatcher("/fabric/sibling")
				setCardVerify(fx, "exit 0")
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if !warningsContain(result.Digest.Reasons, fmt.Sprintf("%q", "git -C /fabric/sibling checkout HEAD~1 -- webster/state.json")) {
					t.Errorf("Reasons = %v; want the command named", result.Digest.Reasons)
				}
				archived(t, fx, 1)
			},
		},
		{
			name:   "a writer no allowlist names fails the batch",
			audits: forkWithCommand(`python3 -c "open('/fabric/sibling/webster/state.json','w').write('{}')"`),
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Deps.RefMatcher = fabricPathMatcher("/fabric/sibling")
				setCardVerify(fx, "exit 0")
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if !warningsContain(result.Digest.Reasons, fmt.Sprintf("%q", `python3 -c "open('/fabric/sibling/webster/state.json','w').write('{}')"`)) {
					t.Errorf("Reasons = %v; want the command named", result.Digest.Reasons)
				}
				archived(t, fx, 1)
			},
		},
		{
			name:   "a Master write to the scratch pause flag records its path as uncheckable",
			audits: oneFork,
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Deps.Geom.ScratchDir = t.TempDir()
				fx.Audit.scripted[0].ParentWrites = []string{filepath.Join(fx.Deps.Geom.ScratchDir, "pause")}
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				pause := filepath.Join(fx.Deps.Geom.ScratchDir, "pause")
				if got := fx.Deps.State.Batches[1].Uncheckable; len(got) != 1 || got[0] != pause {
					t.Errorf("Uncheckable = %v; want [%s]", got, pause)
				}
			},
		},
		{
			name:   "a done report over an unrendered amendment fails the batch with card_amended",
			audits: oneFork,
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Deps.State.Batches[1].AmendedCards = []websterengine.AmendedCard{{Card: "01-json-flag"}}
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				var failed *websterengine.BatchFailedError
				if !errors.As(err, &failed) || !failed.CardAmended {
					t.Errorf("err = %v; want a BatchFailedError with CardAmended", err)
				}
				if !warningsContain(result.Digest.Reasons, "card 01-json-flag was amended after this attempt began") {
					t.Errorf("Reasons = %v; want the amended card named", result.Digest.Reasons)
				}
				if got := fx.Deps.State.Batches[1].Uncheckable; len(got) != 0 {
					t.Errorf("Uncheckable = %v; want empty", got)
				}
				archived(t, fx, 1)
			},
		},
		{
			name:       "a Master write to a tracked file fails the batch yet stays checkable by recovery",
			audits:     oneFork,
			prepare:    parentWrite(trackedFile),
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if got := fx.Deps.State.Batches[1].Uncheckable; len(got) != 0 {
					t.Errorf("Uncheckable = %v; want empty", got)
				}
				// A failing finding is dispositioned failed and never also recorded as a warning.
				st := fx.Deps.State
				if len(st.AuditDispositions) != 1 {
					t.Errorf("AuditDispositions = %v; want one entry", st.AuditDispositions)
				}
				for id, disposition := range st.AuditDispositions {
					if disposition != "failed" {
						t.Errorf("AuditDispositions[%q] = %q; want failed", id, disposition)
					}
				}
				if len(st.AuditWarnings) != 0 || len(st.Batches[1].AuditWarnings) != 0 {
					t.Errorf("AuditWarnings = %v, batch AuditWarnings = %v; want none", st.AuditWarnings, st.Batches[1].AuditWarnings)
				}
			},
		},
		{
			name:       "a Master write to a tracked file fails the batch with no report at all",
			audits:     oneFork,
			prepare:    parentWrite(trackedFile),
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if !warningsContain(result.Digest.Reasons, trackedFile(fx)) {
					t.Errorf("Reasons = %v; want the written path named", result.Digest.Reasons)
				}
				archived(t, fx, 0)
			},
		},
		{
			name:       "a correctness finding fails the batch even over a FAILED report",
			audits:     oneFork,
			prepare:    parentWrite(trackedFile),
			report:     failedReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				archived(t, fx, 1)
			},
		},
		{
			name:       "a policy finding with a failing card verify fails on the same call",
			audits:     func() []shuttleengine.ForkAudit { return []shuttleengine.ForkAudit{forkNestedAgentAudit()} },
			prepare:    func(t *testing.T, fx *recordFixture) { setCardVerify(fx, "exit 1") },
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
					t.Errorf("live report stat = %v; want it archived away", statErr)
				}
				archived(t, fx, 1)
			},
		},
		{
			name:   "a Master write to the run's state.json fails the batch, naming the path",
			audits: oneFork,
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Deps.Geom.WebsterDir = t.TempDir()
				fx.Audit.scripted[0].ParentWrites = []string{filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")}
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if state := filepath.Join(fx.Deps.Geom.WebsterDir, "state.json"); !warningsContain(result.Digest.Reasons, state) {
					t.Errorf("Reasons = %v; want the state.json path %q named", result.Digest.Reasons, state)
				}
			},
		},
		{
			name:   "a fork writing a Master contract file fails the batch and archives its report",
			audits: oneFork,
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Audit.scripted[0].Forks[0].WritePaths = []string{fx.Deps.OutcomePath}
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				archived(t, fx, 1)
			},
		},
		{
			// The report is archived, the card file is named as a suspect path, and the failed
			// record is one recover-batch proceeds from.
			name:   "a fork writing a card file under the plan directory fails the batch",
			audits: oneFork,
			prepare: func(t *testing.T, fx *recordFixture) {
				card := filepath.Join(fx.Deps.Geom.PlanDir, "03-x.md")
				fx.Audit.scripted[0].Forks[0].WritePaths = []string{card}
				// A plan file is checkable by recovery only against recorded plan hashes.
				fx.Deps.State.PlanFileHashes = map[string]string{"03-x.md": "recorded"}
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if card := filepath.Join(fx.Deps.Geom.PlanDir, "03-x.md"); !warningsContain(result.Digest.Reasons, card) {
					t.Errorf("Reasons = %v; want the card file %q named", result.Digest.Reasons, card)
				}
				archived(t, fx, 1)

				rfx := newRecoverFixture(t)
				failed := *fx.Deps.State.Batches[1]
				rfx.Deps.State.Batches[1] = &failed
				clk := &recoverFakeClock{now: time.Unix(0, 0)}
				if _, spawned, err := websterengine.RecoverSpawnOrAttach(rfx.Deps, 1, clk); err != nil || !spawned {
					t.Fatalf("RecoverSpawnOrAttach() = spawned %v, err %v; want a recovery strand spawned", spawned, err)
				}
			},
		},
		{
			// The 2026-09-30 incident: a fork's fabric reference on an otherwise clean batch no
			// longer wedges every retry.
			name:   "a fabric reference on a clean batch fails once and no retry is wedged by an audit refusal",
			audits: forkWithCommand("cat FABRICREF/webster/state.json"),
			prepare: func(t *testing.T, fx *recordFixture) {
				fx.Deps.RefMatcher = fabricMatcher{}
			},
			report:     okReport,
			wantFailed: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if !strings.Contains(err.Error(), "recover-batch 01") {
					t.Errorf("first RecordBatch() error = %q; want it to name recover-batch 01", err.Error())
				}
				archived(t, fx, 1)

				_, err = websterengine.RecordBatch(fx.Deps, 1)
				if err == nil || !strings.Contains(err.Error(), "already terminal") || strings.Contains(err.Error(), "violation") {
					t.Fatalf("second RecordBatch() error = %v; want only the already-terminal refusal, no audit refusal", err)
				}

				rfx := newRecoverFixture(t)
				failed := *fx.Deps.State.Batches[1]
				rfx.Deps.State.Batches[1] = &failed
				clk := &recoverFakeClock{now: time.Unix(0, 0)}
				// A fabric reference has no path recovery could check, so recover-batch names the reset-to-start route instead of spawning.
				if _, spawned, err := websterengine.RecoverSpawnOrAttach(rfx.Deps, 1, clk); !errors.Is(err, websterengine.ErrRecoveryNeedsFresh) || spawned || !strings.Contains(err.Error(), "1) lyx webster reset --to start; 2) lyx webster run") {
					t.Fatalf("RecoverSpawnOrAttach() = spawned %v, err %v; want ErrRecoveryNeedsFresh naming the reset-to-start route", spawned, err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRecordFixture(t, tc.audits())
			if tc.prepare != nil {
				tc.prepare(t, fx)
			}
			if tc.report != nil {
				writeReport(t, fx.ReportsDir, tc.report(fx))
			}

			result, err := websterengine.RecordBatch(fx.Deps, 1)
			if tc.wantFailed {
				if !errors.Is(err, websterengine.ErrBatchFailed) {
					t.Fatalf("RecordBatch() error = %v; want ErrBatchFailed", err)
				}
				if !strings.Contains(err.Error(), "recover-batch") {
					t.Errorf("error = %q; want it to name recover-batch", err.Error())
				}
				bs := fx.Deps.State.Batches[1]
				if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed || !result.Failed {
					t.Errorf("batch = %+v, result = %+v; want terminal failed", bs, result)
				}
			} else if err != nil {
				t.Fatalf("RecordBatch() error = %v; want nil", err)
			}
			if tc.check != nil {
				tc.check(t, fx, result, err)
			}
		})
	}
}

// TestRecordBatch_OneNewTranscriptNoReport_RetrySeesExactlyOneNew proves the no_report ladder: a
// fork transcript with no report yet returns NoReport true, leaves the batch non-terminal, and
// STILL advances attribution — so a second record-batch call (after Master's re-fork) sees exactly
// its own new transcript and resolves clean, never re-counting the first one.
//
//testtiming:keep pins attribution advancing across a no-report call and the second call classifying only its own new transcript; the audit-outcome table only covers single calls
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

// TestRecordBatch_ResumedForkAcrossNoReportCall proves a fork stopped and resumed across a no-report call is attributed on the next call:
// the bracket's own transcript counts when it wrote the batch's report and is re-audited in full, its earlier findings are not reported again,
// and a bracket whose transcripts never wrote the report still meets ErrNoForkTranscripts.
// Every attributed transcript is held once in the seen set, the batch's fork transcripts and its bracket transcripts, so no other claim path takes it.
func TestRecordBatch_ResumedForkAcrossNoReportCall(t *testing.T) {
	t.Parallel()

	const f1, f2 = "subagents/f1.jsonl", "subagents/f2.jsonl"
	ownReportWrite := func(fx *recordFixture) []string {
		return []string{filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))}
	}
	// audits builds the scripted audits from fx, so a row can name the batch's report path.
	cases := []struct {
		name   string
		audits func(fx *recordFixture) []shuttleengine.ForkAudit
		// prepare edits the state before the first call.
		prepare func(fx *recordFixture)
		// skipFirst starts at the second call, as after a re-begin.
		skipFirst bool
		check     func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error)
	}{
		{
			name: "a resumed fork's report is recorded and the bracket transcript re-read in full",
			audits: func(fx *recordFixture) []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1}}},
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1, WritePaths: ownReportWrite(fx), ReportReturned: true}}},
				}
			},
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if err != nil || result.Digest == nil || !fx.Deps.State.Batches[1].Terminal {
					t.Fatalf("RecordBatch() = %+v, %v; want the batch recorded", result, err)
				}
				// Call 0 is the first record-batch call; the second call's first fetch is call 1.
				if fx.Audit.seen[0][f1] || fx.Audit.seen[1][f1] {
					t.Errorf("seen sets handed to the engine = %v; want the bracket's transcript excluded", fx.Audit.seen)
				}
				if got := fx.Deps.State.SeenForkTranscripts; !slices.Equal(got, []string{f1}) {
					t.Errorf("SeenForkTranscripts = %v; want [%s] once", got, f1)
				}
			},
		},
		{
			name: "a later policy finding is reported and an earlier dispositioned one is not",
			audits: func(fx *recordFixture) []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1, AgentCalls: 1}}},
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1, AgentCalls: 1, WritePaths: ownReportWrite(fx), ReportReturned: true}}},
				}
			},
			prepare: func(fx *recordFixture) { setCardVerify(fx, "exit 0") },
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if err != nil || result.Digest == nil {
					t.Fatalf("RecordBatch() = %+v, %v; want the batch recorded", result, err)
				}
				if warningsContain(result.Warnings, "nested-agent") {
					t.Errorf("second call Warnings = %v; want the first call's nested-agent finding not reported again", result.Warnings)
				}
				if got := fx.Deps.State.Batches[1].AuditWarnings; len(got) != 1 {
					t.Errorf("AuditWarnings = %v; want the finding held once", got)
				}
			},
		},
		{
			name: "a policy finding from the transcript's later part is reported",
			audits: func(fx *recordFixture) []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1}}},
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1, AgentCalls: 1, WritePaths: ownReportWrite(fx), ReportReturned: true}}},
				}
			},
			prepare: func(fx *recordFixture) { setCardVerify(fx, "exit 0") },
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if err != nil || !warningsContain(result.Warnings, "nested-agent") {
					t.Errorf("RecordBatch() warnings = %v, err = %v; want the later nested-agent finding reported", result.Warnings, err)
				}
			},
		},
		{
			name: "a bracket transcript that never wrote the report is archived as unattributable",
			audits: func(fx *recordFixture) []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1}}},
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1, WritePaths: []string{"/elsewhere/notes.md"}, ReportReturned: true}}},
				}
			},
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if !errors.Is(err, websterengine.ErrNoForkTranscripts) || !errors.Is(err, websterengine.ErrReportArchived) {
					t.Errorf("RecordBatch() error = %v; want ErrReportArchived wrapping ErrNoForkTranscripts", err)
				}
			},
		},
		{
			name: "a bracket transcript of an earlier bracket does not count after a re-begin",
			audits: func(fx *recordFixture) []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1, WritePaths: ownReportWrite(fx), ReportReturned: true}}}}
			},
			prepare: func(fx *recordFixture) {
				fx.Deps.State.SeenForkTranscripts = []string{f1}
				fx.Deps.State.Batches[1].ForkTranscripts = []string{f1}
			},
			skipFirst: true,
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if !errors.Is(err, websterengine.ErrNoForkTranscripts) {
					t.Errorf("RecordBatch() error = %v; want ErrNoForkTranscripts", err)
				}
			},
		},
		{
			name: "a fork re-launched in the open bracket is attributed within the settle window",
			audits: func(fx *recordFixture) []shuttleengine.ForkAudit {
				return []shuttleengine.ForkAudit{
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1}}},
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1}}},
					{Forks: []shuttleengine.ForkReport{{TranscriptPath: f1}, {TranscriptPath: f2, WritePaths: ownReportWrite(fx), ReportReturned: true}}},
				}
			},
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, err error) {
				if err != nil || len(result.Warnings) != 0 || !fx.Deps.State.Batches[1].Terminal {
					t.Fatalf("RecordBatch() = %+v, %v; want the batch recorded cleanly on f2 alone", result, err)
				}
				want := []string{f1, f2}
				bs := fx.Deps.State.Batches[1]
				if !slices.Equal(fx.Deps.State.SeenForkTranscripts, want) || !slices.Equal(bs.ForkTranscripts, want) || !slices.Equal(bs.BracketTranscripts, want) {
					t.Errorf("Seen = %v, ForkTranscripts = %v, BracketTranscripts = %v; want each %v with no duplicate", fx.Deps.State.SeenForkTranscripts, bs.ForkTranscripts, bs.BracketTranscripts, want)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRecordFixture(t, nil)
			fx.Audit.scripted = tc.audits(fx)
			if tc.prepare != nil {
				tc.prepare(fx)
			}
			if !tc.skipFirst {
				result, err := websterengine.RecordBatch(fx.Deps, 1)
				if err != nil || !result.NoReport {
					t.Fatalf("first RecordBatch() = %+v, %v; want a no-report call", result, err)
				}
				bs := fx.Deps.State.Batches[1]
				if !slices.Equal(fx.Deps.State.SeenForkTranscripts, []string{f1}) || !slices.Equal(bs.ForkTranscripts, []string{f1}) || !slices.Equal(bs.BracketTranscripts, []string{f1}) {
					t.Fatalf("after the no-report call Seen = %v, ForkTranscripts = %v, BracketTranscripts = %v; want each [%s]", fx.Deps.State.SeenForkTranscripts, bs.ForkTranscripts, bs.BracketTranscripts, f1)
				}
			}
			writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
			result, err := websterengine.RecordBatch(fx.Deps, 1)
			tc.check(t, fx, result, err)
		})
	}
}

// TestRecordBatch_RetryNeverDuplicatesWarning proves a finding first seen on a no-report call is warned once:
// the later OK report re-runs the verify and records done without a second warning.
//
//testtiming:keep pins the warning staying recorded once across a no-report call and its retry, and the card verify re-running on the later OK report; the audit-outcome table only covers single calls
func TestRecordBatch_RetryNeverDuplicatesWarning(t *testing.T) {
	audit := shuttleengine.ForkAudit{
		Forks:       []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
		NamedSpawns: 1,
	}
	audit2 := audit
	audit2.Forks = []shuttleengine.ForkReport{
		{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true},
		{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true},
	}
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{audit, audit2})
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

// TestRecordBatch_RetryFailingVerifyFailsNamingEarlierWarning proves the same flow with a failing verify fails the batch,
// and the earlier recorded warning is among the reasons.
func TestRecordBatch_RetryFailingVerifyFailsNamingEarlierWarning(t *testing.T) {
	audit := shuttleengine.ForkAudit{
		Forks:       []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
		NamedSpawns: 1,
	}
	audit2 := audit
	audit2.Forks = []shuttleengine.ForkReport{
		{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true},
		{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true},
	}
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{audit, audit2})
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
	if !warningsContain(result.Digest.Reasons, "audit warning (named-spawn)") {
		t.Errorf("Reasons = %v; want the earlier warning among them", result.Digest.Reasons)
	}
	if !warningsContain(result.Digest.Reasons, "verify exit 1 exited 1") {
		t.Errorf("Reasons = %v; want the verify failure among them", result.Digest.Reasons)
	}
}

// TestRecordBatch_NamedSpawnWarnsOnceAcrossBatches proves a whole-session parent finding is dispositioned by the first record-batch that reports it:
// the next batch in the same session records done with no refusal and no repeated warning.
//
//testtiming:keep pins a session-wide finding dispositioned by the first batch and not re-warned on the second batch; the audit-outcome table only records single batches
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

// TestRecordBatch_ForgedTerminalRecordFails proves a fork that marks its own batch done by writing state.json is audited before the "already terminal" refusal:
// the batch fails with its reasons naming fork-state-write, while a terminal batch with no new transcript still refuses untouched.
func TestRecordBatch_ForgedTerminalRecordFails(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}},
	}})
	fx.Deps.State.Batches[1].Terminal = true
	fx.Deps.State.Batches[1].Status = websterengine.DigestStatusDone
	fx.Deps.State.Batches[1].Digest = &websterengine.Digest{Batch: "01-json-flag", Status: websterengine.DigestStatusDone, HeadSHA: fx.HeadSHA}

	// No new transcript: the refusal stands and nothing moves.
	fx.Deps.State.SeenForkTranscripts = []string{"subagents/f1.jsonl"}
	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil || !strings.Contains(err.Error(), "already terminal") {
		t.Fatalf("RecordBatch() with no new transcript error = %v; want the already-terminal refusal", err)
	}
	if got := fx.Deps.State.Batches[1].Status; got != websterengine.DigestStatusDone {
		t.Fatalf("status after the plain refusal = %q; want it unchanged (done)", got)
	}

	// A new transcript that wrote state.json fails the batch.
	fx.Deps.State.SeenForkTranscripts = nil
	fx.Audit.scripted[0].Forks[0].WritePaths = []string{filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")}
	result, err := websterengine.RecordBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecordBatch() error = %v; want ErrBatchFailed", err)
	}
	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed {
		t.Errorf("record = terminal %v, status %q; want terminal failed", bs.Terminal, bs.Status)
	}
	if !warningsContain(result.Digest.Reasons, "fork-state-write") {
		t.Errorf("Reasons = %v; want fork-state-write named", result.Digest.Reasons)
	}
}

// TestRecordBatch_TerminalAuditSkipsAnotherForksTranscript proves a repeated record-batch on a done batch never attributes another fork's unseen transcript to it:
// while a later fork batch of the same session is open, or once the verify-gate report exists, the call refuses as already terminal and consumes nothing.
func TestRecordBatch_TerminalAuditSkipsAnotherForksTranscript(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, fx *recordFixture)
	}{
		{"later batch open", func(t *testing.T, fx *recordFixture) {
			fx.Deps.State.Batches[2] = &websterengine.BatchState{Slug: "later", Kind: "fork", SessionID: "session-1"}
		}},
		{"verify-gate report present", func(t *testing.T, fx *recordFixture) {
			if err := os.WriteFile(websterengine.VerifyGateReportPath(fx.ReportsDir), []byte("attempt: 1\ncap: 3\n"), 0o644); err != nil {
				t.Fatalf("write verify-gate report: %v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newRecordFixture(t, []shuttleengine.ForkAudit{{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f2.jsonl", ReportReturned: true}},
			}})
			fx.Deps.State.Batches[1].Terminal = true
			fx.Deps.State.Batches[1].Status = websterengine.DigestStatusDone
			fx.Deps.State.Batches[1].Digest = &websterengine.Digest{Batch: "01-json-flag", Status: websterengine.DigestStatusDone, HeadSHA: fx.HeadSHA}
			fx.Audit.scripted[0].Forks[0].WritePaths = []string{filepath.Join(fx.ReportsDir, websterengine.ReportFileName(2, "later"))}
			tt.setup(t, fx)

			_, err := websterengine.RecordBatch(fx.Deps, 1)
			if err == nil || !strings.Contains(err.Error(), "already terminal") {
				t.Fatalf("RecordBatch() error = %v; want the already-terminal refusal", err)
			}
			if got := fx.Deps.State.Batches[1].Status; got != websterengine.DigestStatusDone {
				t.Errorf("status = %q; want it unchanged (done)", got)
			}
			if len(fx.Deps.State.SeenForkTranscripts) != 0 {
				t.Errorf("SeenForkTranscripts = %v; want the other fork's transcript left unseen", fx.Deps.State.SeenForkTranscripts)
			}
		})
	}
}

// TestRecordBatch_HeadSHAMismatchErrors proves a batch-report whose own self-reported head_sha
// disagrees with the worktree's actual current HEAD is a hard error, naming both — the fork's
// report and the repo it left behind must never be trusted to agree silently.
//
//testtiming:keep pins the head_sha mismatch refusal naming the worktree's actual HEAD and the same call recording once the report is corrected; the parent-moved-HEAD table only refuses moved heads that match the report
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

	// A value that is not hex is no abbreviation, so it meets the mismatch refusal too.
	writeReport(t, fx.ReportsDir, validReport("main"))
	restore()
	if _, err := websterengine.RecordBatch(fx.Deps, 1); err == nil || errors.Is(err, websterengine.ErrHeadSHAUnresolved) || !strings.Contains(err.Error(), fx.HeadSHA) {
		t.Errorf("RecordBatch() with head_sha main error = %v; want the mismatch refusal naming %q", err, fx.HeadSHA)
	}

	// Taking the way forward: the report names the worktree's actual HEAD, and the same call records.
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	restore()
	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Fatalf("RecordBatch() with a corrected head_sha error = %v; want nil", err)
	}
}

// TestRecordBatch_UnresolvableHeadSHARefused proves an abbreviated head_sha naming no commit, or several, is refused with ErrHeadSHAUnresolved and the git rev-parse way forward, never a reset.
// The batch stays non-terminal with the report in place, and the same call records once the report names the full HEAD.
func TestRecordBatch_UnresolvableHeadSHARefused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// headSHA is the report's head_sha and prepare registers any extra commit it needs.
		headSHA func(fx *recordFixture) string
		prepare func(fx *recordFixture)
		want    string
	}{
		{
			name:    "no commit",
			headSHA: func(*recordFixture) string { return "abcdef123" },
			want:    "names no commit",
		},
		{
			name:    "two commits",
			headSHA: func(fx *recordFixture) string { return fx.HeadSHA[:9] },
			prepare: func(fx *recordFixture) { fx.Git.parents[fx.HeadSHA[:9]+strings.Repeat("0", 31)] = nil },
			want:    "names 2 commits",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRecordFixture(t, []shuttleengine.ForkAudit{oneForkAudit()})
			if tc.prepare != nil {
				tc.prepare(fx)
			}
			writeReport(t, fx.ReportsDir, validReport(tc.headSHA(fx)))
			restore := snapshotRecordState(fx)

			_, err := websterengine.RecordBatch(fx.Deps, 1)
			if !errors.Is(err, websterengine.ErrHeadSHAUnresolved) {
				t.Fatalf("RecordBatch() error = %v; want ErrHeadSHAUnresolved", err)
			}
			for _, want := range []string{tc.want, "`git rev-parse HEAD`"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err.Error(), want)
				}
			}
			if strings.Contains(err.Error(), "reset") {
				t.Errorf("error %q names a reset; want only the git rev-parse way forward", err.Error())
			}
			assertBatchOpen(t, fx)
			if got := archivedReports(t, fx.ReportsDir); len(got) != 0 {
				t.Errorf("archived reports = %v; want the report left in place", got)
			}

			writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
			restore()
			if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
				t.Fatalf("RecordBatch() with the full head_sha error = %v; want nil", err)
			}
		})
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

// TestRecordBatch_MissingSessionTranscriptArchivesReport proves the TRUE cross-machine resume failure — the bracket-opening session's transcript file does not exist on this machine at all, so the audit read itself fails with fs.ErrNotExist — archives the report, keeps the batch begun, and explains the machine-local transcripts with the begin-batch way forward (found live in crucible round fable-r3).
// errors.Is must still see the underlying fs.ErrNotExist.
func TestRecordBatch_MissingSessionTranscriptArchivesReport(t *testing.T) {
	fx := newRecordFixture(t, nil)
	fx.Audit.auditErr = fmt.Errorf("claudeengine: read parent transcript %q: %w", "/nope/session.jsonl", fs.ErrNotExist)
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
// a Create target that still does not resolve against the worktree's actual post-card tree fails the batch terminally with its findings as reasons and the report archived.
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

// deleteReferencedBatches returns two batches, card 1 (json-flag) that deletes internal/foo#Gone and the unbegun card 2 (later) that edits internal/foo/user.go, and writes both files into worktree: impl.go still declares Gone, and user.go calls it on line 4 when referenced is true.
func deleteReferencedBatches(t *testing.T, worktree string, referenced bool) []batcher.Batch {
	t.Helper()
	writeWorktreeFile(t, worktree, "internal/foo/impl.go", "package foo\n\nfunc Gone() {}\n")
	user := "package foo\n\nfunc user() {}\n"
	if referenced {
		user = "package foo\n\nfunc user() {\n\tGone()\n}\n"
	}
	writeWorktreeFile(t, worktree, "internal/foo/user.go", user)
	deleting := planparser.Card{
		Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "delete Gone",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeDelete, Refs: []string{"internal/foo#Gone"}}},
	}
	later := planparser.Card{
		Number: 2, Slug: "later", Title: "later", Intent: "edit the user",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/foo/user.go"}}},
	}
	return []batcher.Batch{{Cards: []planparser.Card{deleting}}, {Cards: []planparser.Card{later}}}
}

// TestRecordBatch_DeleteNotDoneNamesLaterCard proves a delete-not-done failure caused by an unbegun later card still referencing the target names that card and the reference and moves the way forward to the plan edit, or to the --fresh steps when the record also lists uncheckable entries;
// a delete-not-done with no later reference keeps recover-batch.
func TestRecordBatch_DeleteNotDoneNamesLaterCard(t *testing.T) {
	tests := []struct {
		name        string
		referenced  bool
		uncheckable []string
		wantIn      []string
		wantNotIn   []string
	}{
		{
			name:       "a later card still references the target",
			referenced: true,
			wantIn:     []string{"delete-not-done", "2-later", "internal/foo/user.go:4", "way forward: move the delete to a card after", "lyx webster rebaseline --card NN", "lyx webster recover-batch 01"},
			wantNotIn:  []string{"reset --to start"},
		},
		{
			name:        "an uncheckable record names the fresh restart",
			referenced:  true,
			uncheckable: []string{"fabric-reference: Bash command references the fabric"},
			wantIn:      []string{"2-later", "internal/foo/user.go:4", "way forward: 1) lyx webster reset --to start; 2) lyx webster run"},
			wantNotIn:   []string{"rebaseline"},
		},
		{
			name:      "no later reference keeps recover-batch",
			wantIn:    []string{"delete-not-done", "way forward: lyx webster recover-batch 01"},
			wantNotIn: []string{"2-later", "rebaseline"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newRecordFixture(t, []shuttleengine.ForkAudit{
				{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
			})
			writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
			fx.Deps.Batches = deleteReferencedBatches(t, fx.Worktree, tt.referenced)
			fx.Deps.Plan.Cards = []planparser.Card{fx.Deps.Batches[0].Cards[0], fx.Deps.Batches[1].Cards[0]}
			fx.Deps.State.Batches[1].Uncheckable = tt.uncheckable

			result, err := websterengine.RecordBatch(fx.Deps, 1)
			if !errors.Is(err, websterengine.ErrBatchFailed) {
				t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrBatchFailed)", err)
			}
			if result == nil || !result.Failed {
				t.Fatalf("RecordBatch() result = %+v; want Failed", result)
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
			for _, bad := range tt.wantNotIn {
				if strings.Contains(err.Error(), bad) {
					t.Errorf("error %q contains %q", err, bad)
				}
			}
			if bs := fx.Deps.State.Batches[1]; !bs.Terminal || bs.Status != websterengine.DigestStatusFailed {
				t.Errorf("BatchState = terminal %v status %q; want terminal failed", bs.Terminal, bs.Status)
			}
		})
	}
}

// writeRecordPlanDir writes a minimal, valid on-disk plan directory holding one card whose body is
// cardBody, returning the directory and its freshly parsed *planparser.Plan — the record-batch
// wiring test's own plan-fixture builder, package-local to this file since planglyph's own
// writePlanFixture is unexported to its package.
func writeRecordPlanDir(t *testing.T, cardBody string) (string, *planparser.Plan) {
	t.Helper()
	dir := t.TempDir()
	plankit.Write(t, dir, plankit.Plan{
		Approved: true,
		Language: "go",
		Framing:  "framing",
		Cards:    []plankit.Card{{Number: 1, Slug: "json-flag", Summary: "summary"}},
	})

	// The card body is the part under test, so it replaces the card file plankit rendered.
	content := "# Card 1 — json-flag\n\n" + cardBody + "\n"
	if err := os.WriteFile(filepath.Join(dir, "01-json-flag.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) returned error: %v", dir, err)
	}
	return dir, plan
}

// TestRecordBatch_RefusesPlanEditedSinceBegin proves a card's Verify weakened on disk after begin-batch, which no Write/Edit audit sees, is refused with ErrFingerprintMismatch
// before attribution advances: the batch stays open, the transcripts stay unseen, the plan hashes are not re-recorded over the edit and the begun card's recorded hashes stay unchanged,
// including when a canonicalizing planglyph.ValidateDispatch ran after the edit with no restamp, as validate's path does when its edit check refuses.
func TestRecordBatch_RefusesPlanEditedSinceBegin(t *testing.T) {
	for _, canonicalize := range []bool{false, true} {
		t.Run(fmt.Sprintf("canonicalize=%v", canonicalize), func(t *testing.T) {
			t.Parallel()
			fx := newRecordFixture(t, []shuttleengine.ForkAudit{
				{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
			})
			planDir, plan := writeRecordPlanDir(t, "**Intent:** x.\n\n**Verify:** go test ./...\n")
			if canonicalize {
				files := plankit.Render(plankit.Plan{
					Approved: true,
					Language: "go",
					Framing:  "framing",
					Cards: []plankit.Card{
						{Number: 1, Slug: "json-flag", Summary: "summary"},
						{
							Number:  2,
							Slug:    "pending",
							Summary: "declares a draft handle",
							Groups:  []plankit.Group{{Label: "Create", Targets: []string{"plan:internal/foo#Barr` -> `func Bar()"}}},
							Intent:  "declare a draft handle.",
						},
					},
				})
				// Card 1 stays as the test wrote it; only the overview and the new card 2 are added.
				for _, name := range []string{"00-overview.md", "02-pending.md"} {
					if err := os.WriteFile(filepath.Join(planDir, name), files[name], 0o644); err != nil {
						t.Fatalf("write %s: %v", name, err)
					}
				}
				var err error
				if plan, err = planparser.ParsePlan(planDir); err != nil {
					t.Fatalf("ParsePlan() error = %v", err)
				}
			}
			fx.Deps.Geom.PlanDir = planDir
			fx.Deps.Plan = plan
			if err := websterengine.RestampPlanBaseline(fx.Deps.State, planDir, fx.Deps.Geom.WebsterDir); err != nil {
				t.Fatalf("RestampPlanBaseline() error = %v", err)
			}
			cardPath := filepath.Join(planDir, "01-json-flag.md")
			original, err := os.ReadFile(cardPath)
			if err != nil {
				t.Fatalf("read card: %v", err)
			}
			sum := sha256.Sum256(original)
			fx.Deps.State.Batches[1].CardHashes = map[string]string{"01-json-flag": hex.EncodeToString(sum[:])}
			cardHashesBefore := fmt.Sprint(fx.Deps.State.Batches[1].CardHashes)
			writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))

			if err := os.WriteFile(cardPath, []byte("# Card 1 — json-flag\n\n**Intent:** x.\n\n**Verify:** true\n"), 0o644); err != nil {
				t.Fatalf("weaken card verify: %v", err)
			}
			if canonicalize {
				edited, err := planparser.ParsePlan(planDir)
				if err != nil {
					t.Fatalf("ParsePlan() error = %v", err)
				}
				if _, err := planglyph.ValidateDispatch(edited, fx.Worktree, nil, nil); err != nil {
					t.Fatalf("ValidateDispatch() error = %v", err)
				}
				rewritten, err := os.ReadFile(filepath.Join(planDir, "02-pending.md"))
				if err != nil {
					t.Fatalf("read card 2: %v", err)
				}
				if !strings.Contains(string(rewritten), "plan:internal/foo#Bar`") {
					t.Fatalf("card 2 = %q; want the handle canonicalized, or the fixture exercises no rewrite", rewritten)
				}
			}
			hashesBefore := fmt.Sprint(fx.Deps.State.PlanFileHashes)
			fingerprintBefore := fx.Deps.State.PlanFingerprint

			result, err := websterengine.RecordBatch(fx.Deps, 1)
			if !errors.Is(err, websterengine.ErrFingerprintMismatch) {
				t.Fatalf("RecordBatch() error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
			}
			if result != nil {
				t.Errorf("RecordBatch() result = %+v; want nil on a refusal", result)
			}
			if !strings.Contains(err.Error(), "restore-plan") {
				t.Errorf("RecordBatch() error = %q; want the restore-plan way forward", err)
			}
			if bs := fx.Deps.State.Batches[1]; bs.Terminal {
				t.Errorf("BatchState.Terminal = true; want the batch left open")
			}
			if len(fx.Deps.State.SeenForkTranscripts) != 0 {
				t.Errorf("State.SeenForkTranscripts = %v; want none (attribution must not advance)", fx.Deps.State.SeenForkTranscripts)
			}
			if got := fmt.Sprint(fx.Deps.State.PlanFileHashes); got != hashesBefore {
				t.Errorf("State.PlanFileHashes = %s; want %s unchanged", got, hashesBefore)
			}
			if fx.Deps.State.PlanFingerprint != fingerprintBefore {
				t.Errorf("State.PlanFingerprint changed to %q; want %q", fx.Deps.State.PlanFingerprint, fingerprintBefore)
			}
			if got := fmt.Sprint(fx.Deps.State.Batches[1].CardHashes); got != cardHashesBefore {
				t.Errorf("CardHashes = %s; want %s unchanged by the refusal", got, cardHashesBefore)
			}
		})
	}
}

// TestRecordBatch_ScopeGuardDegradesOnDeltaUnavailable proves the guard's own degradation path: a
// delta infrastructure error (an unresolvable start SHA) records the guard-could-not-run notice
// and never panics — the batch still terminates, since neither the done-checks above (their own
// Resolve, unaffected) nor BindHandles (this card's card carries no Create declaration) has
// anything to block on.
func TestRecordBatch_ScopeGuardDegradesOnDeltaUnavailable(t *testing.T) {
	fx := newRecordFixture(t, []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	})
	writeReport(t, fx.ReportsDir, validReport(fx.HeadSHA))
	fx.Deps.State.Batches[1].StartSHA = "does-not-exist-rev"
	fx.Index.deltaErr = fmt.Errorf("%w: delta does-not-exist-rev..%s: unknown revision", planglyph.ErrQuarryUnavailable, fx.HeadSHA)

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

// parentMerge simulates a clean parent merge-in on top of the current head, returning the merge commit's SHA.
func parentMerge(fx *recordFixture) string {
	merge, _ := fx.Git.merge("")
	return merge
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

// TestRecordBatch_ParentMovedHead asserts what record-batch does when HEAD moved past the report's head_sha:
// parent merge-ins landing after the fork's commit record the batch at the report's own head_sha with one warning naming each merge,
// the delta is the fork's own StartSHA..head_sha range, and any other movement — a non-merge commit or a merge carrying an extra edit — is refused with the reset way forward, the batch left open, and the same call recording once HEAD is moved back.
//
//testtiming:keep pins each moved-HEAD shape's record or refusal, the warning naming every merge, the fork's own delta range and the retry once HEAD is moved back; each covering test reaches one shape
func TestRecordBatch_ParentMovedHead(t *testing.T) {
	t.Parallel()

	resetWayForward := func(fx *recordFixture) string {
		return "way forward: 1) lyx webster reset --to report-head --batch 01"
	}
	cases := []struct {
		name string
		// move changes HEAD (and the plan, when the case needs it) and returns the merge commits it made.
		move func(fx *recordFixture) (merges []string)
		// wantRefusal lists what the refusal contains; nil means the call records.
		wantRefusal func(fx *recordFixture, newHead string) []string
		check       func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, merges []string)
	}{
		{
			// The merge commit is both the new HEAD and the one walked merge.
			name: "a parent merge after the fork commit records at the report's head_sha",
			move: func(fx *recordFixture) []string { return []string{parentMerge(fx)} },
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, merges []string) {
				if result.Digest == nil || !fx.Deps.State.Batches[1].Terminal {
					t.Fatalf("RecordBatch() digest = %+v; want a terminal batch", result.Digest)
				}
				if got := fx.Deps.State.Batches[1].CardSHAs; len(got) != 1 || got[0] != fx.HeadSHA {
					t.Errorf("CardSHAs = %v; want [%s]", got, fx.HeadSHA)
				}
				if len(result.Warnings) != 1 {
					t.Fatalf("Warnings = %v; want exactly one", result.Warnings)
				}
				for _, want := range []string{fx.HeadSHA, "HEAD \"" + merges[0] + "\"", "(" + merges[0] + ")"} {
					if !strings.Contains(result.Warnings[0], want) {
						t.Errorf("warning %q missing %q", result.Warnings[0], want)
					}
				}
			},
		},
		{
			name: "an abbreviation a parent merge sits above is accepted with the full SHA in the warning",
			move: func(fx *recordFixture) []string {
				path := filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))
				if err := os.WriteFile(path, []byte(validReport(fx.HeadSHA[:9])), 0o644); err != nil {
					panic(err)
				}
				return []string{parentMerge(fx)}
			},
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, merges []string) {
				if !fx.Deps.State.Batches[1].Terminal || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], `head_sha "`+fx.HeadSHA+`"`) {
					t.Errorf("Warnings = %v, terminal = %v; want a recorded batch with one warning naming the full SHA %s", result.Warnings, fx.Deps.State.Batches[1].Terminal, fx.HeadSHA)
				}
			},
		},
		{
			name: "two parent merges after the fork commit name both in one warning",
			move: func(fx *recordFixture) []string { return []string{parentMerge(fx), parentMerge(fx)} },
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, merges []string) {
				if !fx.Deps.State.Batches[1].Terminal {
					t.Fatal("batch is not terminal")
				}
				if len(result.Warnings) != 1 {
					t.Fatalf("Warnings = %v; want exactly one", result.Warnings)
				}
				for _, want := range merges {
					if !strings.Contains(result.Warnings[0], want) {
						t.Errorf("warning %q missing merge %q", result.Warnings[0], want)
					}
				}
			},
		},
		{
			// What the parent side brought in raises no scope, drift or bind finding.
			name: "the parent merge's symbols are not the batch's own",
			move: func(fx *recordFixture) []string {
				fx.Deps.Plan.Cards[0].Targets = []string{"unrelated/thing#Nothing"}
				return []string{parentMerge(fx)}
			},
			check: func(t *testing.T, fx *recordFixture, result *websterengine.RecordResult, merges []string) {
				if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "only merge commits") {
					t.Errorf("Warnings = %v; want only the moved-HEAD warning", result.Warnings)
				}
				for _, w := range result.Warnings {
					if strings.Contains(w, "FromParent") || strings.Contains(w, "scope-outside-plan") {
						t.Errorf("warning %q names the parent side's symbol", w)
					}
				}
				if want := []deltaRange{{from: fx.StartSHA, to: fx.HeadSHA}}; !slices.Equal(fx.Index.deltaRanges, want) {
					t.Errorf("delta ranges asked = %v; want %v", fx.Index.deltaRanges, want)
				}
			},
		},
		{
			name: "a non-merge commit alone is refused",
			move: func(fx *recordFixture) []string {
				fx.Git.commit()
				return nil
			},
			wantRefusal: func(fx *recordFixture, newHead string) []string {
				return []string{fx.HeadSHA, newHead, "only merge commits", resetWayForward(fx)}
			},
		},
		{
			name: "a non-merge commit after a merge is refused",
			move: func(fx *recordFixture) []string {
				parentMerge(fx)
				fx.Git.commit()
				return nil
			},
			wantRefusal: func(fx *recordFixture, newHead string) []string {
				return []string{fx.HeadSHA, newHead, "only merge commits", resetWayForward(fx)}
			},
		},
		{
			name: "several non-merge commits are refused",
			move: func(fx *recordFixture) []string {
				fx.Git.commit()
				fx.Git.commit()
				return nil
			},
			wantRefusal: func(fx *recordFixture, newHead string) []string {
				return []string{fx.HeadSHA, newHead, "only merge commits", resetWayForward(fx)}
			},
		},
		{
			// Content outside the audited StartSHA..head_sha delta can never ride in on a merge commit.
			name: "a parent merge carrying an extra edit is refused",
			move: func(fx *recordFixture) []string {
				fx.Git.merge("its tree differs from the clean merge of its parents, so it carries changes beyond a clean merge")
				return nil
			},
			wantRefusal: func(fx *recordFixture, newHead string) []string {
				return []string{fx.HeadSHA, "carries changes beyond a clean merge", resetWayForward(fx), "2) re-run this verb"}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := parentMergeFixture(t)
			restore := snapshotRecordState(fx)
			merges := tc.move(fx)
			newHead := fx.Git.head

			result, err := websterengine.RecordBatch(fx.Deps, 1)
			if tc.wantRefusal == nil {
				if err != nil {
					t.Fatalf("RecordBatch() error = %v; want nil", err)
				}
				tc.check(t, fx, result, merges)
				return
			}

			if err == nil {
				t.Fatal("RecordBatch() error = nil; want a refusal")
			}
			for _, want := range tc.wantRefusal(fx, newHead) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err.Error(), want)
				}
			}
			assertBatchOpen(t, fx)

			// Taking the way forward: HEAD goes back to the report's head_sha and the same call records.
			fx.Git.head = fx.HeadSHA
			restore()
			if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
				t.Fatalf("retry RecordBatch() error = %v; want nil", err)
			}
		})
	}
}

// TestRecordBatch_MergeInProgressRefusedThenSucceeds proves a conflicting parent merge left in progress refuses record-batch,
// that concluding it by hand is still refused because a conflict resolution is not a clean parent merge,
// and that the same call succeeds once HEAD is moved back to the report's head_sha as the refusal's remedy says.
func TestRecordBatch_MergeInProgressRefusedThenSucceeds(t *testing.T) {
	fx := parentMergeFixture(t)
	restore := snapshotRecordState(fx)

	fx.Git.merging = true

	_, err := websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() error = nil; want a refusal while a merge is in progress")
	}
	if !strings.Contains(err.Error(), "merge --continue") || !strings.Contains(err.Error(), "merge --abort") {
		t.Errorf("error %q; want the merge --continue/--abort pointer", err.Error())
	}
	assertBatchOpen(t, fx)

	fx.Git.merging = false
	fx.Git.merge("its parents do not merge cleanly, so it carries a hand-made conflict resolution")

	restore()
	_, err = websterengine.RecordBatch(fx.Deps, 1)
	if err == nil {
		t.Fatal("RecordBatch() after a hand-resolved merge: error = nil; want a refusal")
	}
	for _, want := range []string{"do not merge cleanly", "way forward: 1) lyx webster reset --to report-head --batch 01"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
	assertBatchOpen(t, fx)

	fx.Git.head = fx.HeadSHA
	restore()
	if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
		t.Fatalf("retry RecordBatch() error = %v; want nil", err)
	}
	if got := fx.Deps.State.Batches[1].CardSHAs; len(got) != 1 || got[0] != fx.HeadSHA {
		t.Errorf("CardSHAs = %v; want [%s]", got, fx.HeadSHA)
	}
}
