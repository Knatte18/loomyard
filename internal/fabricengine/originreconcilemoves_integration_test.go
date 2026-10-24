//go:build tier1

// originreconcilemoves_integration_test.go proves what the origin reconcile moves and what it leaves where it was, on hubforge hubs.
// The other machine is a second clone of the hub's code or records bare repository.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

// liveWayForward is the line a pair kept live names, which ends in no re-entry.
const liveWayForward = "no step now: the run's own Merge-In judges and moves its pair, and `lyx cleanup --apply` re-judges it once the run has ended"

// The lock files a pair weft's lock directory holds.
const (
	weftWriteLockName = "weft.write.lock"
	weftPushLockName  = "fabric.push.lock"
)

// moveReconcile runs a reconcile with seams over hub.
func moveReconcile(t *testing.T, hub *originHub, scope fabricengine.ReconcileScope, seams fabricengine.OriginSeams) fabricengine.OriginReport {
	t.Helper()
	report, err := fabricengine.ReconcileOrigin(hub.Location, scope, seams, fabricengine.ReconcileOptions{})
	if err != nil {
		t.Fatalf("ReconcileOrigin(%+v) error = %v; want nil", scope, err)
	}
	return report
}

// publishBranch pushes the branch checked out at dir with an upstream and returns its name.
func publishBranch(t *testing.T, dir string) string {
	t.Helper()
	branch := gitkit.CurrentBranch(t, dir)
	gitkit.Git(t, dir, "push", "--set-upstream", "origin", branch)
	return branch
}

// originCommitFile commits rel from a second clone of bare at branch, pushes it and returns the new origin head.
func originCommitFile(t *testing.T, bare, branch, rel, content string) string {
	t.Helper()
	other := otherMachine(t, bare, branch)
	sha := gitkit.CommitFile(t, other, rel, content, "origin "+rel)
	gitkit.Git(t, other, "push", "origin", branch)
	return sha
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

// entryOf returns the entry for slug, failing the test when the report holds none.
func entryOf(t *testing.T, report fabricengine.OriginReport, slug string) fabricengine.EntryOutcome {
	t.Helper()
	for _, entry := range report.Entries {
		if entry.Slug == slug {
			return entry
		}
	}
	t.Fatalf("report holds no entry %q; entries = %+v", slug, report.Entries)
	return fabricengine.EntryOutcome{}
}

// requireEntry fails the test unless slug's entry is in state.
func requireEntry(t *testing.T, report fabricengine.OriginReport, slug string, state fabricengine.EntryState) fabricengine.EntryOutcome {
	t.Helper()
	entry := entryOf(t, report, slug)
	if entry.State != state {
		t.Errorf("entry %q state = %q; want %q", slug, entry.State, state)
	}
	return entry
}

// kindCount counts the report's mutation entries of kind.
func kindCount(report fabricengine.OriginReport, kind fabricengine.Kind) int {
	count := 0
	for _, mutation := range report.Mutations.Entries() {
		if mutation.Kind == kind {
			count++
		}
	}
	return count
}

// movesPair is a hub with the pair "task" whose records branch is published and whose run has a batten record.
type movesPair struct {
	*originHub
	code        string
	codeBranch  string
	records     string
	recordsName string
}

func newMovesPair(t *testing.T) *movesPair {
	t.Helper()
	hub := newOriginHub(t)
	code, codeBranch := originPair(t, hub, "task")
	records := hub.PairRecordsSibling("task")
	writeStatus(t, hub, "task", "Land")
	return &movesPair{originHub: hub, code: code, codeBranch: codeBranch, records: records, recordsName: publishBranch(t, records)}
}

// TestReconcileOrigin_Moves covers the origin reconcile's moves: what it fast-forwards or replays, and what a dirty path, a live pair, a held lock or a conflict leaves where it was.
// It runs without t.Parallel because it sets the process-global origin call hook.
func TestReconcileOrigin_Moves(t *testing.T) {
	boardkit.InstallReplay(t)

	t.Run("A hub reconcile fast-forwards a weft, Bolt and the prime's code branch and a judge-only one moves nothing", func(t *testing.T) {
		hub := newOriginHub(t)
		warpHead := originCommitFile(t, hub.CodeBare, hub.primeBranch, "moved.txt", "moved")
		recordsHead := originCommitFile(t, hub.RecordsBare, hub.primeRecords, "moved.txt", "moved")
		boltHead := originCommitFile(t, hub.RecordsBare, "main", "moved.txt", "moved")
		refsBefore := refsOf(t, hub.PrimeRecords())

		judged := moveReconcile(t, hub, fabricengine.ReconcileScope{JudgeOnly: true}, fabricengine.OriginSeams{})
		if got := judged.Mutations.Entries(); len(got) != 0 {
			t.Errorf("judge-only Mutations = %v; want none", got)
		}
		if got := refsOf(t, hub.PrimeRecords()); got != refsBefore {
			t.Errorf("refs after a judge-only reconcile:\n%s\nwant them unchanged:\n%s", got, refsBefore)
		}

		var calls callLog
		t.Cleanup(fabricengine.SetOriginCallHookForTest(calls.record))
		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

		for _, row := range []struct {
			name, repo, branch, want, dir string
		}{
			{"code", "code", hub.primeBranch, warpHead, hub.PrimeWorktree()},
			{"records", "records", hub.primeRecords, recordsHead, hub.PrimeRecords()},
			{"bolt", "bolt", "main", boltHead, hub.BoardDir()},
		} {
			outcome := requireState(t, report, row.repo, row.branch, fabricengine.BranchLevel)
			if !outcome.Moved || outcome.WayForward != "" {
				t.Errorf("%s outcome = %+v; want it moved with no way forward", row.name, outcome)
			}
			if got := gitkit.RevParse(t, row.dir, "HEAD"); got != row.want {
				t.Errorf("%s HEAD = %q; want origin's head %q", row.name, got, row.want)
			}
		}
		if got := kindCount(report, fabricengine.KindRepoAdvanced); got != 3 {
			t.Errorf("repo_advanced entries = %d; want one per fast-forward, 3", got)
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

	t.Run("A pair weft whose origin moved is fast-forwarded when clean on the touched paths and stopped as dirty otherwise", func(t *testing.T) {
		pair := newMovesPair(t)
		gitkit.CommitFile(t, pair.records, "notes/a.md", "base\n", "base note")
		gitkit.Git(t, pair.records, "push", "origin", pair.recordsName)
		originHead := originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/up.md", "up\n")

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "records", pair.recordsName, fabricengine.BranchLevel)
		if !outcome.Moved || gitkit.RevParse(t, pair.records, "HEAD") != originHead {
			t.Errorf("clean pair weft outcome = %+v, HEAD = %q; want it fast-forwarded to %q", outcome, gitkit.RevParse(t, pair.records, "HEAD"), originHead)
		}
		requireEntry(t, report, "task", fabricengine.EntryActive)

		originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/a.md", "origin\n")
		writeFile(t, filepath.Join(pair.records, "notes", "a.md"), "local\n")
		before := gitkit.RevParse(t, pair.records, "HEAD")

		report = moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome = requireState(t, report, "records", pair.recordsName, fabricengine.BranchDirty)
		want := "commit the uncommitted paths (notes/a.md) with `lyx fabric commit` in " + pair.code + ", or any of them with `git commit` or `git checkout -- <path>` in " + pair.records
		if outcome.WayForward != want || !slices.Equal(outcome.Paths, []string{"notes/a.md"}) || outcome.Moved {
			t.Errorf("dirty pair weft outcome = %+v; want WayForward %q, the path notes/a.md and nothing moved", outcome, want)
		}
		if got := gitkit.RevParse(t, pair.records, "HEAD"); got != before {
			t.Errorf("dirty pair weft HEAD = %q; want %q, nothing moved", got, before)
		}
		entry := requireEntry(t, report, "task", fabricengine.EntryStopped)
		if entry.WayForward != want {
			t.Errorf("entry WayForward = %q; want its branch's line %q", entry.WayForward, want)
		}
	})

	t.Run("A tracked change on a path the prime weft's or a pair's code branch fast-forward touches is dirty with that branch's own line", func(t *testing.T) {
		pair := newMovesPair(t)
		gitkit.CommitFile(t, pair.PrimeRecords(), "notes/p.md", "base\n", "base prime note")
		gitkit.Git(t, pair.PrimeRecords(), "push", "origin", pair.primeRecords)
		gitkit.CommitFile(t, pair.code, "code.txt", "base\n", "base code")
		gitkit.Git(t, pair.code, "push", "origin", pair.codeBranch)
		originCommitFile(t, pair.RecordsBare, pair.primeRecords, "notes/p.md", "origin\n")
		originCommitFile(t, pair.CodeBare, pair.codeBranch, "code.txt", "origin\n")
		writeFile(t, filepath.Join(pair.PrimeRecords(), "notes", "p.md"), "local\n")
		writeFile(t, filepath.Join(pair.code, "code.txt"), "local\n")

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

		prime := requireState(t, report, "records", pair.primeRecords, fabricengine.BranchDirty)
		wantPrime := "commit the uncommitted paths (notes/p.md) with `lyx fabric commit` in " + pair.PrimeWorktree() + ", or any of them with `git commit` or `git checkout -- <path>` in " + pair.PrimeRecords()
		if prime.WayForward != wantPrime {
			t.Errorf("prime weft WayForward = %q; want %q", prime.WayForward, wantPrime)
		}
		code := requireState(t, report, "code", pair.codeBranch, fabricengine.BranchDirty)
		wantCode := "commit or restore the uncommitted paths (code.txt) with `git commit` or `git checkout -- <path>` in " + pair.code
		if code.WayForward != wantCode {
			t.Errorf("pair code WayForward = %q; want %q", code.WayForward, wantCode)
		}
		requireEntry(t, report, "task", fabricengine.EntryStopped)
	})

	t.Run("A live pair is judged and not moved, active while level or ahead and stopped once origin moved past it", func(t *testing.T) {
		pair := newMovesPair(t)
		seams := fabricengine.OriginSeams{LivePair: func(slug string) (bool, error) { return slug == "task", nil }}

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, seams)
		requireState(t, report, "code", pair.codeBranch, fabricengine.BranchLevel)
		requireEntry(t, report, "task", fabricengine.EntryActive)

		originHead := originCommitFile(t, pair.CodeBare, pair.codeBranch, "moved.txt", "moved")
		before := gitkit.RevParse(t, pair.code, "HEAD")

		report = moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, seams)

		outcome := requireState(t, report, "code", pair.codeBranch, fabricengine.BranchLive)
		if outcome.WayForward != liveWayForward || outcome.Moved || gitkit.RevParse(t, pair.code, "HEAD") != before {
			t.Errorf("live outcome = %+v; want the live line and nothing moved", outcome)
		}
		if entry := requireEntry(t, report, "task", fabricengine.EntryStopped); entry.WayForward != liveWayForward {
			t.Errorf("entry WayForward = %q; want the live line", entry.WayForward)
		}

		report = moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{Run: "task"}, seams)

		outcome = requireState(t, report, "code", pair.codeBranch, fabricengine.BranchLevel)
		if !outcome.Moved || gitkit.RevParse(t, pair.code, "HEAD") != originHead {
			t.Errorf("the run scope's own pair outcome = %+v; want it moved to %q although its driver reads live", outcome, originHead)
		}
		requireEntry(t, report, "task", fabricengine.EntryActive)
	})

	t.Run("The prime is busy and not moved while its prime lock is held or a live owner holds its parent lock", func(t *testing.T) {
		for _, row := range []struct {
			name     string
			hold     func(t *testing.T, hub *originHub)
			seams    fabricengine.OriginSeams
			wantLock string
		}{
			{"the prime lock held", func(t *testing.T, hub *originHub) { holdLock(t, shedrun.PrimeRunLock(hub.Location)) }, fabricengine.OriginSeams{}, "prime"},
			{"a live parent lock owner", func(*testing.T, *originHub) {}, fabricengine.OriginSeams{PrimeParentLockFree: func() (bool, error) { return false, nil }}, "prime parent"},
		} {
			hub := newOriginHub(t)
			row.hold(t, hub)
			originCommitFile(t, hub.CodeBare, hub.primeBranch, "moved.txt", "moved")
			originCommitFile(t, hub.RecordsBare, hub.primeRecords, "moved.txt", "moved")
			codeBefore := gitkit.RevParse(t, hub.PrimeWorktree(), "HEAD")
			recordsBefore := gitkit.RevParse(t, hub.PrimeRecords(), "HEAD")

			report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, row.seams)

			for _, outcome := range []fabricengine.BranchOutcome{
				requireState(t, report, "code", hub.primeBranch, fabricengine.BranchBusy),
				requireState(t, report, "records", hub.primeRecords, fabricengine.BranchBusy),
			} {
				if outcome.BusyLock != row.wantLock || outcome.Moved {
					t.Errorf("%s: outcome = %+v; want busy on the %s lock, unmoved", row.name, outcome, row.wantLock)
				}
			}
			if gitkit.RevParse(t, hub.PrimeWorktree(), "HEAD") != codeBefore || gitkit.RevParse(t, hub.PrimeRecords(), "HEAD") != recordsBefore {
				t.Errorf("%s: a prime branch moved", row.name)
			}
		}
	})

	t.Run("A run scope judges the parent branch behind origin and does not move it", func(t *testing.T) {
		pair := newMovesPair(t)
		originCommitFile(t, pair.CodeBare, pair.primeBranch, "moved.txt", "moved")
		before := gitkit.RevParse(t, pair.PrimeWorktree(), "HEAD")

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{Run: "task"}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "code", pair.primeBranch, fabricengine.BranchFastForward)
		if outcome.Moved || gitkit.RevParse(t, pair.PrimeWorktree(), "HEAD") != before {
			t.Errorf("parent outcome = %+v; want it judged only", outcome)
		}
	})

	t.Run("A records replay that conflicts is diverged and stops its entry, and a diverged code branch stays unrebased", func(t *testing.T) {
		pair := newMovesPair(t)
		local := gitkit.CommitFile(t, pair.records, "notes/c.md", "local\n", "local note")
		originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/c.md", "origin\n")
		localCode := gitkit.CommitFile(t, pair.code, "local.txt", "local", "local side")
		originCommitFile(t, pair.CodeBare, pair.codeBranch, "origin.txt", "origin")

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "records", pair.recordsName, fabricengine.BranchDiverged)
		want := fabricengine.RecordsConflictWayForwardForTest(pair.records, pair.recordsName, []string{"notes/c.md"}, []string{local}, fabricengine.PushStepForTest(fabricengine.FailedPush{Repo: "records", RepoPath: pair.records, CodeWorktree: pair.code, Branch: pair.recordsName}))
		if outcome.WayForward != want || !slices.Equal(outcome.Paths, []string{"notes/c.md"}) || !strings.Contains(report.Summary(), want) {
			t.Errorf("diverged weft outcome = %+v, Summary() = %q; want the paths notes/c.md and %q", outcome, report.Summary(), want)
		}
		if got := gitkit.RevParse(t, pair.records, "HEAD"); got != local {
			t.Errorf("diverged weft HEAD = %q; want the starting tip %q restored", got, local)
		}
		requireState(t, report, "code", pair.codeBranch, fabricengine.BranchDiverged)
		if got := gitkit.RevParse(t, pair.code, "HEAD"); got != localCode {
			t.Errorf("diverged code HEAD = %q; want %q, never rebased", got, localCode)
		}
		requireEntry(t, report, "task", fabricengine.EntryStopped)
	})

	t.Run("A both-sides change of the prime weft's records stops every entry with the conflict line", func(t *testing.T) {
		pair := newMovesPair(t)
		local := gitkit.CommitFile(t, pair.PrimeRecords(), "notes/c.md", "local\n", "local note")
		originCommitFile(t, pair.RecordsBare, pair.primeRecords, "notes/c.md", "origin\n")

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "records", pair.primeRecords, fabricengine.BranchDiverged)
		want := fabricengine.RecordsConflictWayForwardForTest(pair.PrimeRecords(), pair.primeRecords, []string{"notes/c.md"}, []string{local}, fabricengine.PushStepForTest(fabricengine.FailedPush{Repo: "records", RepoPath: pair.PrimeRecords(), CodeWorktree: pair.PrimeWorktree(), Branch: pair.primeRecords, Prime: true}))
		if outcome.WayForward != want {
			t.Errorf("prime weft WayForward = %q; want %q", outcome.WayForward, want)
		}
		if entry := requireEntry(t, report, "task", fabricengine.EntryStopped); entry.WayForward != want {
			t.Errorf("entry WayForward = %q; want the prime weft's line %q", entry.WayForward, want)
		}
	})

	t.Run("A Bolt replay sets aside the local status of an entry origin ended, with two ls-remote calls and no wait", func(t *testing.T) {
		hub := newOriginHub(t)
		board := hub.BoardDir()
		branch := gitkit.CurrentBranch(t, board)
		gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("neve")), "base board")
		gitkit.Git(t, board, "push", "origin", branch)
		running := boardEntry("neve", withBoardStatus(boardengine.RunStatus("running", "Implementer")))
		local := gitkit.CommitFile(t, board, "board.json", boardFileOf(t, running), "neve runs")
		originHead := originCommitFile(t, hub.RecordsBare, branch, "board.json", boardFileOf(t, boardEntry("neve", withBoardStatus("done"))))
		var calls callLog
		t.Cleanup(fabricengine.SetOriginCallHookForTest(calls.record))

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "bolt", branch, fabricengine.BranchLevel)
		if !outcome.Moved || gitkit.RevParse(t, board, "HEAD") != originHead {
			t.Errorf("bolt outcome = %+v, HEAD = %q; want the local commit set aside and Bolt at origin's head %q", outcome, gitkit.RevParse(t, board, "HEAD"), originHead)
		}
		if got := tagsOf(t, board, "set-aside/neve/*"); !slices.Equal(got, []string{"set-aside/neve/" + local[:12]}) {
			t.Errorf("set-aside tags = %v; want one for %s", got, local)
		}
		if got := kindCount(report, fabricengine.KindTagCreated); got != 1 {
			t.Errorf("tag_created entries = %d; want one per set-aside tag", got)
		}
		if got := len(calls.withVerb("ls-remote")); got != 2 {
			t.Errorf("ls-remote calls = %d; want 2", got)
		}
	})

	t.Run("A tracked change on a path Bolt's move touches stops every entry with Bolt's dirty line", func(t *testing.T) {
		pair := newMovesPair(t)
		board := pair.BoardDir()
		branch := gitkit.CurrentBranch(t, board)
		gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("neve")), "base board")
		gitkit.Git(t, board, "push", "origin", branch)
		originCommitFile(t, pair.RecordsBare, branch, "board.json", boardFileOf(t, boardEntry("neve", withBoardTitle("origin"))))
		writeFile(t, filepath.Join(board, "board.json"), boardFileOf(t, boardEntry("neve", withBoardTitle("local"))))

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

		outcome := requireState(t, report, "bolt", branch, fabricengine.BranchDirty)
		want := "commit or restore the uncommitted paths (board.json) with `git commit` or `git checkout -- <path>` in " + board
		if outcome.WayForward != want {
			t.Errorf("bolt WayForward = %q; want %q", outcome.WayForward, want)
		}
		if entry := requireEntry(t, report, "task", fabricengine.EntryStopped); entry.WayForward != want {
			t.Errorf("entry WayForward = %q; want Bolt's line %q", entry.WayForward, want)
		}
	})

	t.Run("A held pair weft lock is busy and stops that entry, the push lock naming the parked run", func(t *testing.T) {
		for _, row := range []struct {
			name, lockFile, wantLock, wantWayForward string
			wantPushHeld                             bool
		}{
			{"write lock", weftWriteLockName, "weft write", "wait for the holder of the weft write lock to end", false},
			{"push lock", weftPushLockName, "weft push", "`lyx resume task`", true},
		} {
			pair := newMovesPair(t)
			originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/up.md", "up\n")
			holdLock(t, filepath.Join(fabricengine.RecordsLockDirPath(pair.records), row.lockFile))
			before := gitkit.RevParse(t, pair.records, "HEAD")

			report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})

			outcome := requireState(t, report, "records", pair.recordsName, fabricengine.BranchBusy)
			if outcome.BusyLock != row.wantLock || outcome.PushLockHeld != row.wantPushHeld || outcome.WayForward != row.wantWayForward || outcome.Moved {
				t.Errorf("%s: outcome = %+v; want busy on %q with way forward %q", row.name, outcome, row.wantLock, row.wantWayForward)
			}
			if gitkit.RevParse(t, pair.records, "HEAD") != before {
				t.Errorf("%s: the pair weft moved", row.name)
			}
			requireEntry(t, report, "task", fabricengine.EntryStopped)
		}
	})

	t.Run("A held Bolt push lock, prime weft write lock and run-scope own weft write lock are busy and stop nothing", func(t *testing.T) {
		for _, row := range []struct {
			name     string
			scope    fabricengine.ReconcileScope
			hold     func(pair *movesPair) string
			moveWhat func(pair *movesPair)
			repo     string
			branch   func(pair *movesPair) string
		}{
			{
				"Bolt push lock", fabricengine.ReconcileScope{},
				func(pair *movesPair) string { return filepath.Join(pair.BoardDir(), gitrepo.PushLockFileName) },
				func(pair *movesPair) { originCommitFile(t, pair.RecordsBare, "main", "moved.txt", "moved") },
				"bolt", func(*movesPair) string { return "main" },
			},
			{
				"prime weft write lock", fabricengine.ReconcileScope{},
				func(pair *movesPair) string {
					return filepath.Join(fabricengine.RecordsLockDirPath(pair.PrimeRecords()), weftWriteLockName)
				},
				func(pair *movesPair) { originCommitFile(t, pair.RecordsBare, pair.primeRecords, "moved.txt", "moved") },
				"records", func(pair *movesPair) string { return pair.primeRecords },
			},
			{
				"own weft write lock", fabricengine.ReconcileScope{Run: "task"},
				func(pair *movesPair) string {
					return filepath.Join(fabricengine.RecordsLockDirPath(pair.records), weftWriteLockName)
				},
				func(pair *movesPair) { originCommitFile(t, pair.RecordsBare, pair.recordsName, "moved.txt", "moved") },
				"records", func(pair *movesPair) string { return pair.recordsName },
			},
		} {
			pair := newMovesPair(t)
			row.moveWhat(pair)
			holdLock(t, row.hold(pair))

			report := moveReconcile(t, pair.originHub, row.scope, fabricengine.OriginSeams{})

			outcome := requireState(t, report, row.repo, row.branch(pair), fabricengine.BranchBusy)
			if outcome.Moved {
				t.Errorf("%s: outcome = %+v; want it unmoved", row.name, outcome)
			}
			requireEntry(t, report, "task", fabricengine.EntryActive)
		}
	})

	t.Run("An entry origin's Bolt gives to another machine is owned_elsewhere with its owner, way forward and summary line, in a run and a judge-only hub scope", func(t *testing.T) {
		const wantWayForward = "`lyx adopt task` from this machine's prime takes it over"
		const wantLine = "owned elsewhere: task by other-machine; `lyx adopt task` takes it over"
		for _, scope := range []fabricengine.ReconcileScope{{Run: "task"}, {JudgeOnly: true}} {
			pair := newMovesPair(t)
			var asked []string
			seams := fabricengine.OriginSeams{OwnerOf: func(_ string, _ []byte, slug string) (string, bool, error) {
				asked = append(asked, slug)
				return "other-machine", true, nil
			}}

			report := moveReconcile(t, pair.originHub, scope, seams)

			entry := requireEntry(t, report, "task", fabricengine.EntryOwnedElsewhere)
			if entry.Owner != "other-machine" || entry.WayForward != wantWayForward {
				t.Errorf("scope %+v: entry = %+v; want owner other-machine and way forward %q", scope, entry, wantWayForward)
			}
			if !strings.Contains(report.Summary(), wantLine) {
				t.Errorf("scope %+v: Summary() = %q; want the line %q", scope, report.Summary(), wantLine)
			}
			if !slices.Equal(asked, []string{"task"}) {
				t.Errorf("scope %+v: OwnerOf asked about %v; want one question for task", scope, asked)
			}
		}
	})

	t.Run("A records branch both sides moved stays unmoved for an entry owned elsewhere while its fast-forwards move, and its pair is never torn down", func(t *testing.T) {
		pair := newMovesPair(t)
		localRecords := gitkit.CommitFile(t, pair.records, "notes/local.md", "local\n", "local note")
		originCommitFile(t, pair.RecordsBare, pair.recordsName, "notes/origin.md", "origin\n")
		originCode := originCommitFile(t, pair.CodeBare, pair.codeBranch, "moved.txt", "moved")
		torn := &teardownRecorder{}
		seams := torn.seams()
		seams.OwnerOf = func(string, []byte, string) (string, bool, error) { return "other-machine", true, nil }

		report := moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, seams)

		outcome := requireState(t, report, "records", pair.recordsName, fabricengine.BranchNeedsReplay)
		if outcome.Moved || gitkit.RevParse(t, pair.records, "HEAD") != localRecords {
			t.Errorf("records outcome = %+v, HEAD = %q; want it unreplayed at %q", outcome, gitkit.RevParse(t, pair.records, "HEAD"), localRecords)
		}
		if code := requireState(t, report, "code", pair.codeBranch, fabricengine.BranchLevel); !code.Moved || gitkit.RevParse(t, pair.code, "HEAD") != originCode {
			t.Errorf("code outcome = %+v; want the fast-forward moved to %q", code, originCode)
		}
		requireEntry(t, report, "task", fabricengine.EntryOwnedElsewhere)
		if len(torn.calls) != 0 {
			t.Errorf("teardown calls = %v; want none for an entry owned elsewhere", torn.slugsCalled())
		}
	})

	t.Run("An ended entry owned elsewhere reports ended, a nil OwnerOf reads it active, an OwnerOf error is the reconcile's error and a hub with no Bolt origin head asks nothing", func(t *testing.T) {
		hub, _ := endedHub(t, "task")
		torn := &teardownRecorder{}
		seams := torn.seams()
		seams.OwnerOf = func(string, []byte, string) (string, bool, error) { return "other-machine", true, nil }
		requireEntry(t, moveReconcile(t, hub, fabricengine.ReconcileScope{}, seams), "task", fabricengine.EntryEnded)

		pair := newMovesPair(t)
		requireEntry(t, moveReconcile(t, pair.originHub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{}), "task", fabricengine.EntryActive)

		failing := fabricengine.OriginSeams{OwnerOf: func(string, []byte, string) (string, bool, error) { return "", false, errors.New("no hostname") }}
		if _, err := fabricengine.ReconcileOrigin(pair.Location, fabricengine.ReconcileScope{}, failing, fabricengine.ReconcileOptions{}); err == nil || !strings.Contains(err.Error(), "no hostname") {
			t.Errorf("ReconcileOrigin error = %v; want the seam's error", err)
		}

		bare := newOriginHub(t)
		originPair(t, bare, "task")
		writeStatus(t, bare, "task", "Land")
		gitkit.Git(t, bare.BoardDir(), "update-ref", "-d", "refs/remotes/origin/"+gitkit.CurrentBranch(t, bare.BoardDir()))
		asked := false
		quiet := fabricengine.OriginSeams{OwnerOf: func(string, []byte, string) (string, bool, error) { asked = true; return "", false, nil }}
		moveReconcile(t, bare, fabricengine.ReconcileScope{}, quiet)
		if asked {
			t.Error("OwnerOf was asked on a hub with no Bolt origin head; want no question")
		}
	})
}
