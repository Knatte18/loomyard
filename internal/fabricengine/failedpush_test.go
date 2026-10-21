// failedpush_test.go covers the failed-push record store, its once-per-episode notice and its way-forward lines.
// It is an external test package, and the notifier is process-global, so every test here that installs one runs without t.Parallel.

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
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// fakePushNotifier counts the calls carrying its own hub path, so a parallel test recording a failure in another hub never reaches its counts.
type fakePushNotifier struct {
	hub    string
	posted bool
	err    error
	lines  []string
}

func (n *fakePushNotifier) notify(hubPath, line string) (bool, error) {
	if hubPath != n.hub {
		return true, nil
	}
	n.lines = append(n.lines, line)
	return n.posted, n.err
}

func installFakePushNotifier(t *testing.T, n *fakePushNotifier) {
	t.Helper()
	t.Cleanup(fabricengine.InstallPushNotifier(n.notify))
}

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

// TestFailedPushRecord covers the record's lifecycle, its single notice per episode, and every side and class way-forward line a push writes.
//
// It runs without t.Parallel: the notifier it installs is process-global.
func TestFailedPushRecord(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("FirstFailureMailsOnceAndLaterOnesOnlyUpdate", func(t *testing.T) {
		hub := t.TempDir()
		fake := &fakePushNotifier{hub: hub, posted: true}
		installFakePushNotifier(t, fake)
		failure := codePushFailure()

		if err := fabricengine.RecordPushFailure(hub, failure, base); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}
		if err := fabricengine.RecordPushFailure(hub, failure, base.Add(time.Minute)); err != nil {
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

	t.Run("NotPostedLeavesNotifiedUnsetSoTheNextFailureMails", func(t *testing.T) {
		hub := t.TempDir()
		fake := &fakePushNotifier{hub: hub, posted: false}
		installFakePushNotifier(t, fake)

		for i := range 2 {
			if err := fabricengine.RecordPushFailure(hub, codePushFailure(), base.Add(time.Duration(i)*time.Minute)); err != nil {
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
		installFakePushNotifier(t, fake)

		for i := range 2 {
			if err := fabricengine.RecordPushFailure(hub, codePushFailure(), base.Add(time.Duration(i)*time.Minute)); err != nil {
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
		installFakePushNotifier(t, fake)
		failure := codePushFailure()
		failure.Class = fabricengine.PushFailureDeferred

		if err := fabricengine.RecordPushFailure(hub, failure, base); err != nil {
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
		dir := filepath.Join(fabricengine.HubScratchDir(hub), lyxdirs.FailedPushDirName)
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

	wayForwardRows := []struct {
		name    string
		failure fabricengine.FailedPush
		want    string
	}{
		{
			"CodeDeferred",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureDeferred},
			"run `lyx fabric push` in /hub/task to retry the push",
		},
		{
			"RecordsDeferred",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureDeferred},
			"run `lyx fabric push` in /hub/task to retry the push",
		},
		{
			"BoltDeferred",
			fabricengine.FailedPush{Repo: "bolt", RepoPath: "/hub/_board", Branch: "main", Class: fabricengine.PushFailureDeferred},
			"run `lyx board sync` from the prime worktree to retry the push",
		},
		{
			"CodeTransport",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureTransport},
			"run `lyx fabric push` in /hub/task to retry the push",
		},
		{
			"RecordsTransport",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureTransport},
			"run `lyx fabric push` in /hub/task to retry the push",
		},
		{
			"PrimeRecordsTransportPushesInThePrimeWorktree",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/main-weft", CodeWorktree: "/hub/main", Branch: "main-weft", Class: fabricengine.PushFailureTransport},
			"run `lyx fabric push` in /hub/main to retry the push",
		},
		{
			"BoltTransport",
			fabricengine.FailedPush{Repo: "bolt", RepoPath: "/hub/_board", Branch: "main", Class: fabricengine.PushFailureTransport},
			"run `lyx board sync` from the prime worktree to retry the push",
		},
		{
			"CodeAuth",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureAuth},
			"fix this machine's permission to push task on origin, then run `lyx fabric push` in /hub/task",
		},
		{
			"RecordsAuth",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureAuth},
			"fix this machine's permission to push task-weft on origin, then run `lyx fabric push` in /hub/task",
		},
		{
			"BoltAuth",
			fabricengine.FailedPush{Repo: "bolt", RepoPath: "/hub/_board", Branch: "main", Class: fabricengine.PushFailureAuth},
			"fix this machine's permission to push main on origin, then run `lyx board sync` from the prime worktree",
		},
		{
			"CodeNotFastForward",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureNotFastForward},
			"origin/task holds commits this branch lacks; run `git merge origin/task` in /hub/task, settle any conflict and commit, then run `lyx fabric push` in /hub/task",
		},
		{
			"RecordsNotFastForward",
			fabricengine.FailedPush{
				Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureNotFastForward,
				Paths: []string{"_lyx/a.md"}, Commits: []string{"abc123"},
			},
			"another machine wrote the same paths on origin/task-weft (paths: _lyx/a.md; local commits: abc123); " +
				"to keep origin's side, run `git reset --keep origin/task-weft` in /hub/task-weft, which drops the local commits; " +
				"to keep this machine's, run `git merge origin/task-weft` in /hub/task-weft, settle the named paths and commit, then run `lyx fabric push` in /hub/task, whose push is then a fast-forward and meets no replay",
		},
		{
			"PrimeRecordsNotFastForwardPushesInThePrimeWorktree",
			fabricengine.FailedPush{
				Repo: "records", RepoPath: "/hub/main-weft", CodeWorktree: "/hub/main", Branch: "main-weft", Class: fabricengine.PushFailureNotFastForward,
				Paths: []string{"_lyx/a.md"}, Commits: []string{"abc123"},
			},
			"another machine wrote the same paths on origin/main-weft (paths: _lyx/a.md; local commits: abc123); " +
				"to keep origin's side, run `git reset --keep origin/main-weft` in /hub/main-weft, which drops the local commits; " +
				"to keep this machine's, run `git merge origin/main-weft` in /hub/main-weft, settle the named paths and commit, then run `lyx fabric push` in /hub/main, whose push is then a fast-forward and meets no replay",
		},
		{
			"BoltNotFastForward",
			fabricengine.FailedPush{
				Repo: "bolt", RepoPath: "/hub/_board", Branch: "main", Class: fabricengine.PushFailureNotFastForward,
				Paths: []string{"board.json"}, Commits: []string{"def456"},
			},
			"another machine wrote the same paths on origin/main (paths: board.json; local commits: def456); " +
				"to keep origin's side, run `git reset --keep origin/main` in /hub/_board, which drops the local commits; " +
				"to keep this machine's, run `git merge origin/main` in /hub/_board, settle the named paths and commit, then run `lyx board sync` from the prime worktree, whose push is then a fast-forward and meets no replay",
		},
		{
			"CodeGone",
			fabricengine.FailedPush{Repo: "code", RepoPath: "/hub/task", Branch: "task", Class: fabricengine.PushFailureGone},
			"origin no longer has branch task, and its board entry is neither done nor removed on origin, since an ended entry writes no record; " +
				"decide whether the run ends: `lyx fabric remove` of the pair at /hub/task, run from the prime, removes the pair and clears this record; " +
				"once origin's board shows the entry done or removed, `lyx cleanup --apply` tears it down instead",
		},
		{
			"RecordsGoneNamesTheCodeWorktree",
			fabricengine.FailedPush{Repo: "records", RepoPath: "/hub/task-weft", CodeWorktree: "/hub/task", Branch: "task-weft", Class: fabricengine.PushFailureGone},
			"origin no longer has branch task-weft, and its board entry is neither done nor removed on origin, since an ended entry writes no record; " +
				"decide whether the run ends: `lyx fabric remove` of the pair at /hub/task, run from the prime, removes the pair and clears this record; " +
				"once origin's board shows the entry done or removed, `lyx cleanup --apply` tears it down instead",
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
	}
	for _, tt := range wayForwardRows {
		t.Run("WayForward_"+tt.name, func(t *testing.T) {
			got := fabricengine.FailedPushWayForward(tt.failure)
			if got != tt.want {
				t.Errorf("FailedPushWayForward(%s) =\n%s\nwant\n%s", tt.name, got, tt.want)
			}
			if tt.failure.Prime && strings.Contains(got, "lyx fabric push") {
				t.Errorf("FailedPushWayForward(%s) = %q; a prime record never names `lyx fabric push`", tt.name, got)
			}
		})
	}
}
