// originreconcilemoves_integration_test.go tests what the origin reconcile moves and what it leaves in place.
// Its hubs are hubforge hubs.
// The other machine is a second clone of the hub's code or records bare repository.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// liveWayForward is the line a pair kept live names: no step runs now, and the run or cleanup judges it later.
const liveWayForward = "no step now: the run's own Parent-Align judges and moves its pair, and `lym cleanup --apply` re-judges it once the run has ended"

// Names of the lock files in a pair weft's lock directory.
const (
	weftWriteLockName = "weft.write.lock"
	weftPushLockName  = "fabric.push.lock"
)

// moveReconcile runs the origin reconcile over hub with scope and seams, failing the test on error.
func moveReconcile(t *testing.T, hub *originHub, scope fabriclayout.ReconcileScope, seams fabricengine.OriginSeams) fabriclayout.OriginReport {
	t.Helper()
	return moveReconcileWith(t, hub, scope, seams, fabricengine.ReconcileOptions{})
}

// moveReconcileWith runs the origin reconcile over hub with scope, seams and opts, failing the test on error.
func moveReconcileWith(t *testing.T, hub *originHub, scope fabriclayout.ReconcileScope, seams fabricengine.OriginSeams, opts fabricengine.ReconcileOptions) fabriclayout.OriginReport {
	t.Helper()
	report, err := fabricengine.ReconcileOrigin(hub.Location, scope, seams, opts)
	if err != nil {
		t.Fatalf("ReconcileOrigin(%+v) error = %v; want nil", scope, err)
	}
	return report
}

// publishBranch pushes the branch checked out at dir with an upstream and returns its name.
func publishBranch(tb testing.TB, dir string) string {
	tb.Helper()
	branch := gitkit.CurrentBranch(tb, dir)
	gitkit.Git(tb, dir, "push", "--set-upstream", "origin", branch)
	return branch
}

// originCommitFile commits rel from a second clone of bare at branch, pushes it and returns the new origin head.
func originCommitFile(t *testing.T, bare, branch, rel, content string) string {
	t.Helper()
	return gitkit.CommitOnBare(t, bare, branch, rel, content, "origin "+rel)
}

// holdLock holds the write lock at path until the test ends.
func holdLock(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	held, err := lock.AcquireWriteLock(path)
	if err != nil {
		t.Fatalf("acquire %s: %v", path, err)
	}
	t.Cleanup(func() { _ = held.Release() })
}

// movesPair is a hub with the pair "task" whose records branch is published and whose run has no record.
type movesPair struct {
	*originHub
	code        string
	codeBranch  string
	records     string
	recordsName string
}

// movesTemplate is originTaskTemplate's hub with the pair's records branch published.
var movesTemplate = hubforge.Registration{
	Key:  "fabricengine-origin-moves",
	Base: hubforge.Shape{Anchor: "."},
	Build: func(tb testing.TB, h *hubforge.Hub) {
		buildOriginTask(tb, h)
		publishBranch(tb, h.PairRecordsSibling("task"))
	},
}

// newMovesPair returns a copy of movesTemplate with the paths and branch names of its pair "task".
func newMovesPair(t *testing.T) *movesPair {
	t.Helper()
	hub := originHubOf(t, hubforge.CopyRegistered(t, &movesTemplate))
	code := hub.PairCodeWorktree("task")
	records := hub.PairRecordsSibling("task")
	return &movesPair{
		originHub:   hub,
		code:        code,
		codeBranch:  gitkit.CurrentBranch(t, code),
		records:     records,
		recordsName: gitkit.CurrentBranch(t, records),
	}
}

// TestReconcileOrigin_Moves covers the origin reconcile's moves: what it fast-forwards or replays.
// It also covers what a dirty path, a live pair, a held lock or a conflict leaves where it was.
func TestReconcileOrigin_Moves(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("A hub reconcile fast-forwards a weft, Bolt and the prime's code branch and a judge-only one moves nothing", func(t *testing.T) {
		hub := newOriginHub(t)
		warpHead := originCommitFile(t, hub.CodeBare, hub.primeBranch, "moved.txt", "moved")
		recordsHead := originCommitFile(t, hub.RecordsBare, hub.primeRecords, "moved.txt", "moved")
		boltHead := originCommitFile(t, hub.RecordsBare, "main", "moved.txt", "moved")
		refsBefore := refsOf(t, hub.PrimeRecords())

		judged := moveReconcile(t, hub, fabriclayout.ReconcileScope{JudgeOnly: true}, fabricengine.OriginSeams{})
		if got := judged.Mutations.Entries(); len(got) != 0 {
			t.Errorf("judge-only Mutations = %v; want none", got)
		}
		if got := refsOf(t, hub.PrimeRecords()); got != refsBefore {
			t.Errorf("refs after a judge-only reconcile:\n%s\nwant them unchanged:\n%s", got, refsBefore)
		}

		var calls callLog
		report := moveReconcileWith(t, hub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{CallHook: calls.record})

		for _, row := range []struct {
			name, repo, branch, want, dir string
		}{
			{"code", "code", hub.primeBranch, warpHead, hub.PrimeWorktree()},
			{"records", "records", hub.primeRecords, recordsHead, hub.PrimeRecords()},
			{"bolt", "bolt", "main", boltHead, hub.BoardDir()},
		} {
			outcome := requireState(t, report, row.repo, row.branch, fabriclayout.BranchLevel)
			if !outcome.Moved || outcome.WayForward != "" {
				t.Errorf("%s outcome = %+v; want it moved with no way forward", row.name, outcome)
			}
			if got := gitkit.RevParse(t, row.dir, "HEAD"); got != row.want {
				t.Errorf("%s HEAD = %q; want origin's head %q", row.name, got, row.want)
			}
		}
		underHub := func(dir string) string {
			rel, err := filepath.Rel(hub.Location.HubPath, dir)
			if err != nil {
				t.Fatalf("Rel(%s): %v", dir, err)
			}
			return filepath.ToSlash(rel)
		}
		wantMutations := []fabriclayout.Mutation{
			{Kind: fabriclayout.KindRepoAdvanced, Target: underHub(hub.BoardDir()), Detail: boltHead},
			{Kind: fabriclayout.KindRepoAdvanced, Target: underHub(hub.PrimeRecords()), Detail: recordsHead},
			{Kind: fabriclayout.KindRepoAdvanced, Target: underHub(hub.PrimeWorktree()), Detail: warpHead},
		}
		if got := report.Mutations.Entries(); !slices.Equal(got, wantMutations) {
			t.Errorf("Mutations = %+v; want %+v", got, wantMutations)
		}
		if got := len(calls.withVerb("ls-remote")); got != 2 {
			t.Errorf("ls-remote calls = %d; want exactly one per repository", got)
		}
		fetches := calls.withVerb("fetch")
		if len(fetches) != 2 {
			t.Fatalf("fetch calls = %v; want one per repository", fetches)
		}
		wantRefspec := func(branch string) string { return "+refs/heads/" + branch + ":refs/remotes/origin/" + branch }
		fetchedByRepo := [][]string{fetches[0][2:], fetches[1][2:]}
		slices.SortFunc(fetchedByRepo, func(a, b []string) int { return len(a) - len(b) })
		recordsBranches := []string{"main", hub.primeRecords}
		slices.Sort(recordsBranches)
		wantRecords := []string{wantRefspec(recordsBranches[0]), wantRefspec(recordsBranches[1])}
		if !slices.Equal(fetchedByRepo[0], []string{wantRefspec(hub.primeBranch)}) || !slices.Equal(fetchedByRepo[1], wantRecords) {
			t.Errorf("fetched refspecs = %v; want the moved heads only", fetchedByRepo)
		}
	})

	t.Run("A pair detached under its marker is judged by the marker's branch and its code worktree is not moved", func(t *testing.T) {
		pair := newMovesPair(t)
		location, err := lymcwd.ResolveWorktree(pair.code)
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		before := gitkit.RevParse(t, pair.code, "HEAD")
		originHead := originCommitFile(t, pair.CodeBare, pair.codeBranch, "moved.txt", "moved")
		gitkit.Git(t, pair.code, "checkout", "-q", "--detach")
		if err := fabricengine.WriteDetachMarker(location, pair.codeBranch); err != nil {
			t.Fatalf("WriteDetachMarker: %v", err)
		}

		report := moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "code", pair.codeBranch, fabriclayout.BranchFastForward)
		if outcome.Moved || outcome.OriginHead != originHead {
			t.Errorf("code outcome = %+v; want it judged against %s and unmoved", outcome, originHead)
		}
		if head, tip := gitkit.RevParse(t, pair.code, "HEAD"), gitkit.RevParse(t, pair.code, pair.codeBranch); head != before || tip != before {
			t.Errorf("HEAD = %s, task branch = %s; want both at %s", head, tip, before)
		}
	})

	t.Run("A locked detached worktree whose directory is gone fails no reconcile", func(t *testing.T) {
		hub := newOriginHub(t)
		gone := filepath.Join(t.TempDir(), "gone")
		gitkit.Git(t, hub.PrimeWorktree(), "worktree", "add", "-q", "--detach", gone)
		gitkit.Git(t, hub.PrimeWorktree(), "worktree", "lock", gone)
		if err := os.RemoveAll(gone); err != nil {
			t.Fatalf("remove the worktree directory: %v", err)
		}

		report := moveReconcile(t, hub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		requireState(t, report, "code", hub.primeBranch, fabriclayout.BranchLevel)
	})

	t.Run("A pair weft whose origin moved is fast-forwarded when clean on the touched paths and stopped as dirty otherwise", func(t *testing.T) {
		pair := newMovesPair(t)
		gitkit.CommitFile(t, pair.records, "notes/a.md", "base\n", "base note")
		gitkit.Git(t, pair.records, "push", "origin", pair.recordsName)
		originHead := originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/up.md", "up\n")

		report := moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "records", pair.recordsName, fabriclayout.BranchLevel)
		if !outcome.Moved || gitkit.RevParse(t, pair.records, "HEAD") != originHead {
			t.Errorf("clean pair weft outcome = %+v, HEAD = %q; want it fast-forwarded to %q", outcome, gitkit.RevParse(t, pair.records, "HEAD"), originHead)
		}

		originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/a.md", "origin\n")
		writeFile(t, filepath.Join(pair.records, "notes", "a.md"), "local\n")
		before := gitkit.RevParse(t, pair.records, "HEAD")

		report = moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome = requireState(t, report, "records", pair.recordsName, fabriclayout.BranchDirty)
		want := "commit the uncommitted paths (notes/a.md) with `lym fabric commit` in " + pair.code + ", or any of them with `git commit` or `git checkout -- <path>` in " + pair.records
		if outcome.WayForward != want || !slices.Equal(outcome.Paths, []string{"notes/a.md"}) || outcome.Moved {
			t.Errorf("dirty pair weft outcome = %+v; want WayForward %q, the path notes/a.md and nothing moved", outcome, want)
		}
		if got := gitkit.RevParse(t, pair.records, "HEAD"); got != before {
			t.Errorf("dirty pair weft HEAD = %q; want %q, nothing moved", got, before)
		}
	})

	t.Run("A weft whose origin tip carries another anchor marker, or whose own marker is empty, is left unmoved as anchor_stopped while the code branch moves", func(t *testing.T) {
		pair := newMovesPair(t)
		originCommitFile(t, pair.RecordsBare, pair.recordsName, ".lym-anchor", "sub\n")
		originCommitFile(t, pair.RecordsBare, pair.primeRecords, ".lym-anchor", "sub\n")
		codeHead := originCommitFile(t, pair.CodeBare, pair.codeBranch, "moved.txt", "moved")
		pairBefore := gitkit.RevParse(t, pair.records, "HEAD")
		primeBefore := gitkit.RevParse(t, pair.PrimeRecords(), "HEAD")

		report := moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		wantPair := "run `lym fabric anchor task --set sub` in " + pair.code
		pairOutcome := requireState(t, report, "records", pair.recordsName, fabriclayout.BranchAnchorStopped)
		if pairOutcome.WayForward != wantPair || pairOutcome.Anchor != "sub" || pairOutcome.Moved {
			t.Errorf("pair weft outcome = %+v; want WayForward %q, Anchor sub and nothing moved", pairOutcome, wantPair)
		}
		primeOutcome := requireState(t, report, "records", pair.primeRecords, fabriclayout.BranchAnchorStopped)
		for _, want := range []string{"`lym reed down`", "`lym fabric anchor --set sub` in " + pair.PrimeWorktree(), "`lym beam start`"} {
			if !strings.Contains(primeOutcome.WayForward, want) {
				t.Errorf("prime weft WayForward = %q; want it to name %s", primeOutcome.WayForward, want)
			}
		}
		if got := gitkit.RevParse(t, pair.records, "HEAD"); got != pairBefore {
			t.Errorf("pair weft HEAD = %q; want %q, nothing moved", got, pairBefore)
		}
		if got := gitkit.RevParse(t, pair.PrimeRecords(), "HEAD"); got != primeBefore {
			t.Errorf("prime weft HEAD = %q; want %q, nothing moved", got, primeBefore)
		}
		code := requireState(t, report, "code", pair.codeBranch, fabriclayout.BranchLevel)
		if !code.Moved || gitkit.RevParse(t, pair.code, "HEAD") != codeHead {
			t.Errorf("pair code outcome = %+v, HEAD = %q; want it fast-forwarded to %q", code, gitkit.RevParse(t, pair.code, "HEAD"), codeHead)
		}

		report = moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{Run: "task"}, fabricengine.OriginSeams{})
		pairOutcome = requireState(t, report, "records", pair.recordsName, fabriclayout.BranchAnchorStopped)
		if pairOutcome.WayForward != wantPair {
			t.Errorf("run-scoped pair weft WayForward = %q; want %q", pairOutcome.WayForward, wantPair)
		}

		writeFile(t, filepath.Join(pair.records, ".lym-anchor"), "\n")
		report = moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})
		pairOutcome = requireState(t, report, "records", pair.recordsName, fabriclayout.BranchAnchorStopped)
		if pairOutcome.Anchor != "" || !strings.Contains(pairOutcome.WayForward, ".lym-anchor") {
			t.Errorf("empty-marker pair weft outcome = %+v; want no Anchor and a way forward naming .lym-anchor", pairOutcome)
		}

		writeMoveRecord(t, pair.records, fabriclayout.MoveRecord{A: ".", B: "x", Kind: fabriclayout.MoveKindMove, PID: 4242})
		report = moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})
		pairOutcome = requireState(t, report, "records", pair.recordsName, fabriclayout.BranchAnchorStopped)
		wantMove := "a change of the anchor of task from . to x was interrupted; run `lym fabric anchor task --set x` to finish it"
		if pairOutcome.WayForward != wantMove {
			t.Errorf("move-record pair weft WayForward = %q; want %q", pairOutcome.WayForward, wantMove)
		}
	})

	t.Run("A live pair is judged and not moved, level while origin holds still and live once origin moved past it", func(t *testing.T) {
		pair := newMovesPair(t)
		seams := fabricengine.OriginSeams{LivePair: func(slug string) (bool, error) { return slug == "task", nil }}

		report := moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, seams)
		requireState(t, report, "code", pair.codeBranch, fabriclayout.BranchLevel)

		originHead := originCommitFile(t, pair.CodeBare, pair.codeBranch, "moved.txt", "moved")
		before := gitkit.RevParse(t, pair.code, "HEAD")

		report = moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, seams)

		outcome := requireState(t, report, "code", pair.codeBranch, fabriclayout.BranchLive)
		if outcome.WayForward != liveWayForward || outcome.Moved || gitkit.RevParse(t, pair.code, "HEAD") != before {
			t.Errorf("live outcome = %+v; want the live line and nothing moved", outcome)
		}

		report = moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{Run: "task"}, seams)

		outcome = requireState(t, report, "code", pair.codeBranch, fabriclayout.BranchLevel)
		if !outcome.Moved || gitkit.RevParse(t, pair.code, "HEAD") != originHead {
			t.Errorf("the run scope's own pair outcome = %+v; want it moved to %q although its driver reads live", outcome, originHead)
		}
	})

	t.Run("The prime is busy and not moved while a live owner holds its parent lock", func(t *testing.T) {
		hub := newOriginHub(t)
		originCommitFile(t, hub.CodeBare, hub.primeBranch, "moved.txt", "moved")
		originCommitFile(t, hub.RecordsBare, hub.primeRecords, "moved.txt", "moved")
		codeBefore := gitkit.RevParse(t, hub.PrimeWorktree(), "HEAD")
		recordsBefore := gitkit.RevParse(t, hub.PrimeRecords(), "HEAD")
		seams := fabricengine.OriginSeams{PrimeParentLockFree: func() (bool, error) { return false, nil }}

		report := moveReconcile(t, hub, fabriclayout.ReconcileScope{}, seams)

		for _, outcome := range []fabriclayout.BranchOutcome{
			requireState(t, report, "code", hub.primeBranch, fabriclayout.BranchBusy),
			requireState(t, report, "records", hub.primeRecords, fabriclayout.BranchBusy),
		} {
			if outcome.BusyLock != "prime parent" || outcome.Moved {
				t.Errorf("outcome = %+v; want busy on the prime parent lock, unmoved", outcome)
			}
		}
		if gitkit.RevParse(t, hub.PrimeWorktree(), "HEAD") != codeBefore || gitkit.RevParse(t, hub.PrimeRecords(), "HEAD") != recordsBefore {
			t.Error("a prime branch moved")
		}
	})

	t.Run("A records replay that conflicts is diverged, and a diverged code branch stays unrebased", func(t *testing.T) {
		pair := newMovesPair(t)
		local := gitkit.CommitFile(t, pair.records, "notes/c.md", "local\n", "local note")
		originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/c.md", "origin\n")
		localCode := gitkit.CommitFile(t, pair.code, "local.txt", "local", "local side")
		originCommitFile(t, pair.CodeBare, pair.codeBranch, "origin.txt", "origin")

		report := moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "records", pair.recordsName, fabriclayout.BranchDiverged)
		want := fabricengine.RecordsConflictWayForwardForTest(pair.records, pair.recordsName, []string{"notes/c.md"}, []string{local}, fabricengine.PushStepForTest(fabriclayout.FailedPush{Repo: "records", RepoPath: pair.records, CodeWorktree: pair.code, Branch: pair.recordsName}))
		if outcome.WayForward != want || !slices.Equal(outcome.Paths, []string{"notes/c.md"}) || !strings.Contains(report.Summary(), want) {
			t.Errorf("diverged weft outcome = %+v, Summary() = %q; want the paths notes/c.md and %q", outcome, report.Summary(), want)
		}
		if got := gitkit.RevParse(t, pair.records, "HEAD"); got != local {
			t.Errorf("diverged weft HEAD = %q; want the starting tip %q restored", got, local)
		}
		requireState(t, report, "code", pair.codeBranch, fabriclayout.BranchDiverged)
		if got := gitkit.RevParse(t, pair.code, "HEAD"); got != localCode {
			t.Errorf("diverged code HEAD = %q; want %q, never rebased", got, localCode)
		}
	})

	t.Run("A held pair weft lock is busy, the push lock naming the parked run", func(t *testing.T) {
		for _, row := range []struct {
			name, lockFile, wantLock, wantWayForward string
			wantPushHeld                             bool
		}{
			{"write lock", weftWriteLockName, "weft write", "wait for the holder of the weft write lock to end", false},
			{"push lock", weftPushLockName, "weft push", "`lym tender resume task`", true},
		} {
			pair := newMovesPair(t)
			originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/up.md", "up\n")
			holdLock(t, filepath.Join(fabriclayout.RecordsLockDirPath(pair.records), row.lockFile))
			before := gitkit.RevParse(t, pair.records, "HEAD")

			report := moveReconcile(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{})

			outcome := requireState(t, report, "records", pair.recordsName, fabriclayout.BranchBusy)
			if outcome.BusyLock != row.wantLock || outcome.PushLockHeld != row.wantPushHeld || outcome.WayForward != row.wantWayForward || outcome.Moved {
				t.Errorf("%s: outcome = %+v; want busy on %q with way forward %q", row.name, outcome, row.wantLock, row.wantWayForward)
			}
			if gitkit.RevParse(t, pair.records, "HEAD") != before {
				t.Errorf("%s: the pair weft moved", row.name)
			}
		}
	})

	t.Run("A held pair weft push lock is waited on up to the bound, the weft moving once it is released and busy while it stays held", func(t *testing.T) {
		for _, row := range []struct {
			name string
			// releaseAt is the poll sleep at which the holder lets go of the lock; 0 holds it throughout.
			releaseAt  int
			wantState  fabriclayout.BranchState
			wantSleeps int
		}{
			{"released during the wait", 2, fabriclayout.BranchLevel, 2},
			{"held past the bound", 0, fabriclayout.BranchBusy, 3},
		} {
			pair := newMovesPair(t)
			originHead := originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/up.md", "up\n")
			path := filepath.Join(fabriclayout.RecordsLockDirPath(pair.records), weftPushLockName)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("mkdir for %s: %v", path, err)
			}
			held, err := lock.AcquireWriteLock(path)
			if err != nil {
				t.Fatalf("acquire %s: %v", path, err)
			}
			released := false
			release := func() {
				if !released {
					released = true
					_ = held.Release()
				}
			}
			t.Cleanup(release)
			sleeps := 0
			opts := fabricengine.ReconcileOptions{PushLockWait: 3 * time.Second, Sleep: func(time.Duration) {
				sleeps++
				if sleeps == row.releaseAt {
					release()
				}
			}}

			report := moveReconcileWith(t, pair.originHub, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{}, opts)

			outcome := requireState(t, report, "records", pair.recordsName, row.wantState)
			if sleeps != row.wantSleeps {
				t.Errorf("%s: poll sleeps = %d; want %d", row.name, sleeps, row.wantSleeps)
			}
			moved := gitkit.RevParse(t, pair.records, "HEAD") == originHead
			if wantMoved := row.wantState == fabriclayout.BranchLevel; outcome.Moved != wantMoved || moved != wantMoved {
				t.Errorf("%s: outcome = %+v, at origin's head %v; want moved %v", row.name, outcome, moved, wantMoved)
			}
		}
	})

	t.Run("A held Bolt push lock, prime weft write lock and run-scope own weft write lock are busy and unmoved", func(t *testing.T) {
		for _, row := range []struct {
			name     string
			scope    fabriclayout.ReconcileScope
			hold     func(pair *movesPair) string
			moveWhat func(pair *movesPair)
			repo     string
			branch   func(pair *movesPair) string
		}{
			{
				"Bolt push lock", fabriclayout.ReconcileScope{},
				func(pair *movesPair) string {
					path, err := gitrepo.New(pair.BoardDir()).PushLockPath()
					if err != nil {
						t.Fatalf("locate the Bolt push lock: %v", err)
					}
					return path
				},
				func(pair *movesPair) { originCommitFile(t, pair.RecordsBare, "main", "moved.txt", "moved") },
				"bolt", func(*movesPair) string { return "main" },
			},
			{
				"prime weft write lock", fabriclayout.ReconcileScope{},
				func(pair *movesPair) string {
					return filepath.Join(fabriclayout.RecordsLockDirPath(pair.PrimeRecords()), weftWriteLockName)
				},
				func(pair *movesPair) { originCommitFile(t, pair.RecordsBare, pair.primeRecords, "moved.txt", "moved") },
				"records", func(pair *movesPair) string { return pair.primeRecords },
			},
			{
				"own weft write lock", fabriclayout.ReconcileScope{Run: "task"},
				func(pair *movesPair) string {
					return filepath.Join(fabriclayout.RecordsLockDirPath(pair.records), weftWriteLockName)
				},
				func(pair *movesPair) { originCommitFile(t, pair.RecordsBare, pair.recordsName, "moved.txt", "moved") },
				"records", func(pair *movesPair) string { return pair.recordsName },
			},
		} {
			pair := newMovesPair(t)
			row.moveWhat(pair)
			holdLock(t, row.hold(pair))

			report := moveReconcile(t, pair.originHub, row.scope, fabricengine.OriginSeams{})

			outcome := requireState(t, report, row.repo, row.branch(pair), fabriclayout.BranchBusy)
			if outcome.Moved {
				t.Errorf("%s: outcome = %+v; want it unmoved", row.name, outcome)
			}
		}
	})
}
