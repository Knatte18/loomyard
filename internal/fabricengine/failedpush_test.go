// failedpush_test.go covers the failed-push record store: its once-per-episode notice and its way-forward lines.

package fabricengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

func composedPushLine(f fabricengine.FailedPush) string {
	return fmt.Sprintf("a push of %s branch %s failed (%s): %s; way forward: %s", f.Repo, f.Branch, f.Class, f.Error, fabricengine.FailedPushWayForward(f))
}

func codePushFailure() fabricengine.FailedPush {
	return fabricengine.FailedPush{
		Repo:     "code",
		RepoPath: "/hub/task",
		Branch:   "task",
		Class:    fabricengine.PushFailureTransport,
		Error:    "connection refused",
	}
}

// TestFailedPushRecord covers the record's lifecycle and its single notice per episode.
// It also checks the way-forward line that each side and class of push writes.
func TestFailedPushRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("FirstFailureMailsOnceAndLaterOnesOnlyUpdate", func(t *testing.T) {
		hub := t.TempDir()
		fake := &fakePushNotifier{hub: hub, posted: true}
		failure := codePushFailure()

		if err := fabricengine.RecordPushFailureNotifyingForTest(hub, failure, base, fake.notify); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}
		if err := fabricengine.RecordPushFailureNotifyingForTest(hub, failure, base.Add(time.Minute), fake.notify); err != nil {
			t.Fatalf("RecordPushFailure second: %v", err)
		}

		if len(fake.lines) != 1 || fake.lines[0] != composedPushLine(failure) {
			t.Errorf("notifier lines = %q; want one line %q", fake.lines, composedPushLine(failure))
		}
		records, err := fabricengine.ListFailedPushes(hub)
		if err != nil || len(records) != 1 {
			t.Fatalf("ListFailedPushes() = %v, %v; want one record", records, err)
		}
		got := records[0]
		if got.Count != 2 || !got.Notified || !got.FirstSeen.Equal(base) || !got.LastSeen.Equal(base.Add(time.Minute)) {
			t.Errorf("record = count %d notified %v first %v last %v; want 2 true %v %v", got.Count, got.Notified, got.FirstSeen, got.LastSeen, base, base.Add(time.Minute))
		}
	})

	noticeTagRows := []struct {
		name    string
		failure fabricengine.FailedPush
		want    fabricengine.PushNotice
	}{
		{
			"PairCodeBranchRunsAsTheWorktreeBaseName",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureTransport, Error: "connection refused"},
			fabricengine.PushNotice{Topic: "push/code/task", Run: "task"},
		},
		{
			"RecordsBranchRunsAsTheCodeWorktreeBaseName",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureTransport, Error: "connection refused"},
			fabricengine.PushNotice{Topic: "push/records/task-weft", Run: "task"},
		},
		{
			"PrimeCodeBranchHasNoRun",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/main", Branch: "main", Prime: true, Class: fabricengine.PushFailureTransport, Error: "connection refused"},
			fabricengine.PushNotice{Topic: "push/code/main"},
		},
		{
			"RecordsBranchWithNoPairedCodeWorktreeHasNoRun",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/stray", Branch: "stray", Class: fabricengine.PushFailureTransport, Error: "connection refused"},
			fabricengine.PushNotice{Topic: "push/records/stray"},
		},
		{
			"BoltBranchHasNoRun",
			fabricengine.FailedPush{Repo: "bolt", RepoPath: "/hub/" + fabricengine.BoltDirName, Branch: "main", Class: fabricengine.PushFailureTransport, Error: "connection refused"},
			fabricengine.PushNotice{Topic: "push/bolt/main"},
		},
	}
	for _, tt := range noticeTagRows {
		t.Run("NoticeTags_"+tt.name, func(t *testing.T) {
			hub := t.TempDir()
			fake := &fakePushNotifier{hub: hub, posted: true}

			if err := fabricengine.RecordPushFailureNotifyingForTest(hub, tt.failure, base, fake.notify); err != nil {
				t.Fatalf("RecordPushFailure: %v", err)
			}

			want := tt.want
			want.Line = composedPushLine(tt.failure)
			if len(fake.notices) != 1 || fake.notices[0] != want {
				t.Errorf("notices = %+v; want one %+v", fake.notices, want)
			}
		})
	}

	t.Run("NotPostedLeavesNotifiedUnsetSoTheNextFailureMails", func(t *testing.T) {
		hub := t.TempDir()
		fake := &fakePushNotifier{hub: hub, posted: false}

		for i := range 2 {
			if err := fabricengine.RecordPushFailureNotifyingForTest(hub, codePushFailure(), base.Add(time.Duration(i)*time.Minute), fake.notify); err != nil {
				t.Fatalf("RecordPushFailure: %v", err)
			}
		}

		if len(fake.lines) != 2 {
			t.Errorf("notifier calls = %d; want 2", len(fake.lines))
		}
		records, _ := fabricengine.ListFailedPushes(hub)
		if len(records) != 1 || records[0].Notified {
			t.Errorf("records = %+v; want one unnotified record", records)
		}
	})

	t.Run("NotifierErrorLeavesTheRecordWrittenAndUnnotified", func(t *testing.T) {
		hub := t.TempDir()
		fake := &fakePushNotifier{hub: hub, posted: true, err: errors.New("creel down")}

		for i := range 2 {
			if err := fabricengine.RecordPushFailureNotifyingForTest(hub, codePushFailure(), base.Add(time.Duration(i)*time.Minute), fake.notify); err != nil {
				t.Fatalf("RecordPushFailure: %v", err)
			}
		}

		if len(fake.lines) != 2 {
			t.Errorf("notifier calls = %d; want 2", len(fake.lines))
		}
		records, _ := fabricengine.ListFailedPushes(hub)
		if len(records) != 1 || records[0].Notified || records[0].Count != 2 {
			t.Errorf("records = %+v; want one unnotified record counted twice", records)
		}
	})

	t.Run("DeferredWritesTheRecordAndNeverMails", func(t *testing.T) {
		hub := t.TempDir()
		fake := &fakePushNotifier{hub: hub, posted: true}
		failure := codePushFailure()
		failure.Class = fabricengine.PushFailureDeferred

		if err := fabricengine.RecordPushFailureNotifyingForTest(hub, failure, base, fake.notify); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}

		if len(fake.lines) != 0 {
			t.Errorf("notifier lines = %q; want none", fake.lines)
		}
		records, _ := fabricengine.ListFailedPushes(hub)
		if len(records) != 1 || records[0].Notified {
			t.Errorf("records = %+v; want one unnotified record", records)
		}
	})

	t.Run("ClearRemovesTheRecordAndASecondClearIsNil", func(t *testing.T) {
		hub := t.TempDir()
		failure := codePushFailure()
		if err := fabricengine.RecordPushFailure(hub, failure, base); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}

		for i := range 2 {
			if err := fabricengine.ClearPushFailure(hub, failure.Repo, failure.Branch); err != nil {
				t.Errorf("ClearPushFailure #%d = %v; want nil", i+1, err)
			}
		}

		if records, _ := fabricengine.ListFailedPushes(hub); len(records) != 0 {
			t.Errorf("ListFailedPushes() = %+v; want none", records)
		}
	})

	t.Run("ListOverAnAbsentDirectoryIsEmptyAndSortsByRepoAndBranch", func(t *testing.T) {
		hub := t.TempDir()
		if records, err := fabricengine.ListFailedPushes(hub); err != nil || len(records) != 0 {
			t.Errorf("ListFailedPushes(empty) = %v, %v; want none, nil", records, err)
		}

		for _, entry := range []struct{ repo, branch string }{{"records", "b"}, {"code", "feature/x"}, {"code", "a"}} {
			failure := codePushFailure()
			failure.Repo, failure.Branch = entry.repo, entry.branch
			if err := fabricengine.RecordPushFailure(hub, failure, base); err != nil {
				t.Fatalf("RecordPushFailure: %v", err)
			}
		}

		records, err := fabricengine.ListFailedPushes(hub)
		if err != nil {
			t.Fatalf("ListFailedPushes: %v", err)
		}
		var got []string
		for _, record := range records {
			got = append(got, record.Repo+":"+record.Branch)
		}
		if want := "code:a code:feature/x records:b"; strings.Join(got, " ") != want {
			t.Errorf("ListFailedPushes order = %q; want %q", strings.Join(got, " "), want)
		}
	})

	t.Run("UnknownKeySurvivesAnUpdate", func(t *testing.T) {
		hub := t.TempDir()
		failure := codePushFailure()
		if err := fabricengine.RecordPushFailure(hub, failure, base); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}
		dir := filepath.Join(fabricengine.HubScratchDir(hub), lymdirs.FailedPushDirName)
		matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		if len(matches) != 1 {
			t.Fatalf("record files = %v; want one", matches)
		}
		raw, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		edited := strings.Replace(string(raw), "{", `{"future_key": "kept",`, 1)
		if err := os.WriteFile(matches[0], []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := fabricengine.RecordPushFailure(hub, failure, base.Add(time.Minute)); err != nil {
			t.Fatalf("RecordPushFailure second: %v", err)
		}

		after, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(after), `"future_key"`) || !strings.Contains(string(after), `"kept"`) {
			t.Errorf("record file after update = %s; want the unknown key kept", after)
		}
	})

	// The anchored hub's task worktree records the anchor src/app.
	// Its lym verbs run in anchoredDir, never at anchoredTask.
	anchoredHub := t.TempDir()
	anchoredTask := filepath.Join(anchoredHub, "task")
	anchoredDir := filepath.Join(anchoredTask, "src", "app")
	anchoredWeft := weftname.SiblingPath(anchoredHub, "task")
	if err := os.MkdirAll(anchoredWeft, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(anchoredWeft, lymcwd.AnchorFileName), []byte("src/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The invalid-marker hub's task worktree records a marker that escapes the worktree, so its verbs run at the root.
	invalidHub := t.TempDir()
	invalidTask := filepath.Join(invalidHub, "task")
	invalidWeft := weftname.SiblingPath(invalidHub, "task")
	if err := os.MkdirAll(invalidWeft, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidWeft, lymcwd.AnchorFileName), []byte("../escape\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wayForwardRows := []struct {
		name    string
		failure fabricengine.FailedPush
		want    string
	}{
		{
			"CodeDeferred",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureDeferred},
			"run `lym fabric push` in /hub/task to retry the push",
		},
		{
			"RecordsDeferred",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureDeferred},
			"run `lym fabric push` in /hub/task to retry the push",
		},
		{
			"BoltDeferred",
			fabricengine.FailedPush{Repo: "bolt", RepoPath: "/hub/" + fabricengine.BoltDirName, Branch: "main", Class: fabricengine.PushFailureDeferred},
			"run `lym board sync` from the prime worktree to retry the push",
		},
		{
			"CodeTransport",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureTransport},
			"run `lym fabric push` in /hub/task to retry the push",
		},
		{
			"RecordsTransport",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureTransport},
			"run `lym fabric push` in /hub/task to retry the push",
		},
		{
			"PrimeRecordsTransportPushesInThePrimeWorktree",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/main-weft", CodeWorktree: "/hub/main", Branch: "main-weft", Class: fabricengine.PushFailureTransport},
			"run `lym fabric push` in /hub/main to retry the push",
		},
		{
			"BoltTransport",
			fabricengine.FailedPush{Repo: "bolt", RepoPath: "/hub/" + fabricengine.BoltDirName, Branch: "main", Class: fabricengine.PushFailureTransport},
			"run `lym board sync` from the prime worktree to retry the push",
		},
		{
			"CodeAuth",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureAuth},
			"fix this machine's permission to push task on origin, then run `lym fabric push` in /hub/task",
		},
		{
			"RecordsAuth",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureAuth},
			"fix this machine's permission to push task-weft on origin, then run `lym fabric push` in /hub/task",
		},
		{
			"BoltAuth",
			fabricengine.FailedPush{Repo: "bolt", RepoPath: "/hub/" + fabricengine.BoltDirName, Branch: "main", Class: fabricengine.PushFailureAuth},
			"fix this machine's permission to push main on origin, then run `lym board sync` from the prime worktree",
		},
		{
			"CodeNotFastForward",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureNotFastForward},
			"origin/task holds commits this branch lacks; run `git merge origin/task` in /hub/task, settle any conflict and commit, then run `lym fabric push` in /hub/task",
		},
		{
			"RecordsNotFastForward",
			fabricengine.FailedPush{
				Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureNotFastForward,
				Paths: []string{"_lym/a.md"}, Commits: []string{"abc123"},
			},
			"origin/task-weft holds commits that change the same paths as this branch's (paths: _lym/a.md; local commits: abc123); " +
				"to keep origin's side, run `git reset --keep origin/task-weft` in /hub/task-weft, which drops the local commits; " +
				"to keep this machine's, run `git merge origin/task-weft` in /hub/task-weft, settle the named paths and commit, then run `lym fabric push` in /hub/task, whose push is then a fast-forward and meets no replay",
		},
		{
			"RecordsNotFastForwardWithNoConflictingPathsNamesNone",
			fabricengine.FailedPush{
				Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureNotFastForward,
				Commits: []string{"abc123"},
			},
			"origin/task-weft holds commits this branch lacks (local commits: abc123); " +
				"to keep origin's side, run `git reset --keep origin/task-weft` in /hub/task-weft, which drops the local commits; " +
				"to keep this machine's, run `git merge origin/task-weft` in /hub/task-weft, settle any conflict and commit, then run `lym fabric push` in /hub/task, whose push is then a fast-forward and meets no replay",
		},
		{
			"PrimeRecordsNotFastForwardPushesInThePrimeWorktree",
			fabricengine.FailedPush{
				Repo: "records", RepoPath: "/hub/main-weft", CodeWorktree: "/hub/main", Branch: "main-weft", Class: fabricengine.PushFailureNotFastForward,
				Paths: []string{"_lym/a.md"}, Commits: []string{"abc123"},
			},
			"origin/main-weft holds commits that change the same paths as this branch's (paths: _lym/a.md; local commits: abc123); " +
				"to keep origin's side, run `git reset --keep origin/main-weft` in /hub/main-weft, which drops the local commits; " +
				"to keep this machine's, run `git merge origin/main-weft` in /hub/main-weft, settle the named paths and commit, then run `lym fabric push` in /hub/main, whose push is then a fast-forward and meets no replay",
		},
		{
			"BoltNotFastForward",
			fabricengine.FailedPush{
				Repo: "bolt", RepoPath: "/hub/" + fabricengine.BoltDirName, Branch: "main", Class: fabricengine.PushFailureNotFastForward,
				Paths: []string{"board.json"}, Commits: []string{"def456"},
			},
			"origin/main holds commits that change the same paths as this branch's (paths: board.json; local commits: def456); " +
				"to keep origin's side, run `git reset --keep origin/main` in /hub/" + fabricengine.BoltDirName + ", which drops the local commits; " +
				"to keep this machine's, run `git merge origin/main` in /hub/" + fabricengine.BoltDirName + ", settle the named paths and commit, then run `lym board sync` from the prime worktree, whose push is then a fast-forward and meets no replay",
		},
		{
			"CodeGone",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureGone},
			"origin no longer has branch task, and its board entry is neither done nor removed on origin, since an ended entry writes no record; " +
				"decide whether the run ends: `lym fabric remove` of the pair at /hub/task, run from the prime, removes the pair and clears this record; " +
				"once origin's board shows the entry done or removed, `lym cleanup --apply` tears it down instead",
		},
		{
			"RecordsGoneNamesTheCodeWorktree",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureGone},
			"origin no longer has branch task-weft, and its board entry is neither done nor removed on origin, since an ended entry writes no record; " +
				"decide whether the run ends: `lym fabric remove` of the pair at /hub/task, run from the prime, removes the pair and clears this record; " +
				"once origin's board shows the entry done or removed, `lym cleanup --apply` tears it down instead",
		},
		{
			"PrimeCodeTransportNamesGitPush",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/main", Branch: "main", Prime: true, Class: fabricengine.PushFailureTransport},
			"run `git push` in /hub/main to retry the push",
		},
		{
			"PrimeCodeAuthNamesGitPush",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/main", Branch: "main", Prime: true, Class: fabricengine.PushFailureAuth},
			"fix this machine's permission to push main on origin, then run `git push` in /hub/main",
		},
		{
			"PrimeCodeNotFastForwardMergesAndPushesInThePrime",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/main", Branch: "main", Prime: true, Class: fabricengine.PushFailureNotFastForward},
			"origin/main holds commits this branch lacks; run `git merge origin/main` in /hub/main, settle any conflict and commit, then run `git push` in /hub/main",
		},
		{
			"AnchoredCodeTransportPushesInTheAnchoredDir",
			fabricengine.FailedPush{Repo: "code", RepoPath: anchoredTask, Branch: "task", Class: fabricengine.PushFailureTransport},
			"run `lym fabric push` in " + anchoredDir + " to retry the push",
		},
		{
			"AnchoredRecordsAuthPushesInTheCodeWorktreesAnchoredDir",
			fabricengine.FailedPush{Repo: "records", RepoPath: anchoredWeft, CodeWorktree: anchoredTask, Branch: "task-weft", Class: fabricengine.PushFailureAuth},
			"fix this machine's permission to push task-weft on origin, then run `lym fabric push` in " + anchoredDir,
		},
		{
			"AnchoredCodeNotFastForwardMergesAtTheRootAndPushesInTheAnchoredDir",
			fabricengine.FailedPush{Repo: "code", RepoPath: anchoredTask, Branch: "task", Class: fabricengine.PushFailureNotFastForward},
			"origin/task holds commits this branch lacks; run `git merge origin/task` in " + anchoredTask + ", settle any conflict and commit, then run `lym fabric push` in " + anchoredDir,
		},
		{
			"RecordsWithNoCodeWorktreeNamesNoDirectory",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", Branch: "task-weft", Class: fabricengine.PushFailureDeferred},
			"run `lym fabric push` in  to retry the push",
		},
		{
			"InvalidAnchorMarkerFallsBackToTheRoot",
			fabricengine.FailedPush{Repo: "code", RepoPath: invalidTask, Branch: "task", Class: fabricengine.PushFailureTransport},
			"run `lym fabric push` in " + invalidTask + " to retry the push",
		},
		{
			"AnchoredCodeRewoundResetsAtTheRootAndPushesInTheAnchoredDir",
			fabricengine.FailedPush{Repo: "code", RepoPath: anchoredTask, Branch: "task", Class: fabricengine.PushFailureRewound, OriginHead: "abc123", Commits: []string{"def456"}},
			"origin rewound task to abc123, dropping def456; " +
				"to accept the rewind, run `git reset --keep abc123` in " + anchoredTask + ", which drops the local commits past origin's head, then `lym fabric push` in " + anchoredDir + "; " +
				"to restore the dropped commits, run `git push origin task` in " + anchoredTask + ", a plain fast-forward",
		},
	}
	for _, tt := range wayForwardRows {
		t.Run("WayForward_"+tt.name, func(t *testing.T) {
			got := fabricengine.FailedPushWayForward(tt.failure)
			if got != tt.want {
				t.Errorf("FailedPushWayForward(%s) =\n%s\nwant\n%s", tt.name, got, tt.want)
			}
			if tt.failure.Prime && strings.Contains(got, "lym fabric push") {
				t.Errorf("FailedPushWayForward(%s) = %q; a prime record never names `lym fabric push`", tt.name, got)
			}
		})
	}

	stopRows := []struct {
		name    string
		outcome fabricengine.BranchOutcome
		want    string
	}{
		{
			"RewrittenCodeBranchPullsInTheAnchoredDir",
			fabricengine.BranchOutcome{Repo: "code", Worktree: anchoredTask, CodeWorktree: anchoredTask, State: fabricengine.BranchRewritten},
			"`lym fabric pull` in " + anchoredDir,
		},
		{
			"AnchorStoppedPairWeftSetsTheAnchorInTheAnchoredDir",
			fabricengine.BranchOutcome{Repo: "records", Slug: "task", Worktree: anchoredWeft, CodeWorktree: anchoredTask, State: fabricengine.BranchAnchorStopped, Anchor: "sub"},
			"run `lym fabric anchor task --set sub` in " + anchoredDir,
		},
		{
			"AnchorStoppedPrimeWeftSetsTheAnchorInTheAnchoredDir",
			fabricengine.BranchOutcome{Repo: "records", Worktree: anchoredWeft, CodeWorktree: anchoredTask, State: fabricengine.BranchAnchorStopped, Anchor: "sub"},
			"end every reed session of the hub, the orch's included, with `lym reed down` in each, and wait until no batten run of the hub is live; " +
				"then run `lym fabric anchor --set sub` in " + anchoredDir + "; then run `lym beam start` again",
		},
		{
			"DirtyRecordsBranchCommitsInTheAnchoredDirAndRestoresAtTheRecordsRoot",
			fabricengine.BranchOutcome{Repo: "records", Worktree: anchoredWeft, CodeWorktree: anchoredTask, State: fabricengine.BranchDirty, Paths: []string{"notes/a.md"}},
			"commit the uncommitted paths (notes/a.md) with `lym fabric commit` in " + anchoredDir + ", or any of them with `git commit` or `git checkout -- <path>` in " + anchoredWeft,
		},
	}
	for _, tt := range stopRows {
		t.Run("StopWayForward_"+tt.name, func(t *testing.T) {
			if got := fabricengine.StopWayForwardForTest(tt.outcome); got != tt.want {
				t.Errorf("stopWayForward(%s) =\n%s\nwant\n%s", tt.name, got, tt.want)
			}
		})
	}

	t.Run("WeftDirtyStopCommitsInTheAnchoredDir", func(t *testing.T) {
		stop := fabricengine.WeftDirtyStopForTest(anchoredTask, anchoredWeft, "task-weft", []string{"notes/a.md"})
		want := "commit the uncommitted paths in " + anchoredWeft + " (notes/a.md) with `lym fabric commit` in " + anchoredDir + ", then re-run `lym fabric pull`"
		if stop.WayForward != want {
			t.Errorf("weftDirtyStop way forward = %q; want %q", stop.WayForward, want)
		}
	})

	t.Run("WeftConflictStopMergesAtTheRecordsRootAndPushesInTheAnchoredDir", func(t *testing.T) {
		stop := fabricengine.WeftConflictStopForTest(anchoredTask, anchoredWeft, "task-weft", []string{"notes/a.md"}, []string{"abc123"})
		want := "origin/task-weft holds commits that change the same paths as this branch's (paths: notes/a.md; local commits: abc123); " +
			"to keep origin's side, run `git reset --keep origin/task-weft` in " + anchoredWeft + ", which drops the local commits; " +
			"to keep this machine's, run `git merge origin/task-weft` in " + anchoredWeft + ", settle the named paths and commit, " +
			"then run `lym fabric push` in " + anchoredDir + ", whose push is then a fast-forward and meets no replay"
		if stop.WayForward != want {
			t.Errorf("weftConflictStop way forward =\n%s\nwant\n%s", stop.WayForward, want)
		}
	})
}
