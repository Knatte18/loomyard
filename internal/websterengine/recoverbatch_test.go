// recoverbatch_test.go exercises RecoverBatch end to end (Tier 1 — see docs/benchmarks/running-tests.md): a temp directory backs WorktreeRoot over a fakeGit for the head, dirty and merge questions, a real *shuttleengine.Runner wired over the shuttlefake Reed/Engine is the Starter, webster's own established fake-starter approach, and a fake Clock replays the whole bounded-wait sequence with no real sleeps, webster's own fakeClock.
// The re-entrancy contract (spawn-once, attach-thereafter, elapsed-across-calls) is this file's test centre, per the batch's own "Batch Tests" note.

package websterengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recoverFakeClock is a package-local, scriptable clock double: Now starts
// at a fixed base and only advances when Sleep is called or a test directly
// mutates Now, so a test controls exactly how much virtual time elapses —
// including simulating the wall-clock gap BETWEEN two separate
// RecoverBatch calls, which a real process boundary would otherwise supply
// for free — without ever blocking for real.
type recoverFakeClock struct {
	now time.Time
}

func (c *recoverFakeClock) Now() time.Time        { return c.now }
func (c *recoverFakeClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

var _ websterengine.Clock = (*recoverFakeClock)(nil)

// recoverFixture is a fully-wired set of RecoverBatch dependencies: a temp
// directory holding base.txt as WorktreeRoot over a fakeGit at one commit, a one-batch plan backed
// by a seeded plan dir and its corresponding execution-batch list, a real
// *shuttleengine.Runner over shuttlefake.Reed/shuttlefake.Engine as the Starter,
// and webster's two roles pre-resolved.
type recoverFixture struct {
	Deps       websterengine.RecoverDeps
	Reed       *shuttlefake.Reed
	Engine     *shuttlefake.Engine
	Git        *fakeGit
	Worktree   string
	ReportsDir string
	// Head returns the worktree's current HEAD commit.
	Head func(t *testing.T) string
}

func newRecoverFixture(t *testing.T) *recoverFixture {
	t.Helper()
	worktree := t.TempDir()
	writeWorktreeFile(t, worktree, "base.txt", "base")
	git := newFakeGit()
	fx := newRecoverFixtureOver(t, worktree, git)
	fx.Git = git
	fx.Head = func(*testing.T) string { return git.head }
	return fx
}

// newRecoverFixtureOver builds the fixture over worktree, answering git questions from git; a nil git means the real repository at worktree.
func newRecoverFixtureOver(t *testing.T, worktree string, git websterengine.Git) *recoverFixture {
	t.Helper()

	_, index := indexOver(git)

	plan := &planparser.Plan{}
	batches := []batcher.Batch{
		{Cards: []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}},
	}

	reed := &shuttlefake.Reed{}
	engine := &shuttlefake.Engine{}
	hubPath := filepath.Dir(worktree)
	// webster's prompts are read from disk at call time now, so the fixture's
	// hub must carry them before RecoverBatch reaches RenderRecoveryPrompt.
	seedHubStencils(t, hubPath)
	shuttleCfg := shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(reed, engine, worktree, worktree, shuttleCfg)

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:   {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleRecovery: {Engine: "claude", Model: "recovery-model", Params: map[string]string{"effort": "high"}},
	}

	reportsDir := t.TempDir()

	// The terminal recovery path refuses a plan that differs from the recorded fingerprint, so the state records this one.
	planDir := seedPlanDir(t)
	websterDir := t.TempDir()
	state := &websterengine.State{Batches: map[int]*websterengine.BatchState{}}
	if err := websterengine.RestampPlanBaseline(state, planDir, websterDir); err != nil {
		t.Fatalf("RestampPlanBaseline() error = %v", err)
	}

	deps := websterengine.RecoverDeps{
		Starter:    runner,
		Plan:       plan,
		Batches:    batches,
		State:      state,
		Roles:      roles,
		Config:     websterengine.Config{SelfFixCap: 2, RecoveryTimeoutMin: 30},
		Engine:     engine,
		Reed:       reed,
		ShuttleCfg: shuttleCfg,
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   websterDir,
			ReportsDir:   reportsDir,
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			SpecsDir:     fabricengine.SpecsDir(hubPath),
			// A real (empty) plan directory: the terminal recovery path now runs the same
			// post-batch mechanical pass record-batch does, which re-baselines the plan
			// fingerprint over this directory. No card in this fixture declares a handle, so
			// nothing is ever written into it.
			PlanDir: planDir,
			Git:     git,
			Index:   index,
		},
	}

	return &recoverFixture{Deps: deps, Reed: reed, Engine: engine, Worktree: worktree, ReportsDir: reportsDir}
}

// writeRecoverReport seeds fx's reportsDir with a batch-report YAML file for
// batch 1 at its plan-format-pinned filename.
func writeRecoverReport(t *testing.T, reportsDir, content string) {
	t.Helper()
	path := filepath.Join(reportsDir, websterengine.ReportFileName(1, "json-flag"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// recoverDriveResult mirrors the composed per-call outcome the CLI verb
// assembles from the three lease-scoped phases, so each test keeps asserting
// one call's whole effect.
type recoverDriveResult struct {
	Digest   *websterengine.Digest
	Running  bool
	Spawned  bool
	ElapsedS int
	Warnings []string
}

// driveRecoverBatch composes RecoverSpawnOrAttach -> RecoverAwait ->
// PersistRecoveryTerminal against deps' in-memory state, exactly the
// sequence webstercli's recover-batch verb drives (minus the lease and
// SaveState/fabric steps, which the CLI owns) — so every re-entrancy and
// classification test below exercises the same composition production runs.
func driveRecoverBatch(deps websterengine.RecoverDeps, batchNumber int, wait time.Duration, clk websterengine.Clock) (*recoverDriveResult, error) {
	bs, spawned, err := websterengine.RecoverSpawnOrAttach(deps, batchNumber, clk)
	if err != nil {
		return nil, err
	}
	result, err := websterengine.RecoverAwait(deps, batchNumber, bs, wait, clk)
	if err != nil {
		return nil, err
	}
	if result.Digest != nil {
		postWarnings, perr := websterengine.PersistRecoveryTerminal(deps, deps.State, batchNumber, result.Digest)
		result.Warnings = append(result.Warnings, postWarnings...)
		if perr != nil {
			return nil, perr
		}
	}
	return &recoverDriveResult{
		Digest:   result.Digest,
		Running:  result.Running,
		Spawned:  spawned,
		ElapsedS: result.ElapsedS,
		Warnings: result.Warnings,
	}, nil
}

// TestRecoverBatch_FirstCallSpawnsArchivesStaleReportAndStopsLiveStrand proves the first call for a
// batch with no live recovery record spawns a fresh recovery strand: a stale report at the batch's
// own report path is archived (renamed with a timestamp suffix, never deleted), a prior recorded
// strand still reported live by the reed is stopped, the fresh BatchState's strand fields are
// recorded, and — with no report landing inside the wait window — the call returns Running with
// Spawned: true.
//
//testtiming:keep pins the whole first-spawn effect — the stale report archived under the injected clock's stamp, the prior live strand stopped and every recorded strand field — which the spawn-decision table only samples one field at a time
func TestRecoverBatch_FirstCallSpawnsArchivesStaleReportAndStopsLiveStrand(t *testing.T) {
	fx := newRecoverFixture(t)

	stalePath := filepath.Join(fx.ReportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(stalePath, []byte("status: FAILED\nhead_sha: deadbeef\n"), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}

	fx.Deps.State.Batches[1] = &websterengine.BatchState{
		Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "orphan-1",
	}
	fx.Reed.Strands = []reedengine.StrandStatus{{GUID: "orphan-1", Live: true}}

	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	result, err := driveRecoverBatch(fx.Deps, 1, 3*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() error = %v; want nil", err)
	}
	if !result.Spawned {
		t.Error("RecoverResult.Spawned = false; want true (first call spawns)")
	}
	if !result.Running {
		t.Errorf("RecoverResult.Running = false; want true (no report landed inside the wait window)")
	}
	if result.Digest != nil {
		t.Errorf("RecoverResult.Digest = %+v; want nil for a running result", result.Digest)
	}

	// The stale report was archived (renamed with a timestamp suffix), never
	// deleted, and the live path is free for the fresh recovery's own report.
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stat(%s) = %v; want the live report path freed (archived away)", stalePath, statErr)
	}
	archived, globErr := filepath.Glob(filepath.Join(fx.ReportsDir, "01-json-flag-*.yaml"))
	if globErr != nil || len(archived) != 1 {
		t.Fatalf("archived report glob = %v, %v; want exactly 1 archive", archived, globErr)
	}
	// The archive timestamp comes from the INJECTED clock (clk starts at
	// time.Unix(0,0)), not the wall clock — F16: recoverSpawn archives with
	// clk.Now so a test clock makes the archive name deterministic.
	if !strings.Contains(archived[0], "19700101T000000Z") {
		t.Errorf("archived report %q; want the injected-clock epoch stamp 19700101T000000Z", archived[0])
	}
	data, err := os.ReadFile(archived[0])
	if err != nil {
		t.Fatalf("read archived report %s: %v", archived[0], err)
	}
	if !strings.Contains(string(data), "status: FAILED") {
		t.Errorf("archived report content = %q; want the prior report preserved verbatim", string(data))
	}

	// The prior orphan's live strand was stopped before the fresh spawn.
	found := false
	for _, guid := range fx.Reed.RemovedGUIDs {
		if guid == "orphan-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("RemoveStrand calls = %v; want the prior live strand %q stopped", fx.Reed.RemovedGUIDs, "orphan-1")
	}

	if len(fx.Reed.AddedSpecs) != 1 || fx.Reed.AddedSpecs[0].Segment != string(segmentcolor.Webster) {
		t.Errorf("AddStrand specs = %+v; want exactly one carrying Segment %q", fx.Reed.AddedSpecs, segmentcolor.Webster)
	}

	// The fresh BatchState's strand fields are recorded.
	bs := fx.Deps.State.Batches[1]
	if bs.Kind != "recovery" {
		t.Errorf("BatchState.Kind = %q; want %q", bs.Kind, "recovery")
	}
	if bs.Terminal {
		t.Error("BatchState.Terminal = true after a running result; want false")
	}
	if bs.StrandGUID == "" || bs.StrandGUID == "orphan-1" {
		t.Errorf("BatchState.StrandGUID = %q; want a freshly minted guid distinct from the stopped orphan", bs.StrandGUID)
	}
	if bs.ShuttleRunDir == "" {
		t.Error("BatchState.ShuttleRunDir is empty; want the resolved run directory")
	}
	if bs.EventsPath == "" {
		t.Error("BatchState.EventsPath is empty; want the resolved events.jsonl path")
	}
	if _, parseErr := time.Parse(time.RFC3339, bs.SpawnedAt); parseErr != nil {
		t.Errorf("BatchState.SpawnedAt = %q: %v; want a valid RFC3339 timestamp", bs.SpawnedAt, parseErr)
	}
	wantHead := fx.Git.head
	if bs.StartSHA != wantHead {
		t.Errorf("BatchState.StartSHA = %q; want the fresh HeadSHA %q", bs.StartSHA, wantHead)
	}

	if fx.Engine.PrepareCalls != 1 {
		t.Errorf("Engine.prepareCalls = %d; want exactly 1", fx.Engine.PrepareCalls)
	}
}

// TestRecoverBatch_DoneReportGuard proves the finished-work guard: a batch whose on-disk report already parses to status: done is refused,
// naming record-batch as the consuming verb and leaving the report in place for it (recover-batch never archives finished work),
// EXCEPT when the batch's persisted state is terminal dead, webster's own dead-orphan late-report case, where the report is archived and the spawn proceeds.
func TestRecoverBatch_DoneReportGuard(t *testing.T) {
	t.Parallel()

	const doneReport = "status: OK\nhead_sha: deadbeef\n"
	cases := []struct {
		name  string
		prior *websterengine.BatchState
		check func(t *testing.T, fx *recoverFixture, result *recoverDriveResult, err error)
	}{
		{
			name: "a done report with no prior record is refused",
			check: func(t *testing.T, fx *recoverFixture, result *recoverDriveResult, err error) {
				if err == nil {
					t.Fatal("RecoverBatch() with a done report = nil error; want the finished-work refusal")
				}
				if !strings.Contains(err.Error(), "record-batch") || !strings.Contains(err.Error(), "`lyx webster record-batch 1`") {
					t.Errorf("error = %q; want it to name `lyx webster record-batch 1` as the consuming verb", err.Error())
				}
				// The done report must be untouched — never archived by a refusal.
				if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, "01-json-flag.yaml")); statErr != nil {
					t.Errorf("stat(done report) = %v; want the report left in place for record-batch", statErr)
				}
				if fx.Engine.PrepareCalls != 0 {
					t.Errorf("Engine.prepareCalls = %d; want 0 (no spawn on refusal)", fx.Engine.PrepareCalls)
				}
			},
		},
		{
			name:  "a done report over a terminal dead prior is archived and a strand spawned",
			prior: &websterengine.BatchState{Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead"},
			check: func(t *testing.T, fx *recoverFixture, result *recoverDriveResult, err error) {
				if err != nil {
					t.Fatalf("RecoverBatch() for a dead batch with a late done report = %v; want the archive-and-respawn path", err)
				}
				if !result.Spawned {
					t.Error("RecoverResult.Spawned = false; want true (dead-orphan late report is archive-never-refuse)")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRecoverFixture(t)
			if err := os.WriteFile(filepath.Join(fx.ReportsDir, "01-json-flag.yaml"), []byte(doneReport), 0o644); err != nil {
				t.Fatalf("seed done report: %v", err)
			}
			if tc.prior != nil {
				fx.Deps.State.Batches[1] = tc.prior
			}

			result, err := driveRecoverBatch(fx.Deps, 1, time.Second, &recoverFakeClock{now: time.Unix(0, 0)})
			tc.check(t, fx, result, err)
		})
	}
}

// recoverAtReportHead spawns the recovery strand, then seeds a done report at the worktree's current HEAD and returns that head.
func recoverAtReportHead(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) string {
	t.Helper()
	first, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
	}
	if !first.Running {
		t.Fatalf("first call = %+v; want Running=true", first)
	}
	head := fx.Head(t)
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
	return head
}

// TestRecoverBatch_SecondCall asserts how a re-entrant second call classifies a recovery strand the first call spawned:
// a landed report attaches (never re-spawns) and persists the done digest, a report disagreeing with the worktree or a HEAD moved by anything but a clean parent merge is refused and leaves the batch non-terminal,
// the recovery timeout is measured from the recorded SpawnedAt across calls, and the default wait budget ends a silent strand dead inside one call but returns at a report that landed.
func TestRecoverBatch_SecondCall(t *testing.T) {
	t.Parallel()

	// secondCall is what a case's afterFirst reports about the second call it prepared.
	type secondCall struct {
		// head is the head_sha the seeded report names.
		head string
		// refusal lists what the second call's error contains; nil means it succeeds.
		refusal []string
	}
	// attempt is one second call's outcome, with the facts recorded before it ran.
	type attempt struct {
		second      *recoverDriveResult
		runDir      string
		strandGUID  string
		clockBefore time.Time
		head        string
	}
	seedReportAtHead := func(t *testing.T, fx *recoverFixture) string {
		head := fx.Head(t)
		writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
		return head
	}

	cases := []struct {
		name string
		// config edits the deps before the first call.
		config func(fx *recoverFixture)
		// singleCall drives one call only, with the default wait budget.
		singleCall bool
		// afterFirst prepares the second call, between the two separate CLI invocations.
		afterFirst func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall
		// useBudget gives the second call the default wait budget instead of a short wait.
		useBudget bool
		check     func(t *testing.T, fx *recoverFixture, a attempt)
	}{
		{
			// The re-fork's report self-reports the worktree's real HEAD (the head_sha
			// cross-check refuses a report whose SHA disagrees with the worktree it left behind).
			name: "a landed report attaches and persists the done digest",
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				return secondCall{head: seedReportAtHead(t, fx)}
			},
			check: func(t *testing.T, fx *recoverFixture, a attempt) {
				if a.second.Spawned {
					t.Error("second call Spawned = true; want false (ATTACH, not a re-spawn)")
				}
				if a.second.Running {
					t.Error("second call Running = true; want false (terminal once the report landed)")
				}
				if a.second.Digest == nil || a.second.Digest.Status != websterengine.DigestStatusDone {
					t.Fatalf("second call Digest = %+v; want a done digest", a.second.Digest)
				}
				if len(a.second.Warnings) != 0 {
					t.Errorf("second call Warnings = %v; want none", a.second.Warnings)
				}
				if fx.Engine.PrepareCalls != 1 {
					t.Errorf("Engine.prepareCalls = %d; want exactly 1 (no second spawn on ATTACH)", fx.Engine.PrepareCalls)
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
				// A recovery-completed batch must land in the accumulated CardSHAs trail
				// exactly like a fork batch, or the integration-suite bisect searches a
				// gapped trail and blames the wrong card (crucible round fable-r1's F3).
				if len(bs.CardSHAs) != 1 || bs.CardSHAs[0] != a.head {
					t.Errorf("BatchState.CardSHAs = %v; want [%s] persisted at recovery terminal", bs.CardSHAs, a.head)
				}
				if fx.Deps.State.CurrentBatch != 0 {
					t.Errorf("State.CurrentBatch = %d; want 0 (cleared)", fx.Deps.State.CurrentBatch)
				}

				// done-substrate release: strand removed, run dir removed.
				if !slices.Contains(fx.Reed.RemovedGUIDs, a.strandGUID) {
					t.Errorf("RemoveStrand calls = %v; want the done strand %q removed", fx.Reed.RemovedGUIDs, a.strandGUID)
				}
				if _, statErr := os.Stat(a.runDir); !os.IsNotExist(statErr) {
					t.Errorf("stat(%s) = %v; want the done run dir removed", a.runDir, statErr)
				}
			},
		},
		{
			// The recovery path applies RecordBatch's own head_sha cross-check: a report that
			// disagrees with the worktree is refused loud rather than persisted into the digest
			// and the bisect trail (crucible round fable-r1's F4).
			name: "a report whose head_sha disagrees with the worktree is a hard error",
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: main\n")
				return secondCall{refusal: []string{"does not match the worktree's actual HEAD"}}
			},
		},
		{
			name: "an abbreviated head_sha records the full SHA",
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				head := fx.Head(t)
				writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head[:9]+"\n")
				return secondCall{head: head}
			},
			check: func(t *testing.T, fx *recoverFixture, a attempt) {
				if a.second.Digest == nil || a.second.Digest.HeadSHA != a.head {
					t.Errorf("Digest = %+v; want HeadSHA %q", a.second.Digest, a.head)
				}
				if got := fx.Deps.State.Batches[1].CardSHAs; len(got) != 1 || got[0] != a.head {
					t.Errorf("CardSHAs = %v; want [%s]", got, a.head)
				}
			},
		},
		{
			name: "an abbreviated head_sha naming no commit is refused",
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: abcdef123\n")
				return secondCall{refusal: []string{"report head_sha unresolved", "names no commit", "`git rev-parse HEAD`"}}
			},
		},
		{
			name: "a plain commit on top of the report's head is refused with both SHAs",
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				head := seedReportAtHead(t, fx)
				newHead := fx.Git.commit()
				return secondCall{head: head, refusal: []string{head, newHead, "only merge commits"}}
			},
		},
		{
			name: "a conflicting merge left in progress is refused with the continue/abort pointer",
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				head := seedReportAtHead(t, fx)
				fx.Git.merging = true
				return secondCall{head: head, refusal: []string{"merge --continue", "merge --abort"}}
			},
			// A refusing call returns no result,
			// so the check reads only the reed double.
			check: func(t *testing.T, fx *recoverFixture, a attempt) {
				if !slices.Contains(fx.Reed.RemovedGUIDs, a.strandGUID) {
					t.Errorf("RemoveStrand calls = %v; want the recovery strand %q removed although the call refused", fx.Reed.RemovedGUIDs, a.strandGUID)
				}
			},
		},
		{
			// recover-batch follows record-batch's merge-only head rule: a --no-ff parent merge
			// landing after the report's head is accepted and the moved-HEAD notice names the merge.
			name: "a parent merge after the report's head is accepted",
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				head := seedReportAtHead(t, fx)
				fx.Deps.ParentBranch = func() (string, error) { return "parent1", nil }
				merge, _ := fx.Git.merge("")
				return secondCall{head: head + " " + merge}
			},
			check: func(t *testing.T, fx *recoverFixture, a attempt) {
				head, merge, _ := strings.Cut(a.head, " ")
				if a.second.Digest == nil || a.second.Digest.Status != websterengine.DigestStatusDone {
					t.Fatalf("Digest = %+v; want a done digest", a.second.Digest)
				}
				bs := fx.Deps.State.Batches[1]
				if !bs.Terminal {
					t.Error("BatchState.Terminal = false; want true")
				}
				if len(bs.CardSHAs) != 1 || bs.CardSHAs[0] != head {
					t.Errorf("BatchState.CardSHAs = %v; want [%s]", bs.CardSHAs, head)
				}
				if !warningsContain(a.second.Warnings, "only merge commits", merge) {
					t.Errorf("Warnings = %v; want the moved-HEAD notice naming merge %s", a.second.Warnings, merge)
				}
			},
		},
		{
			// RecoveryTimeoutMin is measured from the recorded SpawnedAt ACROSS re-entrant calls:
			// virtual time advanced past the timeout between two calls classifies dead/timeout on the
			// second call though neither call's own wait budget crosses it, and the dead
			// classification removes the strand but keeps the run directory (diagnosis material).
			name:   "the recovery timeout is measured across calls and a dead strand is removed with its run dir kept",
			config: func(fx *recoverFixture) { fx.Deps.Config.RecoveryTimeoutMin = 1 },
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				// Two minutes pass between the two separate CLI invocations, with no report ever landing.
				clk.now = clk.now.Add(2 * time.Minute)
				return secondCall{}
			},
			check: func(t *testing.T, fx *recoverFixture, a attempt) {
				if a.second.Running {
					t.Error("second call Running = true; want false (terminal dead/timeout)")
				}
				if a.second.Digest == nil || a.second.Digest.Status != websterengine.DigestStatusDead {
					t.Fatalf("second call Digest = %+v; want a dead digest", a.second.Digest)
				}
				if a.second.Digest.DeadReason != websterengine.DeadReasonTimeout {
					t.Errorf("second call Digest.DeadReason = %q; want %q", a.second.Digest.DeadReason, websterengine.DeadReasonTimeout)
				}
				if a.second.ElapsedS < 120 {
					t.Errorf("second call ElapsedS = %d; want >= 120 (measured since the original spawn)", a.second.ElapsedS)
				}
				if !slices.Contains(fx.Reed.RemovedGUIDs, a.strandGUID) {
					t.Errorf("RemoveStrand calls = %v; want the dead-classified strand %q removed", fx.Reed.RemovedGUIDs, a.strandGUID)
				}
				if _, statErr := os.Stat(a.runDir); statErr != nil {
					t.Errorf("stat(%s) = %v; want the dead-classified run dir kept", a.runDir, statErr)
				}
			},
		},
		{
			// The default wait budget outlasts recovery_timeout_min, so a strand that never reports
			// classifies dead/timeout inside one call and never returns a running snapshot.
			name:       "the default wait budget ends a silent strand dead inside one call",
			config:     func(fx *recoverFixture) { fx.Deps.Config.RecoveryTimeoutMin = 1 },
			singleCall: true,
			check: func(t *testing.T, fx *recoverFixture, a attempt) {
				if budget := websterengine.RecoveryWaitBudget(fx.Deps.Config); budget <= time.Duration(fx.Deps.Config.RecoveryTimeoutMin)*time.Minute {
					t.Fatalf("RecoveryWaitBudget() = %v; want more than recovery_timeout_min", budget)
				}
				if a.second.Running || a.second.Digest == nil {
					t.Fatalf("result = %+v; want a terminal digest, never a running snapshot", a.second)
				}
				if a.second.Digest.Status != websterengine.DigestStatusDead || a.second.Digest.DeadReason != websterengine.DeadReasonTimeout {
					t.Errorf("Digest = %+v; want dead/%s", a.second.Digest, websterengine.DeadReasonTimeout)
				}
			},
		},
		{
			name: "a report landing before the timeout ends the call without waiting out the budget",
			afterFirst: func(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) secondCall {
				return secondCall{head: seedReportAtHead(t, fx)}
			},
			useBudget: true,
			check: func(t *testing.T, fx *recoverFixture, a attempt) {
				if a.second.Running || a.second.Digest == nil || a.second.Digest.Status != websterengine.DigestStatusDone {
					t.Fatalf("second call = %+v; want a done digest", a.second)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRecoverFixture(t)
			if tc.config != nil {
				tc.config(fx)
			}
			clk := &recoverFakeClock{now: time.Unix(0, 0)}

			var a attempt
			var prepared secondCall
			var wait time.Duration
			if tc.singleCall {
				wait = websterengine.RecoveryWaitBudget(fx.Deps.Config)
			} else {
				first, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
				if err != nil {
					t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
				}
				if !first.Spawned || !first.Running {
					t.Fatalf("first call = %+v; want Spawned=true Running=true", first)
				}
				a.runDir = fx.Deps.State.Batches[1].ShuttleRunDir
				a.strandGUID = fx.Deps.State.Batches[1].StrandGUID
				prepared = tc.afterFirst(t, fx, clk)
				a.head = prepared.head
				wait = 2 * time.Second
				if tc.useBudget {
					wait = websterengine.RecoveryWaitBudget(fx.Deps.Config)
				}
			}
			a.clockBefore = clk.now

			second, err := driveRecoverBatch(fx.Deps, 1, wait, clk)
			if prepared.refusal != nil {
				if err == nil {
					t.Fatal("RecoverBatch() second call = nil error; want a refusal")
				}
				for _, want := range prepared.refusal {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err.Error(), want)
					}
				}
				// Nothing was persisted terminal: the batch record stays non-terminal for
				// a corrected report or an operator to resolve.
				if bs := fx.Deps.State.Batches[1]; bs.Terminal {
					t.Errorf("BatchState.Terminal = true; want false after a refusal")
				}
				if tc.check != nil {
					tc.check(t, fx, a)
				}
				return
			}
			if err != nil {
				t.Fatalf("RecoverBatch() second call error = %v; want nil", err)
			}
			a.second = second
			if tc.useBudget && clk.now.Sub(a.clockBefore) >= time.Minute {
				t.Errorf("call advanced the clock by %v; want it to return at the report, not wait out the budget", clk.now.Sub(a.clockBefore))
			}
			tc.check(t, fx, a)
		})
	}
}

// TestRecoverBatch_TerminalRunsTheSamePostBatchChecksAsRecordBatch is the regression test for the
// round-4 review's R4-03. A recovery batch reaching status done used to be marked terminal on the
// recovery strand's self-reported status plus the head-SHA cross-check alone — no done-checks, no
// handle binding, no scope guard, no drift detection and no plan-staleness re-baseline. Master's own
// failure ladder treats a terminal recovery digest as "move on to the next batch", so a card
// recovered that way never bound its plan: handles and every later card kept referencing an unbound
// handle for the rest of the plan's life.
//
// Here the recovery reports done over a Create card whose target never appeared in the tree.
// The mechanical pass must refuse it and take the batch terminal failed with its report archived, exactly as record-batch does, so the next recover-batch spawns a fresh strand.
func TestRecoverBatch_TerminalRunsTheSamePostBatchChecksAsRecordBatch(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	// The batch's card declares a Create target that the recovery never lands.
	card := planparser.Card{
		Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag",
		Targets:      []string{"internal/never/there.go#"},
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"internal/never/there.go#"}}},
	}
	fx.Deps.Plan.Cards = []planparser.Card{card}
	fx.Deps.Batches[0].Cards = []planparser.Card{card}

	first, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
	}
	if !first.Spawned || !first.Running {
		t.Fatalf("first call = %+v; want Spawned=true Running=true", first)
	}

	realHead := fx.Git.head
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+realHead+"\n")

	_, err = driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecoverBatch() second call error = %v; want errors.Is(err, ErrBatchFailed) — a recovery reporting done over an unlanded Create target must be failed", err)
	}
	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed {
		t.Errorf("BatchState = terminal %v status %q; want terminal failed", bs.Terminal, bs.Status)
	}
	if bs.Digest == nil || len(bs.Digest.Reasons) == 0 {
		t.Errorf("failed digest = %+v; want reasons naming the done-check finding", bs.Digest)
	}
	if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
		t.Errorf("stat(live report) = %v; want the report archived away", statErr)
	}
	if fx.Deps.State.CurrentBatch != 0 {
		t.Errorf("State.CurrentBatch = %d; want 0", fx.Deps.State.CurrentBatch)
	}

	// The next recover-batch spawns a fresh strand rather than re-attaching to the finished one.
	again, err := driveRecoverBatch(fx.Deps, 1, 1*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() third call error = %v; want nil", err)
	}
	if !again.Spawned {
		t.Errorf("third call = %+v; want Spawned=true after a failed recovery", again)
	}
}

// TestRecoverBatch_AmendedCards walks one batch's amendment through its recoveries.
// A spawn renders the amended card into the prompt and carries the entry as rendered, so the rendering recovery's own stuck record is not forced failed.
// A re-edit during a recovery forces that recovery's done report failed with card_amended.
// A stuck recovery keeps the entry for the next spawn, and a done recovery clears it.
func TestRecoverBatch_AmendedCards(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	const amendedLine = "card 01-json-flag was amended after the previous attempt began"

	spawnRendering := func(step string) {
		t.Helper()
		prompts := fx.Engine.PrepareCalls
		res, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
		if err != nil || !res.Spawned || !res.Running {
			t.Fatalf("%s: spawn = %+v, %v; want a running spawned recovery", step, res, err)
		}
		if fx.Engine.PrepareCalls != prompts+1 || !strings.Contains(fx.Engine.LastPrompt, amendedLine) {
			t.Fatalf("%s: prompt does not carry the amended card instruction %q", step, amendedLine)
		}
		if got := fx.Deps.State.Batches[1].AmendedCards; len(got) != 1 || got[0].Card != "01-json-flag" || !got[0].Rendered {
			t.Fatalf("%s: AmendedCards = %+v; want the entry carried as rendered", step, got)
		}
	}

	// A fork attempt failed on a different reason while its card was amended, so the failure carries both.
	prior := failedRecord("an earlier reason")
	prior.AmendedCards = []websterengine.AmendedCard{{Card: "01-json-flag"}}
	fx.Deps.State.Batches[1] = prior

	spawnRendering("first spawn")
	head := fx.Git.head

	// An amendment accepted while that recovery runs forces its done report failed, whatever the report says.
	fx.Deps.State.Batches[1].AmendedCards[0].Rendered = false
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
	_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	var failed *websterengine.BatchFailedError
	if !errors.As(err, &failed) || !failed.CardAmended {
		t.Fatalf("done report over a re-edited card: err = %v; want a BatchFailedError with CardAmended", err)
	}
	if bs := fx.Deps.State.Batches[1]; !bs.Terminal || bs.Status != websterengine.DigestStatusFailed || len(bs.AmendedCards) != 1 || bs.AmendedCards[0].Rendered {
		t.Fatalf("record after the forced failure = %+v; want terminal failed with the entry still unrendered", bs)
	}

	// The next spawn renders it again, and the recovery ending stuck keeps the entry.
	spawnRendering("second spawn")
	writeRecoverReport(t, fx.ReportsDir, "status: FAILED\nhead_sha: "+head+"\n")
	if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
		t.Fatalf("stuck recovery over a rendered amendment: err = %v; want it recorded, not forced failed", err)
	}
	if bs := fx.Deps.State.Batches[1]; bs.Status != websterengine.DigestStatusStuck || len(bs.AmendedCards) != 1 {
		t.Fatalf("record after the stuck recovery = status %q, AmendedCards %+v; want stuck with the entry kept", bs.Status, bs.AmendedCards)
	}

	// A stuck prior is no failed digest, yet its kept entry is rendered into the next spawn, and a done recovery clears it.
	spawnRendering("third spawn")
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
	if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
		t.Fatalf("done recovery over a rendered amendment: err = %v; want it recorded done", err)
	}
	if bs := fx.Deps.State.Batches[1]; bs.Status != websterengine.DigestStatusDone || len(bs.AmendedCards) != 0 {
		t.Fatalf("record after the done recovery = status %q, AmendedCards %+v; want done with the entries cleared", bs.Status, bs.AmendedCards)
	}
}

// failedRecord builds the record RecordBatch leaves behind for a batch it failed on its merits:
// terminal, status failed, reasons ending with the suspect paths.
func failedRecord(reasons ...string) *websterengine.BatchState {
	return &websterengine.BatchState{
		Slug: "json-flag", Kind: "fork", Terminal: true, Status: websterengine.DigestStatusFailed,
		Digest: &websterengine.Digest{Batch: "01-json-flag", Status: websterengine.DigestStatusFailed, Reasons: reasons},
	}
}

// erroringStarter is a websterengine.Starter double whose Start always fails wrapping shuttleengine.ErrNotStarted — the not-ready-start error shuttle now returns from Start itself (per the shuttle-start-guarantees-readiness discussion), which recoverFixture's real *shuttleengine.Runner over shuttlefake.Engine/shuttlefake.Reed never reaches on its own, since shuttlefake.Engine.Startup reports StartupReady by default.
type erroringStarter struct{}

func (erroringStarter) Start(spec shuttleengine.Spec) (*shuttleengine.Run, error) {
	return nil, fmt.Errorf("webster test: recovery strand never became ready: %w", shuttleengine.ErrNotStarted)
}

var _ websterengine.Starter = erroringStarter{}

// TestRecoverSpawnOrAttach asserts RecoverSpawnOrAttach's spawn-or-attach decision for the state a batch is in:
// no record, a terminal prior or a failed batch spawns fresh, with the recorded card set, the original bracket's start commit, the failure digest and the execution predecessor's digest in the prompt, and any late or malformed report archived;
// a recorded non-terminal recovery attaches; findings recovery cannot check are refused toward the reset-to-start route; and a failed or not-ready start surfaces without recording a strand.
//
//testtiming:keep pins the spawn-or-attach decision for every batch state, the prompt's card set, start commit and digests, the archived late report and the refusals; each covering test reaches one state
func TestRecoverSpawnOrAttach(t *testing.T) {
	t.Parallel()

	reorderedBatches := func(fx *recoverFixture) {
		// Batch 2 (list-tests) runs before batch 1 (json-flag) in execution order, even though batch 1's declared number is lower.
		fx.Deps.Batches = []batcher.Batch{
			{Cards: []planparser.Card{{Number: 2, Slug: "list-tests", Title: "list-tests", Intent: "list the tests"}}},
			{Cards: []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}},
		}
	}
	requireSpawned := func(t *testing.T, spawned bool, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("RecoverSpawnOrAttach() error = %v; want nil", err)
		}
		if !spawned {
			t.Fatal("RecoverSpawnOrAttach() spawned = false; want a fresh recovery strand")
		}
	}
	failedBatchPrompt := func(reasons ...string) (func(fx *recoverFixture), func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error)) {
		setup := func(fx *recoverFixture) { fx.Deps.State.Batches[1] = failedRecord(reasons...) }
		check := func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
			requireSpawned(t, spawned, err)
			for _, reason := range reasons {
				if !strings.Contains(fx.Engine.LastPrompt, reason) {
					t.Errorf("recovery prompt lacks reason %q", reason)
				}
			}
		}
		return setup, check
	}
	forkContractSetup, forkContractCheck := failedBatchPrompt("fork wrote the report contract file", "suspect path: _lyx/webster/state.json")
	fabricSetup, fabricCheck := failedBatchPrompt("fabric-reference in parent transcript", "card verify failed: go test ./x", "suspect path: internal/x/x.go")
	doneCheckSetup, doneCheckCheck := failedBatchPrompt("Create target internal/never/there.go# does not resolve")
	uncheckableRefusal := func(uncheckable ...string) (func(fx *recoverFixture) *websterengine.BatchState, func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error)) {
		var rec *websterengine.BatchState
		setup := func(fx *recoverFixture) *websterengine.BatchState {
			rec = failedRecord("correctness finding")
			rec.Uncheckable = uncheckable
			fx.Deps.State.Batches[1] = rec
			return rec
		}
		check := func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
			if !errors.Is(err, websterengine.ErrRecoveryNeedsFresh) {
				t.Fatalf("RecoverSpawnOrAttach() error = %v; want ErrRecoveryNeedsFresh", err)
			}
			if spawned {
				t.Error("spawned = true; want no strand")
			}
			for _, want := range append([]string{"1) lyx webster reset --to start; 2) lyx webster run", "batch 01"}, uncheckable...) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
			if fx.Deps.State.Batches[1] != rec || !rec.Terminal || rec.Status != websterengine.DigestStatusFailed || rec.StrandGUID != "" {
				t.Errorf("record changed: %+v", rec)
			}
			if got := fx.Engine.LastPrompt; got != "" {
				t.Errorf("a recovery prompt was rendered: %q", got)
			}
		}
		return setup, check
	}
	fabricRefusalSetup, fabricRefusalBase := uncheckableRefusal("fabric-reference: Bash command references the fabric")
	pauseRefusalSetup, pauseRefusalBase := uncheckableRefusal(".lyx/webster/pause")
	// Only a refusal over pathless fabric references names the accept-audit --batch route.
	const acceptBatchStep = `"lyx webster accept-audit --batch 1" then "lyx webster recover-batch 1"`
	// The route names both evidence: HEAD at the start commit, and a committed batch whose recorded commands are read-only.
	const startRouteText = "when the worktree is clean and HEAD is the batch's start commit (\"lyx webster reset --to batch-start --batch 01\" moves it there if the batch committed)"
	const committedRouteText = "or the batch's commits are kept and every recorded command is read-only"
	fabricRefusalCheck := func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
		fabricRefusalBase(t, fx, bs, spawned, err)
		if err == nil {
			return
		}
		for _, want := range []string{acceptBatchStep, startRouteText, committedRouteText} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q lacks %q", err, want)
			}
		}
	}
	pauseRefusalCheck := func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
		pauseRefusalBase(t, fx, bs, spawned, err)
		if err != nil && strings.Contains(err.Error(), "accept-audit --batch") {
			t.Errorf("error %q names accept-audit --batch for a finding that is not a fabric reference", err)
		}
	}

	cases := []struct {
		name  string
		batch int
		// setup seeds the state the decision is made over; nil leaves the batch unrecorded.
		setup func(fx *recoverFixture)
		check func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error)
	}{
		{
			name: "no recorded batch state spawns fresh and records the card set",
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				requireSpawned(t, spawned, err)
				got := fx.Deps.State.Batches[1].Cards
				if want := []string{"01-json-flag"}; !slices.Equal(got, want) {
					t.Errorf("recovery BatchState.Cards = %v; want %v", got, want)
				}
				if hashes := fx.Deps.State.Batches[1].CardHashes; len(hashes) != 1 || hashes["01-json-flag"] == "" {
					t.Errorf("recovery BatchState.CardHashes = %v; want one hash for 01-json-flag", hashes)
				}
			},
		},
		{
			name: "a terminal prior recovery attempt spawns fresh",
			setup: func(fx *recoverFixture) {
				fx.Deps.State.Batches[1] = &websterengine.BatchState{Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "prior-dead-1"}
				fx.Reed.Strands = []reedengine.StrandStatus{{GUID: "prior-dead-1", Live: true}}
			},
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				requireSpawned(t, spawned, err)
				if fx.Engine.PrepareCalls != 1 {
					t.Errorf("Engine.prepareCalls = %d; want 1", fx.Engine.PrepareCalls)
				}
			},
		},
		{
			name: "a non-terminal recorded recovery attaches",
			setup: func(fx *recoverFixture) {
				fx.Deps.State.Batches[1] = &websterengine.BatchState{
					Slug: "json-flag", Kind: "recovery", Terminal: false, StrandGUID: "still-live-1",
					SpawnedAt: time.Unix(0, 0).UTC().Format(time.RFC3339),
				}
				fx.Reed.Strands = []reedengine.StrandStatus{{GUID: "still-live-1", Live: true}}
			},
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				if err != nil {
					t.Fatalf("RecoverSpawnOrAttach() error = %v; want nil", err)
				}
				if spawned || fx.Engine.PrepareCalls != 0 {
					t.Errorf("spawned = %v, prepareCalls = %d; want an attach with no spawn", spawned, fx.Engine.PrepareCalls)
				}
			},
		},
		{name: "a failed batch on a fork contract write spawns with the failure digest", setup: forkContractSetup, check: forkContractCheck},
		{name: "a failed batch on a fabric reference with a failing verify spawns with the failure digest", setup: fabricSetup, check: fabricCheck},
		{name: "a failed batch on a done-check finding spawns with the failure digest", setup: doneCheckSetup, check: doneCheckCheck},
		{
			name: "a delete an unbegun later card still references is refused before spawning toward the plan edit",
			setup: func(fx *recoverFixture) {
				fx.Deps.Batches = deleteReferencedBatches(t, fx.Worktree, true)
				fx.Deps.State.Batches[1] = failedRecord("delete-not-done/1-json-flag[blocking]: Delete target \"internal/foo#Gone\" still resolves found")
			},
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				if !errors.Is(err, websterengine.ErrRecoveryDeleteReferenced) {
					t.Fatalf("RecoverSpawnOrAttach() error = %v; want ErrRecoveryDeleteReferenced", err)
				}
				if spawned || fx.Engine.PrepareCalls != 0 || fx.Engine.LastPrompt != "" {
					t.Errorf("spawned = %v, prepareCalls = %d, prompt %q; want no strand started", spawned, fx.Engine.PrepareCalls, fx.Engine.LastPrompt)
				}
				for _, want := range []string{"batch 01", "2-later", "internal/foo/user.go:4", "way forward: move the delete to a card after", "lyx webster rebaseline --card NN", "lyx webster recover-batch 01"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q lacks %q", err, want)
					}
				}
			},
		},
		{
			name: "the same failed batch spawns once the later card no longer references the delete",
			setup: func(fx *recoverFixture) {
				fx.Deps.Batches = deleteReferencedBatches(t, fx.Worktree, false)
				fx.Deps.State.Batches[1] = failedRecord("delete-not-done/1-json-flag[blocking]: Delete target \"internal/foo#Gone\" still resolves found")
			},
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				requireSpawned(t, spawned, err)
			},
		},
		{
			name: "a failed batch on a fabric reference recovery cannot check is refused toward the reset-to-start route",
			setup: func(fx *recoverFixture) {
				fabricRefusalSetup(fx)
			},
			check: fabricRefusalCheck,
		},
		{
			name: "a failed batch on the scratch pause flag recovery cannot check is refused toward the reset-to-start route",
			setup: func(fx *recoverFixture) {
				pauseRefusalSetup(fx)
			},
			check: pauseRefusalCheck,
		},
		{
			// An OK report a still-running fork writes after the batch failed is archived rather than refused, so no refusal ring re-forms.
			name: "a failed batch archives a late OK report instead of refusing it",
			setup: func(fx *recoverFixture) {
				fx.Deps.State.Batches[1] = failedRecord("fork wrote the report contract file")
				writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: abc\n")
			},
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				requireSpawned(t, spawned, err)
				if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
					t.Errorf("stat(live report) = %v; want the late report archived away", statErr)
				}
				archived, _ := filepath.Glob(filepath.Join(fx.ReportsDir, "01-json-flag-*.yaml"))
				if len(archived) != 1 {
					t.Errorf("archived reports = %v; want exactly 1", archived)
				}
			},
		},
		{
			// record-batch names recover-batch as the way forward for a malformed report: it is archived and a recovery strand spawned.
			name: "a malformed report is archived and a strand spawned",
			setup: func(fx *recoverFixture) {
				writeRecoverReport(t, fx.ReportsDir, "status: bogus\nhead_sha: deadbeef\n")
			},
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				requireSpawned(t, spawned, err)
				if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, "01-json-flag.yaml")); !os.IsNotExist(statErr) {
					t.Errorf("stat(malformed report) = %v; want it archived out of the report path", statErr)
				}
			},
		},
		{
			// A recovery record carries the ORIGINAL bracket's base commit rather than re-capturing
			// the head at recovery-spawn time: a fork frequently commits part of its work before
			// getting stuck, and a start SHA captured at recovery time excludes exactly that part.
			name: "the recovery record inherits the stuck fork's start commit",
			setup: func(fx *recoverFixture) {
				fx.Deps.State.Batches[1] = &websterengine.BatchState{Slug: "json-flag", StartSHA: fx.Git.head, Kind: "fork"}
				// The stuck fork committed part of its work before reporting stuck.
				writeWorktreeFile(t, fx.Worktree, "internal/partial/impl.go", "package partial\n")
				fx.Git.commit()
			},
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				if err != nil {
					t.Fatalf("RecoverSpawnOrAttach() error = %v; want nil", err)
				}
				got := fx.Deps.State.Batches[1]
				if got.Kind != "recovery" {
					t.Fatalf("BatchState.Kind = %q; want %q", got.Kind, "recovery")
				}
				// setup made exactly one commit on top of the stuck fork's start commit.
				if want := fx.Git.parents[fx.Git.head][0]; got.StartSHA != want {
					t.Errorf("recovery BatchState.StartSHA = %q; want the stuck fork's own %q — the post-batch delta must span the whole bracket, not just the recovery's own share of it", got.StartSHA, want)
				}
			},
		},
		{
			name: "the prompt carries the execution predecessor's digest, not the batch numbered one lower",
			setup: func(fx *recoverFixture) {
				reorderedBatches(fx)
				fx.Deps.State.Batches[2] = &websterengine.BatchState{
					Slug:     "list-tests",
					Terminal: true,
					Status:   "done",
					Digest:   &websterengine.Digest{Batch: "02-list-tests", Status: websterengine.DigestStatusDone, HeadSHA: "cafef00d"},
				}
			},
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				requireSpawned(t, spawned, err)
				for _, want := range []string{"02-list-tests", "head_sha=cafef00d"} {
					if !strings.Contains(fx.Engine.LastPrompt, want) {
						t.Errorf("recovery prompt does not contain %q; got:\n%s", want, fx.Engine.LastPrompt)
					}
				}
			},
		},
		{
			name:  "the batch sitting first renders the no-previous-digest sentinel regardless of its number",
			batch: 2,
			setup: reorderedBatches,
			check: func(t *testing.T, fx *recoverFixture, bs *websterengine.BatchState, spawned bool, err error) {
				requireSpawned(t, spawned, err)
				if !strings.Contains(fx.Engine.LastPrompt, "none (first batch)") {
					t.Errorf("recovery prompt does not contain the first-batch sentinel; got:\n%s", fx.Engine.LastPrompt)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRecoverFixture(t)
			if tc.setup != nil {
				tc.setup(fx)
			}
			batch := tc.batch
			if batch == 0 {
				batch = 1
			}

			bs, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, batch, &recoverFakeClock{now: time.Unix(0, 0)})
			tc.check(t, fx, bs, spawned, err)
		})
	}
}

// TestRecoverSpawnOrAttach_StartFailures asserts a transient failure before the recovery strand is recorded refuses with the transient re-run as the way forward, records no batch state, and spawns no strand, after which re-running the verb once the failure clears spawns it.
// The failures are a not-ready start (shuttle's Start returning ErrNotStarted after tearing its own strand down, a strand that must never be persisted as this batch's recovery record) and, for the recovery prompt's uncommitted paths, a git status that cannot list them or an audit of the run's sessions that cannot tell which of them the run wrote.
func TestRecoverSpawnOrAttach_StartFailures(t *testing.T) {
	t.Parallel()

	statusErr := errors.New("git status failed")
	auditErr := errors.New("transcript unreadable")
	tests := []struct {
		name    string
		fail    func(fx *recoverFixture) (restore func())
		wantErr error
	}{
		{
			name: "a not-ready start",
			fail: func(fx *recoverFixture) func() {
				realStarter := fx.Deps.Starter
				fx.Deps.Starter = erroringStarter{}
				return func() { fx.Deps.Starter = realStarter }
			},
			wantErr: shuttleengine.ErrNotStarted,
		},
		{
			name: "an unreadable uncommitted-path listing",
			fail: func(fx *recoverFixture) func() {
				fx.Git.dirtyPathsErr = statusErr
				return func() { fx.Git.dirtyPathsErr = nil }
			},
			wantErr: statusErr,
		},
		{
			name: "a failed audit of the run's writes to the uncommitted paths",
			fail: func(fx *recoverFixture) func() {
				fx.Git.dirtyPaths = []string{"base.txt"}
				fx.Deps.State.MasterSessionID = "s1"
				fx.Engine.AuditErr = auditErr
				return func() { fx.Engine.AuditErr = nil }
			},
			wantErr: auditErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newRecoverFixture(t)
			restore := tt.fail(fx)
			clk := &recoverFakeClock{now: time.Unix(0, 0)}

			bs, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("RecoverSpawnOrAttach() error = %v; want it to wrap %v", err, tt.wantErr)
			}
			if err == nil || !strings.HasSuffix(err.Error(), "way forward: transient, re-run `lyx webster recover-batch 1`") {
				t.Fatalf("RecoverSpawnOrAttach() error = %v; want the transient re-run way forward", err)
			}
			if spawned {
				t.Error("RecoverSpawnOrAttach() spawned = true; want false")
			}
			if bs != nil {
				t.Errorf("RecoverSpawnOrAttach() BatchState = %+v; want nil", bs)
			}
			if fx.Deps.State.Batches[1] != nil {
				t.Errorf("State.Batches[1] = %+v; want nil", fx.Deps.State.Batches[1])
			}
			if fx.Engine.PrepareCalls != 0 {
				t.Errorf("Engine.PrepareCalls = %d; want no strand prepared", fx.Engine.PrepareCalls)
			}

			restore()
			_, spawned, err = websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)
			if err != nil || !spawned {
				t.Fatalf("RecoverSpawnOrAttach() after the retry = spawned %v, error %v; want a spawned strand", spawned, err)
			}
		})
	}
}

// TestRecoverSpawnOrAttach_ContractFileEvidence proves a failed batch whose only uncheckable entry is a contract file
// is refused toward the delete route while a fork wrote the file last, and recovers once Master wrote it after the fork.
func TestRecoverSpawnOrAttach_ContractFileEvidence(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		master    []shuttleengine.WriteEvent
		wantSpawn bool
		wantInErr []string
		notInErr  []string
	}{
		{
			name:      "fork wrote last",
			master:    []shuttleengine.WriteEvent{{At: at, Succeeded: true}},
			wantInErr: []string{"batch 01", "rm ", "lyx webster recover-batch 1", "after Master's last write"},
			notInErr:  []string{"--fresh"},
		},
		{
			name:      "master wrote after the fork",
			master:    []shuttleengine.WriteEvent{{At: at.Add(2 * time.Minute), Succeeded: true}},
			wantSpawn: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newRecoverFixture(t)
			contract := websterengine.OutcomePath(fx.Deps.Geom.WebsterDir)
			if err := os.MkdirAll(filepath.Dir(contract), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(contract, []byte("outcome: done\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			master := slices.Clone(tt.master)
			master[0].Path = contract
			fx.Engine.AuditForksFn = func(string, string) (shuttleengine.ForkAudit, error) {
				return shuttleengine.ForkAudit{
					ParentWriteEvents: master,
					Forks:             []shuttleengine.ForkReport{{WriteEvents: []shuttleengine.WriteEvent{{Path: contract, At: at.Add(time.Minute), Succeeded: true}}}},
				}, nil
			}
			fx.Deps.State.MasterSessionID = "s1"
			rec := failedRecord("fork wrote the contract file")
			rec.Uncheckable = []string{contract}
			fx.Deps.State.Batches[1] = rec
			clk := &recoverFakeClock{now: time.Unix(0, 0)}

			_, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)
			if tt.wantSpawn {
				if err != nil || !spawned {
					t.Fatalf("RecoverSpawnOrAttach() = spawned %v, err %v; want a spawned recovery", spawned, err)
				}
				return
			}
			if err == nil || errors.Is(err, websterengine.ErrRecoveryNeedsFresh) {
				t.Fatalf("RecoverSpawnOrAttach() error = %v; want the delete refusal, not ErrRecoveryNeedsFresh", err)
			}
			for _, want := range tt.wantInErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
			for _, bad := range tt.notInErr {
				if strings.Contains(err.Error(), bad) {
					t.Errorf("error %q contains %q", err, bad)
				}
			}
			if spawned {
				t.Error("spawned = true; want no strand")
			}
		})
	}
}

// TestPersistRecoveryTerminal_WayForward_NoRecordedState proves a batch whose record vanished underneath the recovery wait names the recover-batch re-run,
// and that re-running spawns afresh.
func TestPersistRecoveryTerminal_WayForward_NoRecordedState(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	_, err := websterengine.PersistRecoveryTerminal(fx.Deps, fx.Deps.State, 1, &websterengine.Digest{Batch: "01-json-flag", Status: websterengine.DigestStatusDone})
	if err == nil || !strings.Contains(err.Error(), "way forward: re-run `lyx webster recover-batch 1`") {
		t.Fatalf("PersistRecoveryTerminal() error = %v; want the recover-batch re-run way forward", err)
	}

	if _, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk); err != nil || !spawned {
		t.Fatalf("RecoverSpawnOrAttach() after the way forward = spawned %v, error %v; want a spawned strand", spawned, err)
	}
}

// TestPersistRecoveryTerminal_RefusesForeignPlanEdit proves a card edited after the recovery spawned is refused with ErrFingerprintMismatch before the post-batch pass restamps over it, the record stays non-terminal,
// and the refusal leaves the begun card's CardHashes unchanged,
// including when a canonicalizing planglyph.ValidateDispatch ran after the edit with no restamp, as validate's path does when its edit check refuses.
func TestPersistRecoveryTerminal_RefusesForeignPlanEdit(t *testing.T) {
	for _, canonicalize := range []bool{false, true} {
		t.Run(fmt.Sprintf("canonicalize=%v", canonicalize), func(t *testing.T) {
			t.Parallel()
			fx := newRecoverFixture(t)
			planDir := fx.Deps.Geom.PlanDir
			if canonicalize {
				draft := "# Card 2 — list-tests\n\n**Create:**\n- `plan:internal/foo#Barr` -> `func Bar()`\n\n**Intent:** declare a draft handle.\n"
				if err := os.WriteFile(filepath.Join(planDir, "02-list-tests.md"), []byte(draft), 0o644); err != nil {
					t.Fatalf("write card 2: %v", err)
				}
				if err := websterengine.RestampPlanBaseline(fx.Deps.State, planDir, fx.Deps.Geom.WebsterDir); err != nil {
					t.Fatalf("RestampPlanBaseline() error = %v", err)
				}
			}
			clk := &recoverFakeClock{now: time.Unix(0, 0)}
			if _, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk); err != nil {
				t.Fatalf("first call error = %v; want nil", err)
			}
			recorded := fmt.Sprint(fx.Deps.State.Batches[1].CardHashes)
			if recorded == "map[]" {
				t.Fatal("recovery BatchState.CardHashes is empty; the test needs a recorded hash")
			}
			realHead := fx.Git.head
			writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+realHead+"\n")

			if err := os.WriteFile(filepath.Join(planDir, "01-json-flag.md"), []byte("# Card 1 — json-flag\n\n**Intent:** edited after the spawn.\n"), 0o644); err != nil {
				t.Fatalf("edit card: %v", err)
			}
			if canonicalize {
				plan, err := planparser.ParsePlan(planDir)
				if err != nil {
					t.Fatalf("ParsePlan() error = %v", err)
				}
				if _, err := planglyph.ValidateDispatch(plan, fx.Worktree, nil, nil); err != nil {
					t.Fatalf("ValidateDispatch() error = %v", err)
				}
				rewritten, err := os.ReadFile(filepath.Join(planDir, "02-list-tests.md"))
				if err != nil {
					t.Fatalf("read card 2: %v", err)
				}
				if !strings.Contains(string(rewritten), "plan:internal/foo#Bar`") {
					t.Fatalf("card 2 = %q; want the handle canonicalized, or the fixture exercises no rewrite", rewritten)
				}
			}

			if _, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk); !errors.Is(err, websterengine.ErrFingerprintMismatch) {
				t.Fatalf("second call error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
			}
			if bs := fx.Deps.State.Batches[1]; bs.Terminal {
				t.Errorf("BatchState.Terminal = true; want the record left non-terminal")
			}
			if got := fmt.Sprint(fx.Deps.State.Batches[1].CardHashes); got != recorded {
				t.Errorf("CardHashes = %s; want %s unchanged by the refusal", got, recorded)
			}
		})
	}
}
