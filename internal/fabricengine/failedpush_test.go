// failedpush_test.go covers the failed-push record store, its once-per-episode notice,
// and the anchor a way-forward line reads from disk.

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
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

func composedPushLine(f fabriclayout.FailedPush) string {
	return fmt.Sprintf("a push of %s branch %s failed (%s): %s; way forward: %s", f.Repo, f.Branch, f.Class, f.Error, fabricengine.FailedPushWayForward(f))
}

func codePushFailure() fabriclayout.FailedPush {
	return fabriclayout.FailedPush{
		Repo:     "code",
		RepoPath: "/hub/task",
		Branch:   "task",
		Class:    fabriclayout.PushFailureTransport,
		Error:    "connection refused",
	}
}

// TestFailedPushRecord covers the record's lifecycle and its single notice per episode.
// It also checks the way-forward lines whose directory is the anchor read from disk.
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
		dir := filepath.Join(fabriclayout.HubScratchDir(hub), lymdirs.FailedPushDirName)
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
		failure fabriclayout.FailedPush
		want    string
	}{
		{
			"AnchoredCodeTransportPushesInTheAnchoredDir",
			fabriclayout.FailedPush{Repo: "code", RepoPath: anchoredTask, Branch: "task", Class: fabriclayout.PushFailureTransport},
			"run `lym fabric push` in " + anchoredDir + " to retry the push",
		},
		{
			"AnchoredRecordsAuthPushesInTheCodeWorktreesAnchoredDir",
			fabriclayout.FailedPush{Repo: "records", RepoPath: anchoredWeft, CodeWorktree: anchoredTask, Branch: "task-weft", Class: fabriclayout.PushFailureAuth},
			"fix this machine's permission to push task-weft on origin, then run `lym fabric push` in " + anchoredDir,
		},
		{
			"InvalidAnchorMarkerFallsBackToTheRoot",
			fabriclayout.FailedPush{Repo: "code", RepoPath: invalidTask, Branch: "task", Class: fabriclayout.PushFailureTransport},
			"run `lym fabric push` in " + invalidTask + " to retry the push",
		},
	}
	for _, tt := range wayForwardRows {
		t.Run("WayForward_"+tt.name, func(t *testing.T) {
			got := fabricengine.FailedPushWayForward(tt.failure)
			if got != tt.want {
				t.Errorf("FailedPushWayForward(%s) =\n%s\nwant\n%s", tt.name, got, tt.want)
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
