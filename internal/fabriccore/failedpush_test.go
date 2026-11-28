// failedpush_test.go tests the rules of the failed-push record: the merge, the notice, the way forward and the filters.

package fabriccore_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFailedPushWayForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boltPath := "/hub/" + fabriclayout.BoltDirName
	// The anchored hub's lym verbs run in anchoredDir, never at anchoredTask.
	const anchoredTask, anchoredDir, anchoredWeft = "/anchored/task", "/anchored/task/src/app", "/anchored/task-weft"
	anchored := fabriccore.VerbDirs{anchoredTask: anchoredDir}

	tests := []struct {
		name    string
		failure fabriclayout.FailedPush
		dirs    fabriccore.VerbDirs
		want    string
	}{
		{
			"CodeDeferred",
			fabriclayout.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabriclayout.PushFailureDeferred},
			nil,
			"run `lym fabric push` in /hub/task to retry the push",
		},
		{
			"RecordsDeferred",
			fabriclayout.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabriclayout.PushFailureDeferred},
			nil,
			"run `lym fabric push` in /hub/task to retry the push",
		},
		{
			"BoltDeferred",
			fabriclayout.FailedPush{Repo: "bolt", RepoPath: boltPath, Branch: "main", Class: fabriclayout.PushFailureDeferred},
			nil,
			"run `lym board sync` from the prime worktree to retry the push",
		},
		{
			"RecordsTeardownLock",
			fabriclayout.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabriclayout.PushFailureTeardownLock},
			nil,
			"the push retries once the removal or push holding the pair's teardown lock lets go, so run `lym fabric push` in /hub/task again then, " +
				"or end the stuck child whose pid `lym tender status` lists",
		},
		{
			"CodeAuth",
			fabriclayout.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabriclayout.PushFailureAuth},
			nil,
			"fix this machine's permission to push task on origin, then run `lym fabric push` in /hub/task",
		},
		{
			"CodeNotFastForward",
			fabriclayout.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabriclayout.PushFailureNotFastForward},
			nil,
			"origin/task holds commits this branch lacks; run `git merge origin/task` in /hub/task, settle any conflict and commit, then run `lym fabric push` in /hub/task",
		},
		{
			"RecordsNotFastForward",
			fabriclayout.FailedPush{
				Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabriclayout.PushFailureNotFastForward,
				Paths: []string{"_lym/a.md"}, Commits: []string{"abc123"},
			},
			nil,
			"origin/task-weft holds commits that change the same paths as this branch's (paths: _lym/a.md; local commits: abc123); " +
				"to keep origin's side, run `git reset --keep origin/task-weft` in /hub/task-weft, which drops the local commits; " +
				"to keep this machine's, run `git merge origin/task-weft` in /hub/task-weft, settle the named paths and commit, then run `lym fabric push` in /hub/task, whose push is then a fast-forward and meets no replay",
		},
		{
			"RecordsNotFastForwardWithNoConflictingPathsNamesNone",
			fabriclayout.FailedPush{
				Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabriclayout.PushFailureNotFastForward,
				Commits: []string{"abc123"},
			},
			nil,
			"origin/task-weft holds commits this branch lacks (local commits: abc123); " +
				"to keep origin's side, run `git reset --keep origin/task-weft` in /hub/task-weft, which drops the local commits; " +
				"to keep this machine's, run `git merge origin/task-weft` in /hub/task-weft, settle any conflict and commit, then run `lym fabric push` in /hub/task, whose push is then a fast-forward and meets no replay",
		},
		{
			"CodeGone",
			fabriclayout.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabriclayout.PushFailureGone},
			nil,
			"origin no longer has branch task, and its board entry is neither done nor removed on origin, since an ended entry writes no record; " +
				"decide whether the run ends: `lym fabric remove` of the pair at /hub/task, run from the prime, removes the pair and clears this record; " +
				"once origin's board shows the entry done or removed, `lym cleanup --apply` tears it down instead",
		},
		{
			"RecordsGoneNamesTheCodeWorktree",
			fabriclayout.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabriclayout.PushFailureGone},
			nil,
			"origin no longer has branch task-weft, and its board entry is neither done nor removed on origin, since an ended entry writes no record; " +
				"decide whether the run ends: `lym fabric remove` of the pair at /hub/task, run from the prime, removes the pair and clears this record; " +
				"once origin's board shows the entry done or removed, `lym cleanup --apply` tears it down instead",
		},
		{
			"PrimeCodeTransportNamesGitPush",
			fabriclayout.FailedPush{Repo: "code", RepoPath: "/hub/main", Branch: "main", Prime: true, Class: fabriclayout.PushFailureTransport},
			nil,
			"run `git push` in /hub/main to retry the push",
		},
		{
			"PrimeCodeNotFastForwardMergesAndPushesInThePrime",
			fabriclayout.FailedPush{Repo: "code", RepoPath: "/hub/main", Branch: "main", Prime: true, Class: fabriclayout.PushFailureNotFastForward},
			nil,
			"origin/main holds commits this branch lacks; run `git merge origin/main` in /hub/main, settle any conflict and commit, then run `git push` in /hub/main",
		},
		{
			"AnchoredCodeNotFastForwardMergesAtTheRootAndPushesInTheAnchoredDir",
			fabriclayout.FailedPush{Repo: "code", RepoPath: anchoredTask, Branch: "task", Class: fabriclayout.PushFailureNotFastForward},
			anchored,
			"origin/task holds commits this branch lacks; run `git merge origin/task` in " + anchoredTask + ", settle any conflict and commit, then run `lym fabric push` in " + anchoredDir,
		},
		{
			"AnchoredCodeRewoundResetsAtTheRootAndPushesInTheAnchoredDir",
			fabriclayout.FailedPush{Repo: "code", RepoPath: anchoredTask, Branch: "task", Class: fabriclayout.PushFailureRewound, OriginHead: "abc123", Commits: []string{"def456"}},
			anchored,
			"origin rewound task to abc123, dropping def456; " +
				"to accept the rewind, run `git reset --keep abc123` in " + anchoredTask + ", which drops the local commits past origin's head, then `lym fabric push` in " + anchoredDir + "; " +
				"to restore the dropped commits, run `git push origin task` in " + anchoredTask + ", a plain fast-forward",
		},
	}
	for _, tt := range tests {
		t.Run("WayForward_"+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabriccore.FailedPushWayForward(tt.failure, tt.dirs); got != tt.want {
				t.Errorf("FailedPushWayForward(%s) =\n%s\nwant\n%s", tt.name, got, tt.want)
			}
		})
	}

	stops := []struct {
		name    string
		outcome fabriclayout.BranchOutcome
		want    string
	}{
		{
			"RewrittenCodeBranchPullsInTheAnchoredDir",
			fabriclayout.BranchOutcome{Repo: "code", Worktree: anchoredTask, CodeWorktree: anchoredTask, State: fabriclayout.BranchRewritten},
			"`lym fabric pull` in " + anchoredDir,
		},
		{
			"AnchorStoppedPrimeWeftSetsTheAnchorInTheAnchoredDir",
			fabriclayout.BranchOutcome{Repo: "records", Worktree: anchoredWeft, CodeWorktree: anchoredTask, State: fabriclayout.BranchAnchorStopped, Anchor: "sub"},
			"end every reed session of the hub, the orch's included, with `lym reed down` in each, and wait until no batten run of the hub is live; " +
				"then run `lym fabric anchor --set sub` in " + anchoredDir + "; then run `lym beam start` again",
		},
		{
			"DirtyRecordsBranchCommitsInTheAnchoredDirAndRestoresAtTheRecordsRoot",
			fabriclayout.BranchOutcome{Repo: "records", Worktree: anchoredWeft, CodeWorktree: anchoredTask, State: fabriclayout.BranchDirty, Paths: []string{"notes/a.md"}},
			"commit the uncommitted paths (notes/a.md) with `lym fabric commit` in " + anchoredDir + ", or any of them with `git commit` or `git checkout -- <path>` in " + anchoredWeft,
		},
		{
			"DivergedPrimeCodeBranchMergesAndPushesByGit",
			fabriclayout.BranchOutcome{Repo: "code", Worktree: "/hub/main", CodeWorktree: "/hub/main", Branch: "main", Prime: true, State: fabriclayout.BranchDiverged},
			"origin/main holds commits this branch lacks; run `git merge origin/main` in /hub/main, settle any conflict and commit, then run `git push` in /hub/main",
		},
		{
			"GoneBranchOfARunAsksWhetherTheRunEnds",
			fabriclayout.BranchOutcome{Repo: "code", Worktree: "/hub/task", CodeWorktree: "/hub/task", Branch: "task", Slug: "task", State: fabriclayout.BranchGone},
			"origin no longer has branch task, and its board entry is neither done nor removed on origin, since an ended entry writes no record; " +
				"decide whether the run ends: `lym fabric remove` of the pair at /hub/task, run from the prime, removes the pair and clears this record; " +
				"once origin's board shows the entry done or removed, `lym cleanup --apply` tears it down instead",
		},
		{
			"GoneBranchOfNoRunIsPublishedAgain",
			fabriclayout.BranchOutcome{Repo: "code", Worktree: "/hub/main", CodeWorktree: "/hub/main", Branch: "main", Prime: true, State: fabriclayout.BranchGone},
			"origin no longer has branch main; run `git push` in /hub/main to publish it again",
		},
	}
	for _, tt := range stops {
		t.Run("StopWayForward_"+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabriccore.BranchStopWayForward(tt.outcome, anchored, ""); got != tt.want {
				t.Errorf("BranchStopWayForward(%s) =\n%s\nwant\n%s", tt.name, got, tt.want)
			}
		})
	}

	heldBack := []struct {
		name     string
		prime    bool
		wantStep string
	}{
		{"ThePrimesCodeBranchIsPushedByGit", true, "`git push` in /hub/parent"},
		{"ABeamParentIsPushedByThePairPush", false, "`lym fabric push` in /hub/parent"},
	}
	for _, tt := range heldBack {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			record := fabriclayout.FailedPush{
				Repo:       fabriclayout.RepoCode,
				RepoPath:   "/hub/parent",
				Branch:     "parent",
				Prime:      tt.prime,
				Class:      fabriclayout.PushFailureHeldBack,
				WayForward: "run `lym adopt root`, then push",
			}

			if got := fabriccore.FailedPushWayForward(record, nil); got != record.WayForward {
				t.Errorf("FailedPushWayForward() = %q; want the record's own %q", got, record.WayForward)
			}
			if got := fabriccore.PushStep(record, nil); got != tt.wantStep {
				t.Errorf("PushStep() = %q; want %q", got, tt.wantStep)
			}
		})
	}
}

func TestMergeFailedPush(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	earlier := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	now := earlier.Add(time.Minute)
	transport := fabriclayout.FailedPush{Branch: "task", Class: fabriclayout.PushFailureTransport}

	tests := []struct {
		name       string
		current    fabriclayout.FailedPush
		found      bool
		next       fabriclayout.FailedPush
		want       fabriclayout.FailedPush
		wantNotify bool
	}{
		{
			name:       "FirstFailure",
			next:       transport,
			want:       fabriclayout.FailedPush{Branch: "task", Class: fabriclayout.PushFailureTransport, FirstSeen: now, LastSeen: now, Count: 1},
			wantNotify: true,
		},
		{
			name:    "LaterFailureKeepsFirstSeenAndNotifiedAndBumpsTheCount",
			current: fabriclayout.FailedPush{FirstSeen: earlier, LastSeen: earlier, Count: 1, Notified: true},
			found:   true,
			next:    transport,
			want:    fabriclayout.FailedPush{Branch: "task", Class: fabriclayout.PushFailureTransport, FirstSeen: earlier, LastSeen: now, Count: 2, Notified: true},
		},
		{
			name:       "RunsUnionExistingFirst",
			current:    fabriclayout.FailedPush{FirstSeen: earlier, Count: 2, Runs: []string{"first", "second"}},
			found:      true,
			next:       fabriclayout.FailedPush{Class: fabriclayout.PushFailureTransport, Runs: []string{"third", "first"}},
			want:       fabriclayout.FailedPush{Class: fabriclayout.PushFailureTransport, FirstSeen: earlier, LastSeen: now, Count: 3, Runs: []string{"first", "second", "third"}},
			wantNotify: true,
		},
		{
			// The record names local commits, so a deferral carrying commits sends no notice either.
			name: "DeferredNeverNotifies",
			next: fabriclayout.FailedPush{Class: fabriclayout.PushFailureDeferred, Commits: []string{"aaa111"}},
			want: fabriclayout.FailedPush{Class: fabriclayout.PushFailureDeferred, Commits: []string{"aaa111"}, FirstSeen: now, LastSeen: now, Count: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, notify := fabriccore.MergeFailedPush(tt.current, tt.found, tt.next, now)
			if !reflect.DeepEqual(got, tt.want) || notify != tt.wantNotify {
				t.Errorf("MergeFailedPush() = %+v, %v; want %+v, %v", got, notify, tt.want, tt.wantNotify)
			}
		})
	}

	notices := []struct {
		name    string
		failure fabriclayout.FailedPush
		want    fabriclayout.PushNotice
	}{
		{
			"PairCodeBranchRunsAsTheWorktreeBaseName",
			fabriclayout.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabriclayout.PushFailureTransport, Error: "connection refused"},
			fabriclayout.PushNotice{
				Line:  "a push of code branch task failed (transport): connection refused; way forward: run `lym fabric push` in /hub/task to retry the push",
				Topic: "push/code/task",
				Run:   "task",
			},
		},
		{
			"RecordsBranchRunsAsTheCodeWorktreeBaseName",
			fabriclayout.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabriclayout.PushFailureTransport, Error: "connection refused"},
			fabriclayout.PushNotice{
				Line:  "a push of records branch task-weft failed (transport): connection refused; way forward: run `lym fabric push` in /hub/task to retry the push",
				Topic: "push/records/task-weft",
				Run:   "task",
			},
		},
		{
			"PrimeCodeBranchHasNoRun",
			fabriclayout.FailedPush{Repo: "code", RepoPath: "/hub/main", Branch: "main", Prime: true, Class: fabriclayout.PushFailureTransport, Error: "connection refused"},
			fabriclayout.PushNotice{
				Line:  "a push of code branch main failed (transport): connection refused; way forward: run `git push` in /hub/main to retry the push",
				Topic: "push/code/main",
			},
		},
	}
	for _, tt := range notices {
		t.Run("Notice_"+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabriccore.FailedPushNotice(tt.failure, nil); got != tt.want {
				t.Errorf("FailedPushNotice() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestFailedPushFilters(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("OwedClasses", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			class fabriclayout.PushFailureClass
			want  bool
		}{
			{fabriclayout.PushFailureTransport, true},
			{fabriclayout.PushFailureDeferred, true},
			{fabriclayout.PushFailureTeardownLock, true},
			{fabriclayout.PushFailureNotFastForward, false},
			{fabriclayout.PushFailureAuth, false},
			{fabriclayout.PushFailureOther, false},
		}
		for _, tt := range tests {
			if got := fabriccore.IsOwedClass(tt.class); got != tt.want {
				t.Errorf("IsOwedClass(%q) = %v; want %v", tt.class, got, tt.want)
			}
		}
	})

	t.Run("OwedByListsTheRunsOwedClassesNewestFirst", func(t *testing.T) {
		t.Parallel()
		base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		records := []fabriclayout.FailedPush{
			{Branch: "task", Class: fabriclayout.PushFailureTransport, Runs: []string{"task"}},
			{Branch: "task-weft", Class: fabriclayout.PushFailureDeferred, Runs: []string{"task"}},
			{Branch: "main-weft", Class: fabriclayout.PushFailureTeardownLock, Runs: []string{"task", "other"}},
			{Branch: "diverged", Class: fabriclayout.PushFailureNotFastForward, Runs: []string{"task"}},
			{Branch: "only-other", Class: fabriclayout.PushFailureTransport, Runs: []string{"other"}},
			{Branch: "no-run", Class: fabriclayout.PushFailureTransport},
		}
		for i := range records {
			records[i].LastSeen = base.Add(time.Duration(i) * time.Minute)
		}

		for slug, want := range map[string][]string{
			"task":  {"main-weft", "task-weft", "task"},
			"other": {"only-other", "main-weft"},
			"ghost": nil,
		} {
			var got []string
			for _, record := range fabriccore.OwedBy(records, slug) {
				got = append(got, record.Branch)
			}
			if !slices.Equal(got, want) {
				t.Errorf("OwedBy(%q) branches = %q; want %q", slug, got, want)
			}
		}
	})

	t.Run("RefusedPushStands", func(t *testing.T) {
		t.Parallel()
		record := func(repo, branch string, class fabriclayout.PushFailureClass) []fabriclayout.FailedPush {
			return []fabriclayout.FailedPush{{Repo: repo, Branch: branch, Class: class}}
		}
		tests := []struct {
			name       string
			records    []fabriclayout.FailedPush
			codeBranch string
			want       bool
		}{
			{"NoRecord", nil, "task", false},
			{"RefusedRecordOfTheRecordsBranch", record("records", "task-weft", fabriclayout.PushFailureNotFastForward), "task", true},
			{"RefusedRecordOfTheCodeBranch", record("code", "task", fabriclayout.PushFailureNotFastForward), "task", true},
			{"OwedRecordOfTheRecordsBranch", record("records", "task-weft", fabriclayout.PushFailureTransport), "task", false},
			{"RecordOfAnotherBranch", record("records", "other-weft", fabriclayout.PushFailureNotFastForward), "task", false},
			{"CodeRecordWithNoCodeBranch", record("code", "task", fabriclayout.PushFailureNotFastForward), "", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if got := fabriccore.RefusedPushStands(tt.records, "task-weft", tt.codeBranch); got != tt.want {
					t.Errorf("RefusedPushStands() = %v; want %v", got, tt.want)
				}
			})
		}
	})
}
