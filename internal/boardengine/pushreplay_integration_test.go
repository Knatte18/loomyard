// pushreplay_integration_test.go covers the pushes that rest on the board's replay:
// a rejected push replayed over origin, and a push of a pair whose entry the board has ended or still holds.

package boardengine_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requireTrackingRefPrunedWithUpstreamSet fails the test unless the checkout at worktree holds no tracking ref of branch.
// The branch must still carry its upstream setting.
func requireTrackingRefPrunedWithUpstreamSet(t *testing.T, worktree, branch string) {
	t.Helper()

	if refs := gitkit.Git(t, worktree, "for-each-ref", "refs/remotes/origin/"+branch); refs != "" {
		t.Errorf("tracking refs of %s = %q; want none, the ref pruned", branch, refs)
	}
	if merge := gitkit.Git(t, worktree, "config", "--get", "branch."+branch+".merge"); merge == "" {
		t.Errorf("branch.%s.merge is empty; want the upstream setting kept", branch)
	}
}

// TestPush_BoardReplay drives the records pushes whose outcome rests on the board:
// its replay of a rejected push, and its reading of whether a pair's entry has ended.
func TestPush_BoardReplay(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	unbounded := fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded}
	owed := fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded, OwedUnrecorded: true}
	noSleep := fabricengine.SyncOptions{BackoffSleep: func(time.Duration) {}}
	// publishedRecords is a hub whose prime records branch is on origin with an upstream.
	publishedRecords := func(t *testing.T) (h *hubforge.Hub, records, branch string) {
		t.Helper()
		h = hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		records = h.PrimeRecords()
		branch = gitkit.CurrentBranch(t, records)
		gitkit.Git(t, records, "push", "--set-upstream", "origin", branch)
		return h, records, branch
	}
	type sides struct {
		location                *lymcwd.Location
		code, records           string
		codeBranch, recordsName string
	}
	// newPair is a hub with the pair slug, both of whose sides have been pushed.
	newPair := func(t *testing.T, slug string) (*hubforge.Hub, sides) {
		t.Helper()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, slug)
		p := sides{code: h.PairCodeWorktree(slug), records: h.PairRecordsSibling(slug)}
		location, err := lymcwd.ResolveWorktree(p.code)
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		p.location = location
		p.codeBranch = gitkit.CurrentBranch(t, p.code)
		p.recordsName = gitkit.CurrentBranch(t, p.records)
		if _, err := fabricengine.PushPairAnchored(location, noSleep, unbounded); err != nil {
			t.Fatalf("PushPairAnchored() priming push error = %v; want nil", err)
		}
		return h, p
	}
	endOnOrigin := func(t *testing.T, h *hubforge.Hub, slug string) {
		t.Helper()
		gitkit.CommitOnBare(t, h.RecordsBare, gitkit.CurrentBranch(t, h.BoardDir()), boardengine.BoardFileRel(), boardkit.BoardFile(entryJSON(slug, `,"status":"done"`)), slug+" done")
	}
	failedPushes := func(t *testing.T, h *hubforge.Hub) []fabriclayout.FailedPush {
		t.Helper()
		records, err := fabricengine.ListFailedPushes(h.Location.HubPath)
		if err != nil {
			t.Fatalf("ListFailedPushes: %v", err)
		}
		return records
	}
	tipOf := func(t *testing.T, bare, branch string) string {
		t.Helper()
		return gitkit.RevParse(t, bare, "refs/heads/"+branch)
	}

	t.Run("ADetachedRecordsPushRejectedAsNotFastForwardIsReplayedAndPushed", func(t *testing.T) {
		t.Parallel()
		h, records, branch := publishedRecords(t)
		gitkit.CommitOnBare(t, h.RecordsBare, branch, "other.txt", "other\n", "other machine")
		gitkit.CommitFile(t, records, "local.txt", "local\n", "local")

		if _, err := fabricengine.CoalescePushRequest(fabriclayout.PushSpawn{WeftPath: records}, noSleep, fabriclayout.MachineNames{}); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
		}

		if got, want := tipOf(t, h.RecordsBare, branch), gitkit.RevParse(t, records, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the replayed local HEAD %s", got, want)
		}
		if files := gitkit.LsFiles(t, records, "."); !slices.Contains(files, "other.txt") || !slices.Contains(files, "local.txt") {
			t.Errorf("tracked files = %v; want other.txt and local.txt, the replay's two sides", files)
		}
		if got := failedPushes(t, h); len(got) != 0 {
			t.Errorf("failed-push records = %+v; want none", got)
		}
	})

	t.Run("APrimeRecordsPushRejectedAfterThisMachinesFinalizeKeepsTheLandingsCommits", func(t *testing.T) {
		t.Parallel()
		h, records, branch := publishedRecords(t)
		hubforge.AddPair(t, h, "neve")
		pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("neve"))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		writeRunStatus(t, pair, "neve", "Land", "Finalize")
		writeRunStatus(t, h.Location, "neve", "Run-Shed")
		_, statusRel, err := shedrun.TreeRunFileRels("neve")
		if err != nil {
			t.Fatalf("TreeRunFileRels: %v", err)
		}
		gitkit.Git(t, records, "add", "--", statusRel)
		gitkit.Git(t, records, "commit", "--message", "neve prime status")
		gitkit.CommitOnBare(t, h.RecordsBare, branch, "notes/up.md", "up\n", "upstream work")
		endOnOrigin(t, h, "neve")

		if _, err := fabricengine.CoalescePushRequest(fabriclayout.PushSpawn{WeftPath: records}, noSleep, fabriclayout.MachineNames{}); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
		}

		if got := gitkit.Git(t, h.RecordsBare, "ls-tree", "--name-only", tipOf(t, h.RecordsBare, branch), statusRel); got != statusRel {
			t.Errorf("origin tip tracks %q; want %s, the landing's commit kept", got, statusRel)
		}
		if tags := gitkit.Git(t, records, "tag", "--list", "set-aside/*"); tags != "" {
			t.Errorf("set-aside tags = %q; want none", tags)
		}
		if got := failedPushes(t, h); len(got) != 0 {
			t.Errorf("failed-push records = %+v; want none", got)
		}
	})

	t.Run("ABoltPushRejectedOverAnUnrelatedOriginEntryIsReplayedAndLands", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		board := h.BoardDir()
		branch := gitkit.CurrentBranch(t, board)
		gitkit.CommitFile(t, board, boardengine.BoardFileRel(), boardkit.BoardFile(titledEntryJSON("a", "A", ""), `{"id":1,"slug":"b","title":"B","kind":"task","labels":["bug"],"issues":[],"depends_on":[]}`), "base board")
		gitkit.Git(t, board, "push", "origin", branch)
		second := func(title string) string {
			return fmt.Sprintf(`{"id":1,"slug":"b","title":%q,"kind":"task","labels":["bug"],"issues":[],"depends_on":[]}`, title)
		}
		gitkit.CommitFile(t, board, boardengine.BoardFileRel(), boardkit.BoardFile(titledEntryJSON("a", "A", ""), second("B2")), "retitle b")
		gitkit.CommitOnBare(t, h.RecordsBare, branch, boardengine.BoardFileRel(), boardkit.BoardFile(titledEntryJSON("a", "A2", ""), second("B")), "retitle a")

		if err := fabricengine.NewBolt(board).Push(noSleep); err != nil {
			t.Fatalf("Bolt.Push() error = %v; want nil", err)
		}

		if got, want := tipOf(t, h.RecordsBare, branch), gitkit.RevParse(t, board, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the replayed board tip %s", got, want)
		}
		tasks, err := boardengine.ParseBoardTasks([]byte(gitkit.Git(t, board, "show", "HEAD:"+boardengine.BoardFileRel())))
		if err != nil {
			t.Fatalf("ParseBoardTasks: %v", err)
		}
		titles := map[string]string{}
		for _, task := range tasks {
			titles[task.Slug] = task.Title
		}
		if titles["a"] != "A2" || titles["b"] != "B2" {
			t.Errorf("board titles = %v; want both machines' retitles", titles)
		}
		if got := failedPushes(t, h); len(got) != 0 {
			t.Errorf("failed-push records = %+v; want none", got)
		}
	})

	t.Run("ABoltPushRejectedWhileTheBoardLockIsHeldDefersWithADeferredRecord", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		board := h.BoardDir()
		branch := gitkit.CurrentBranch(t, board)
		gitkit.CommitFile(t, board, "notes/a.md", "a\n", "board change")
		gitkit.CommitOnBare(t, h.RecordsBare, branch, "notes/up.md", "up\n", "upstream work")
		boardLockPath, err := fabricengine.BoltLockPath(board, fabriclayout.BoardWriteLockFile)
		if err != nil {
			t.Fatalf("locate the board lock: %v", err)
		}
		boardLock, err := lock.AcquireWriteLock(boardLockPath)
		if err != nil {
			t.Fatalf("take the board lock: %v", err)
		}
		before := tipOf(t, h.RecordsBare, branch)

		if err := fabricengine.NewBolt(board).Push(noSleep); err != nil {
			t.Fatalf("Bolt.Push() with the board lock held error = %v; want nil", err)
		}

		if got := failedPushes(t, h); len(got) != 1 || got[0].Repo != "bolt" || got[0].Class != fabriclayout.PushFailureDeferred {
			t.Errorf("failed-push records = %+v; want one deferred bolt record", got)
		}
		if got := tipOf(t, h.RecordsBare, branch); got != before {
			t.Errorf("origin tip = %s; want it unchanged at %s", got, before)
		}
		if err := boardLock.Release(); err != nil {
			t.Fatalf("release the board lock: %v", err)
		}
		if err := fabricengine.NewBolt(board).Push(noSleep); err != nil {
			t.Fatalf("Bolt.Push() after the release error = %v; want nil", err)
		}
		if got, want := tipOf(t, h.RecordsBare, branch), gitkit.RevParse(t, board, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the board tip %s", got, want)
		}
		if got := failedPushes(t, h); len(got) != 0 {
			t.Errorf("failed-push records = %+v; want none", got)
		}
	})

	t.Run("AParkedPushOfABranchGoneOnOriginDoesNotRecreateIt", func(t *testing.T) {
		t.Parallel()
		t.Run("AnEntryNotEndedWritesAGoneRecordAfterTheBackoff", func(t *testing.T) {
			t.Parallel()
			h, p := newPair(t, "gonepair")
			gitkit.Git(t, h.RecordsBare, "branch", "-D", p.recordsName)
			gitkit.CommitFile(t, p.records, "local.txt", "local\n", "local")
			sleeps := 0

			_, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{BackoffSleep: func(time.Duration) { sleeps++ }}, unbounded)

			var failed *fabricengine.PushFailedError
			if !errors.As(err, &failed) || failed.Record.Class != fabriclayout.PushFailureGone || !errors.Is(err, fabricengine.ErrBranchGone) {
				t.Fatalf("PushPairAnchored() error = %v; want a gone *PushFailedError wrapping ErrBranchGone", err)
			}
			if !strings.HasSuffix(err.Error(), "way forward: "+fabricengine.FailedPushWayForward(failed.Record)) {
				t.Errorf("PushPairAnchored() error = %q; want it to end in the gone way forward", err)
			}
			if gitkit.BranchExists(t, h.RecordsBare, p.recordsName) {
				t.Errorf("origin holds %s again; want a parked push never to recreate it", p.recordsName)
			}
			if want := len(gitrepo.PushBackoffDelays()); sleeps != want {
				t.Errorf("backoff sleeps = %d; want %d", sleeps, want)
			}
			if got := failedPushes(t, h); len(got) != 1 || got[0].Repo != "records" || got[0].Class != fabriclayout.PushFailureGone {
				t.Errorf("failed-push records = %+v; want one gone records record", got)
			}
		})

		// The done mark is either on origin before the push or arrives during its backoff.
		tests := []struct {
			name          string
			duringBackoff bool
			wantSleeps    int
		}{
			{"AnEntryEndedOnOriginSinceTheLastBoltFetchWritesNoRecord", false, 0},
			{"TheDoneMarkArrivingDuringTheBackoffEndsItWithNoRecord", true, 1},
		}
		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				slug := fmt.Sprintf("endedpair%d", i)
				h, p := newPair(t, slug)
				gitkit.Git(t, h.RecordsBare, "branch", "-D", p.recordsName)
				if !tt.duringBackoff {
					endOnOrigin(t, h, slug)
				}
				gitkit.CommitFile(t, p.records, "local.txt", "local\n", "local")
				sleeps := 0
				backoff := func(time.Duration) {
					sleeps++
					if tt.duringBackoff {
						endOnOrigin(t, h, slug)
					}
				}

				_, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{BackoffSleep: backoff}, unbounded)

				if err == nil || !strings.Contains(err.Error(), "ended on origin") {
					t.Fatalf("PushPairAnchored() error = %v; want one naming the ended entry, whose branch is left to its teardown", err)
				}
				if sleeps != tt.wantSleeps {
					t.Errorf("backoff sleeps = %d; want %d", sleeps, tt.wantSleeps)
				}
				if gitkit.BranchExists(t, h.RecordsBare, p.recordsName) {
					t.Errorf("origin holds %s again; want it left gone", p.recordsName)
				}
				if got := failedPushes(t, h); len(got) != 0 {
					t.Errorf("failed-push records = %+v; want none", got)
				}
			})
		}

		t.Run("AnUnreachableOriginRestoredDuringTheBackoffRecordsNothingAndPushes", func(t *testing.T) {
			t.Parallel()
			h, p := newPair(t, "restorepair")
			realURL := gitkit.Git(t, p.records, "remote", "get-url", "origin")
			gitkit.CommitFile(t, p.records, "local.txt", "local\n", "local")
			gitkit.Git(t, p.records, "remote", "set-url", "origin", "http://127.0.0.1:1/x")
			sleeps := 0
			backoff := func(time.Duration) {
				sleeps++
				gitkit.Git(t, p.records, "remote", "set-url", "origin", realURL)
			}

			if _, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{BackoffSleep: backoff}, unbounded); err != nil {
				t.Fatalf("PushPairAnchored() error = %v; want nil", err)
			}

			if got, want := tipOf(t, h.RecordsBare, p.recordsName), gitkit.RevParse(t, p.records, "HEAD"); got != want {
				t.Errorf("origin tip = %s; want the pushed local HEAD %s", got, want)
			}
			if sleeps != 1 {
				t.Errorf("backoff sleeps = %d; want 1", sleeps)
			}
			if got := failedPushes(t, h); len(got) != 0 {
				t.Errorf("failed-push records = %+v; want none", got)
			}
		})
	})

	// With no ledger entry, a landing commit of the pair's run past origin on its parent holds the records back.
	// The hold lasts until the run's entry is done, and the code side still goes.
	t.Run("ALandingCommitPastOriginHoldsTheRecordsUntilTheRunIsDone", func(t *testing.T) {
		t.Parallel()
		h, p := newPair(t, "pushpair")
		gitkit.Git(t, h.PrimeWorktree(), "push", "-q", "-u", "origin", "HEAD")
		gitkit.CommitFile(t, h.PrimeWorktree(), "pushpair.txt", "pushpair\n", "land pushpair\n\n"+fabriclayout.LandingTrailerKey+": pushpair")
		recordsBefore := tipOf(t, h.RecordsBare, p.recordsName)
		recordsSHA := gitkit.CommitFile(t, p.records, "records-file.txt", "records change", "records change")
		codeSHA := gitkit.CommitFile(t, p.code, "code-file.txt", "code change", "code change")
		push := func() {
			t.Helper()
			if _, err := fabricengine.PushPairAnchored(p.location, noSleep, unbounded); err != nil {
				t.Fatalf("PushPairAnchored() error = %v; want nil", err)
			}
		}

		push()
		if got := tipOf(t, h.RecordsBare, p.recordsName); got != recordsBefore {
			t.Errorf("records bare = %q; want it held at %q", got, recordsBefore)
		}
		if got := tipOf(t, h.CodeBare, p.codeBranch); got != codeSHA {
			t.Errorf("code bare = %q; want local HEAD %q", got, codeSHA)
		}

		boardkit.WriteBoard(t, fabriclayout.BoardDir(h.Location.HubPath), `{"id":1,"slug":"pushpair","title":"t","status":"done"}`)
		push()
		if got := tipOf(t, h.RecordsBare, p.recordsName); got != recordsSHA {
			t.Errorf("records bare with the run done = %q; want local HEAD %q", got, recordsSHA)
		}
	})

	// A records branch deleted on origin whose entry origin's board ended is left to its teardown:
	// the error wraps ErrEntryEnded, and the branch's tracking ref is deleted with its upstream setting kept.
	t.Run("OwedUnrecorded_ARecordsBranchOfAnEndedEntryDeletedOnOriginDropsItsTrackingRef", func(t *testing.T) {
		t.Parallel()
		h, p := newPair(t, "endedrecords")
		gitkit.Git(t, h.RecordsBare, "branch", "-D", p.recordsName)
		endOnOrigin(t, h, "endedrecords")
		gitkit.CommitFile(t, p.records, "local.txt", "local\n", "local")

		_, err := fabricengine.PushPairAnchored(p.location, noSleep, owed)

		if !errors.Is(err, fabricengine.ErrEntryEnded) || fabricengine.IsPushOwed(err) {
			t.Fatalf("PushPairAnchored() error = %v; want one wrapping ErrEntryEnded that is not owed", err)
		}
		requireTrackingRefPrunedWithUpstreamSet(t, p.records, p.recordsName)
		if got := failedPushes(t, h); len(got) != 0 {
			t.Errorf("failed-push records = %+v; want none", got)
		}
	})

	// A code branch deleted on origin whose entry origin's board ended is skipped with no error,
	// and its tracking ref is deleted; a later push of the pair finds the branch gone and skips it again.
	t.Run("OwedUnrecorded_ACodeBranchOfAnEndedEntryDeletedOnOriginIsSkippedTwice", func(t *testing.T) {
		t.Parallel()
		h, p := newPair(t, "endedcode")
		gitkit.Git(t, h.CodeBare, "branch", "-D", p.codeBranch)
		endOnOrigin(t, h, "endedcode")
		gitkit.CommitFile(t, p.code, "code-file.txt", "code\n", "code change")

		if _, err := fabricengine.PushPairAnchored(p.location, noSleep, owed); err != nil {
			t.Fatalf("first push error = %v; want nil", err)
		}

		requireTrackingRefPrunedWithUpstreamSet(t, p.code, p.codeBranch)
		gitkit.CommitFile(t, p.records, "records-file.txt", "records\n", "records change")
		if _, err := fabricengine.PushPairAnchored(p.location, noSleep, owed); err != nil {
			t.Fatalf("second push after a new records commit error = %v; want nil", err)
		}
		if gitkit.BranchExists(t, h.CodeBare, p.codeBranch) {
			t.Errorf("origin holds %s again; want it left gone", p.codeBranch)
		}
		if got := failedPushes(t, h); len(got) != 0 {
			t.Errorf("failed-push records = %+v; want none", got)
		}
	})
}
