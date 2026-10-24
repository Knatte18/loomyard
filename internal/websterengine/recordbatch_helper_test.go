// recordbatch_helper_test.go holds the RecordBatch fixture the tier0 and tier1 record-batch tests share:
// a scripted fork audit, a recording Sleeper and the fully-wired RecordDeps over a temp worktree.
// The fixtures are package-local: the internal and external test packages deliberately do not share a test-helper package.

package websterengine_test

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recordFakeSleeper is a Sleeper that never actually blocks.
// It only records each requested duration, so SettleRetry's retry loop runs a scripted sequence of attempts at zero real wall-clock cost.
type recordFakeSleeper struct {
	slept []time.Duration
}

func (s *recordFakeSleeper) Sleep(d time.Duration) {
	s.slept = append(s.slept, d)
}

var _ websterengine.Sleeper = (*recordFakeSleeper)(nil)

// recordAudit scripts a shuttlefake.Engine's AuditForksIncremental.
// It returns scripted[callCount] on each call, clamped to the last entry once the script is exhausted.
// A test can thus drive a settle-retry sequence, an empty miss followed by a hit or a stable audit across repeated calls, without any real transcript files.
type recordAudit struct {
	scripted  []shuttleengine.ForkAudit
	callCount int
	// sessions records the sessionID of every call, so a test can assert which session the audit was keyed on:
	// the bracket-opening session, never blindly the current Master session.
	sessions []string
	// auditErr, when non-nil, is returned by every call instead of the script: the missing-transcript failure double for the cross-machine resume path.
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

// recordFixture is a fully-wired set of RecordBatch dependencies.
// A temp directory holds the base and in-scope work files as WorktreeRoot over a fakeGit with one base commit plus one work commit.
// A literal one-batch execution-batch list carries an already-open BatchState, the begin-batch record RecordBatch's bracket-discipline check requires.
// A scripted fake engine and a recording Sleeper complete it.
// HeadSHA is the fakeGit's current HEAD, the work commit: every valid report fixture must self-report exactly this SHA, since RecordBatch cross-checks report.HeadSHA against the worktree's HEAD.
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

// newRecordFixture builds a fresh recordFixture whose engine is scripted with scripted, the AuditForksIncremental sequence the caller's own test needs.
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

	// A caller's startSHA stays on head's first-parent line, so the digest's commit read answers over a fake.
	if fake, ok := git.(*fakeGit); ok {
		fake.ensureOnFirstParentLine(startSHA, headSHA)
	}
	fakeIdx, index := indexOver(git)
	cards := []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}
	batches := []batcher.Batch{{Cards: cards}}
	plan := &planparser.Plan{Format: 5, Cards: cards}

	reportsDir := t.TempDir()
	contractDir := t.TempDir()
	// A real (empty) plan directory: RecordBatch re-baselines the plan fingerprint over it after its own BindHandles and DetectDrift rewrites.
	// This is a genuine read rather than an invented path, and no card here declares a handle, so nothing is ever written into it.
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
		Engines: shuttlefake.EngineSet(audit.engine()),
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

// addPendingCard appends a second card to fx's plan, in its own second batch that has no BatchState and is therefore not terminal, referencing uses.
//
// Drift is defined against the remaining plan, and the batch being recorded is not remaining: its work is exactly what the delta reports.
// A drift fixture therefore needs a genuinely pending card to hold the reference.
// Putting it on batch 1's own card tests a card drifting against itself, which is never drift.
func addPendingCard(fx *recordFixture, uses []string) {
	pending := planparser.Card{Number: 2, Slug: "pending", Title: "pending", Intent: "the not-yet-built card", Uses: uses}
	fx.Deps.Plan.Cards = append(fx.Deps.Plan.Cards, pending)
	fx.Deps.Batches = append(fx.Deps.Batches, batcher.Batch{Cards: []planparser.Card{pending}})
}

// writeReport seeds fx's reportsDir with a batch-report YAML file for batch 1 at its plan-format-pinned filename, using content verbatim.
func writeReport(t *testing.T, reportsDir, content string) {
	t.Helper()
	path := filepath.Join(reportsDir, websterengine.ReportFileName(1, "json-flag"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// writeRecordPlanDir writes a minimal, valid on-disk plan directory holding one card whose body is cardBody, returning the directory and its freshly parsed *planparser.Plan.
// It is the record-batch wiring test's own plan-fixture builder, since planglyph's own writePlanFixture is unexported to its package.
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

// validReport returns a well-formed minimal report YAML self-reporting headSHA.
// The caller must pass fx.HeadSHA, the worktree's actual current HEAD, so RecordBatch's head_sha cross-check passes.
func validReport(headSHA string) string {
	return "status: OK\nhead_sha: " + headSHA + "\n"
}
