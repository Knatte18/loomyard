//go:build tier1

// originreconcileended_integration_test.go proves what the origin reconcile does with the runs origin has ended, on hubforge hubs.
// The teardown seam is a recording fake, since the local removal's own effects belong to the removal.
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
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

// teardownRecorder is a teardown seam that records each request and answers with reply.
// A nil reply removes the pair without a word.
type teardownRecorder struct {
	calls []fabricengine.EndedTeardown
	reply func(fabricengine.EndedTeardown) (fabricengine.TeardownReport, error)
}

func (r *teardownRecorder) seams() fabricengine.OriginSeams {
	return fabricengine.OriginSeams{Teardown: func(request fabricengine.EndedTeardown) (fabricengine.TeardownReport, error) {
		r.calls = append(r.calls, request)
		if r.reply == nil {
			return fabricengine.TeardownReport{}, nil
		}
		return r.reply(request)
	}}
}

// slugsCalled returns the slug of each request, in order.
func (r *teardownRecorder) slugsCalled() []string {
	slugs := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		slugs = append(slugs, call.Slug)
	}
	return slugs
}

// tagMutation returns a record holding one created set-aside tag.
func tagMutation(hub *originHub, tag string) fabricengine.MutationRecord {
	rec := fabricengine.NewMutations(hub.Location.HubPath)
	rec.AppendRef(fabricengine.KindTagCreated, tag, "")
	return fabricengine.MutationRecord{Mutations: rec.Snapshot()}
}

// publishBoard commits a board holding entries at the board checkout, pushes it and returns the Bolt branch.
func publishBoard(t *testing.T, hub *originHub, entries ...boardengine.Task) string {
	t.Helper()
	board := hub.BoardDir()
	branch := gitkit.CurrentBranch(t, board)
	gitkit.CommitFile(t, board, "board.json", boardFileOf(t, entries...), "base board")
	gitkit.Git(t, board, "push", "origin", branch)
	return branch
}

// originBoard has origin's Bolt hold entries.
func originBoard(t *testing.T, hub *originHub, branch string, entries ...boardengine.Task) {
	t.Helper()
	originCommitFile(t, hub.RecordsBare, branch, "board.json", boardFileOf(t, entries...))
}

// writePairStatus gives the pair slug its own run status, with the current producer and the history rows named.
func writePairStatus(t *testing.T, hub *originHub, slug, producer string, history ...string) {
	t.Helper()
	path := filepath.Join(hub.PairCodeWorktree(slug), "_lym", "shed", slug, "status.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(statusJSON(producer, history...)), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// endedHub is a hub with the pairs slugs, each holding a batten run record at the prime before its Finalize, whose board origin has marked done.
func endedHub(t *testing.T, slugs ...string) (*originHub, string) {
	t.Helper()
	hub := newOriginHub(t)
	var live, done []boardengine.Task
	for _, slug := range slugs {
		originPair(t, hub, slug)
		writeStatus(t, hub, slug, "Implementer")
		live = append(live, boardEntry(slug))
		done = append(done, boardEntry(slug, withBoardStatus("done")))
	}
	branch := publishBoard(t, hub, live...)
	originBoard(t, hub, branch, done...)
	return hub, branch
}

// TestReconcileOrigin_Ended covers the origin reconcile's handling of runs origin has ended: which entries it ends, what it hands the teardown seam and how it reports each outcome.
// It runs without t.Parallel because it sets the process-global origin call hook.
func TestReconcileOrigin_Ended(t *testing.T) {
	boardkit.InstallReplay(t)

	t.Run("An entry done on origin with a local pair is handed to the teardown seam once and reported ended as landed", func(t *testing.T) {
		hub, _ := endedHub(t, "task")
		recorder := &teardownRecorder{reply: func(fabricengine.EndedTeardown) (fabricengine.TeardownReport, error) {
			return fabricengine.TeardownReport{MutationRecord: tagMutation(hub, "set-aside/task/abc"), Tags: []string{"set-aside/task/abc"}, Kept: []string{"branch task: unlanded"}}, nil
		}}

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		if got := recorder.slugsCalled(); !slices.Equal(got, []string{"task"}) {
			t.Fatalf("teardown calls = %v; want one call for task", got)
		}
		if recorder.calls[0].EarlierSnapshot || recorder.calls[0].Force {
			t.Errorf("teardown request = %+v; want no earlier snapshot and no force", recorder.calls[0])
		}
		entry := requireEntry(t, report, "task", fabricengine.EntryEnded)
		if entry.How != "done" || !entry.Local || !entry.Removed || entry.Busy || entry.Error != "" {
			t.Errorf("entry = %+v; want ended as done, local and removed", entry)
		}
		if !slices.Equal(entry.Tags, []string{"set-aside/task/abc"}) || !slices.Equal(entry.Kept, []string{"branch task: unlanded"}) {
			t.Errorf("entry Tags = %v, Kept = %v; want the seam report's", entry.Tags, entry.Kept)
		}
		if got := kindCount(report, fabricengine.KindTagCreated); got != 1 {
			t.Errorf("tag_created entries = %d; want the seam's one in the report's record", got)
		}
		for _, branch := range report.Branches {
			if branch.Slug == "task" {
				t.Errorf("branch %q of the removed pair is still reported", branch.Branch)
			}
		}
		if !strings.Contains(report.Summary(), "ended: ") {
			t.Errorf("Summary = %q; want the ended entries listed", report.Summary())
		}
	})

	t.Run("An ended entry with a run record and no local pair is ended with no seam call and no error", func(t *testing.T) {
		hub := newOriginHub(t)
		writeStatus(t, hub, "ghost", "Implementer")
		branch := publishBoard(t, hub, boardEntry("ghost"))
		originBoard(t, hub, branch, boardEntry("ghost", withBoardStatus("done")))
		recorder := &teardownRecorder{}
		var calls callLog
		t.Cleanup(fabricengine.SetOriginCallHookForTest(calls.record))

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none for a run with no local pair", recorder.slugsCalled())
		}
		entry := requireEntry(t, report, "ghost", fabricengine.EntryEnded)
		if entry.Local || entry.Removed || entry.Busy || entry.Error != "" {
			t.Errorf("entry = %+v; want ended with nothing local, nothing removed and no error", entry)
		}
		if got := len(calls.withVerb("ls-remote")); got != 2 {
			t.Errorf("ls-remote calls = %d; want 2", got)
		}
		if strings.Contains(report.Summary(), "ghost") {
			t.Errorf("Summary = %q; want an entry with no local pair left out", report.Summary())
		}
	})

	t.Run("A hub reconcile ends a pair at Finalize before its parent merge although it fast-forwards the parent", func(t *testing.T) {
		hub, _ := endedHub(t, "task")
		writePairStatus(t, hub, "task", "Finalize")
		gitkit.CommitFile(t, hub.PairCodeWorktree("task"), "feature.txt", "task work\n", "task work")
		originCommitFile(t, hub.CodeBare, hub.primeBranch, "moved.txt", "moved")
		recorder := &teardownRecorder{}

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		if got := recorder.slugsCalled(); !slices.Equal(got, []string{"task"}) {
			t.Errorf("teardown calls = %v; want one call for task", got)
		}
		if prime := branchOf(t, report, "code", hub.primeBranch); !prime.Moved {
			t.Errorf("prime code branch = %+v; want it fast-forwarded in the same reconcile", prime)
		}
	})

	t.Run("A pair whose Finalize passed its parent merge is not ended", func(t *testing.T) {
		hub, _ := endedHub(t, "task")
		writePairStatus(t, hub, "task", "Land", "Finalize")
		recorder := &teardownRecorder{}

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none for a landing on this machine", recorder.slugsCalled())
		}
		requireEntry(t, report, "task", fabricengine.EntryActive)
	})

	t.Run("An entry removed from origin's board is ended as removed", func(t *testing.T) {
		hub := newOriginHub(t)
		originPair(t, hub, "task")
		writeStatus(t, hub, "task", "Implementer")
		branch := publishBoard(t, hub, boardEntry("task"), boardEntry("keep"))
		originBoard(t, hub, branch, boardEntry("keep"))
		recorder := &teardownRecorder{}

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		entry := requireEntry(t, report, "task", fabricengine.EntryEnded)
		if entry.How != "removed" || !entry.Removed {
			t.Errorf("entry = %+v; want ended as removed and torn down", entry)
		}
		if got := recorder.slugsCalled(); !slices.Equal(got, []string{"task"}) {
			t.Errorf("teardown calls = %v; want one call for task", got)
		}
	})

	t.Run("A busy teardown leaves the pair, the entry ended with the busy flag and the way forward the wait names", func(t *testing.T) {
		for _, row := range []struct {
			name, busyOn string
			want         []string
		}{
			{"driver", fabricengine.BusyOnDriver, []string{"driver to end", "lym cleanup --apply"}},
			{"lock", fabricengine.BusyOnLock, []string{"lock", "lym cleanup --apply"}},
		} {
			t.Run(row.name, func(t *testing.T) {
				hub, _ := endedHub(t, "task")
				recorder := &teardownRecorder{reply: func(fabricengine.EndedTeardown) (fabricengine.TeardownReport, error) {
					return fabricengine.TeardownReport{Busy: true, BusyOn: row.busyOn}, nil
				}}

				report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

				entry := requireEntry(t, report, "task", fabricengine.EntryEnded)
				if !entry.Busy || entry.Removed {
					t.Errorf("entry = %+v; want ended, busy and not removed", entry)
				}
				for _, text := range row.want {
					if !strings.Contains(entry.WayForward, text) || !strings.Contains(report.Summary(), text) {
						t.Errorf("WayForward = %q, Summary = %q; want both to name %q", entry.WayForward, report.Summary(), text)
					}
				}
			})
		}
	})

	t.Run("A teardown failing with another error is ended with the error and a second entry is still handed to the seam", func(t *testing.T) {
		hub, _ := endedHub(t, "task", "two")
		recorder := &teardownRecorder{reply: func(request fabricengine.EndedTeardown) (fabricengine.TeardownReport, error) {
			if request.Slug == "task" {
				return fabricengine.TeardownReport{}, errors.New("boom")
			}
			return fabricengine.TeardownReport{}, nil
		}}

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		if got := recorder.slugsCalled(); !slices.Equal(got, []string{"task", "two"}) {
			t.Fatalf("teardown calls = %v; want both entries handed to the seam", got)
		}
		failed := requireEntry(t, report, "task", fabricengine.EntryEnded)
		if failed.Error != "boom" || failed.Removed || failed.Busy {
			t.Errorf("entry task = %+v; want ended with the error, not removed and not busy", failed)
		}
		if two := requireEntry(t, report, "two", fabricengine.EntryEnded); !two.Removed {
			t.Errorf("entry two = %+v; want it removed", two)
		}
		if summary := report.Summary(); !strings.Contains(summary, "boom") || !strings.Contains(summary, "`lym cleanup --apply` retries the removal") {
			t.Errorf("Summary = %q; want the error and the retry named", summary)
		}
	})

	t.Run("An entry only local Bolt holds is not ended and its pair is not torn down", func(t *testing.T) {
		hub := newOriginHub(t)
		originPair(t, hub, "mine")
		writePairStatus(t, hub, "mine", "Implementer")
		writeStatus(t, hub, "mine", "Implementer")
		board := hub.BoardDir()
		branch := publishBoard(t, hub, boardEntry("keep"))
		gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("keep"), boardEntry("mine")), "mine, local only")
		originBoard(t, hub, branch, boardEntry("keep", withBoardTitle("origin")))
		recorder := &teardownRecorder{}

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none", recorder.slugsCalled())
		}
		if entry := entryOf(t, report, "mine"); entry.State == fabricengine.EntryEnded {
			t.Errorf("entry mine = %+v; want it not ended", entry)
		}
	})

	t.Run("A pair with no run record and no board entry is never torn down", func(t *testing.T) {
		hub := newOriginHub(t)
		originPair(t, hub, "plain")
		branch := publishBoard(t, hub, boardEntry("keep"))
		originBoard(t, hub, branch, boardEntry("keep", withBoardTitle("origin")))
		recorder := &teardownRecorder{}

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none", recorder.slugsCalled())
		}
		if len(report.Entries) != 0 {
			t.Errorf("entries = %+v; want none for a pair with no run record", report.Entries)
		}
	})

	t.Run("The run scope ends its own run like any other and calls no teardown", func(t *testing.T) {
		for _, row := range []struct {
			name      string
			merged    bool
			wantEnded bool
		}{
			{"a parent lacking the task's work is the landing another machine made", false, true},
			{"a parent the run's own Finalize merged into is the re-entry after its done mark", true, false},
		} {
			t.Run(row.name, func(t *testing.T) {
				hub, _ := endedHub(t, "task")
				writePairStatus(t, hub, "task", "Finalize")
				gitkit.CommitFile(t, hub.PairCodeWorktree("task"), "feature.txt", "task work\n", "task work")
				if row.merged {
					gitkit.Git(t, hub.PrimeWorktree(), "merge", "--no-edit", gitkit.CurrentBranch(t, hub.PairCodeWorktree("task")))
				}
				recorder := &teardownRecorder{}

				report := moveReconcile(t, hub, fabricengine.ReconcileScope{Run: "task"}, recorder.seams())

				if len(recorder.calls) != 0 {
					t.Errorf("teardown calls = %v; want none, since the run's driver is the caller", recorder.slugsCalled())
				}
				state := entryOf(t, report, "task").State
				if (state == fabricengine.EntryEnded) != row.wantEnded {
					t.Errorf("entry task state = %q; want ended = %v", state, row.wantEnded)
				}
			})
		}
	})

	t.Run("A judge-only reconcile reports the teardown it would run and calls none", func(t *testing.T) {
		hub, _ := endedHub(t, "task")
		recorder := &teardownRecorder{}

		report := moveReconcile(t, hub, fabricengine.ReconcileScope{JudgeOnly: true}, recorder.seams())

		if len(recorder.calls) != 0 {
			t.Errorf("teardown calls = %v; want none", recorder.slugsCalled())
		}
		entry := requireEntry(t, report, "task", fabricengine.EntryEnded)
		if !entry.Local || entry.Removed {
			t.Errorf("entry = %+v; want ended with a local pair and nothing removed", entry)
		}
		if !strings.Contains(report.Summary(), "ended: task") {
			t.Errorf("Summary = %q; want task listed as ended", report.Summary())
		}
	})

	t.Run("An earlier snapshot tag sets EarlierSnapshot on the request", func(t *testing.T) {
		hub, _ := endedHub(t, "task")
		weftRoot, err := fabricengine.RecordsRepoRoot(hub.Location)
		if err != nil {
			t.Fatalf("RecordsRepoRoot: %v", err)
		}
		gitkit.Git(t, weftRoot, "tag", "-a", "-m", "snapshot", "set-aside/task/abc123")
		recorder := &teardownRecorder{}

		moveReconcile(t, hub, fabricengine.ReconcileScope{}, recorder.seams())

		if len(recorder.calls) != 1 || !recorder.calls[0].EarlierSnapshot {
			t.Errorf("teardown calls = %+v; want one request with EarlierSnapshot set", recorder.calls)
		}
	})

	t.Run("A failed-push record of a branch no longer local is removed and a moving reconcile only", func(t *testing.T) {
		hub := newOriginHub(t)
		hubPath := hub.Location.HubPath
		seen := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		for _, branch := range []string{"vanished", hub.primeBranch} {
			failure := fabricengine.FailedPush{Repo: "code", RepoPath: hub.PrimeWorktree(), Branch: branch, Class: fabricengine.PushFailureTransport}
			if err := fabricengine.RecordPushFailure(hubPath, failure, seen); err != nil {
				t.Fatalf("RecordPushFailure(%s): %v", branch, err)
			}
		}
		branches := func() []string {
			failures, err := fabricengine.ListFailedPushes(hubPath)
			if err != nil {
				t.Fatalf("ListFailedPushes: %v", err)
			}
			var names []string
			for _, failure := range failures {
				names = append(names, failure.Branch)
			}
			slices.Sort(names)
			return names
		}
		want := []string{hub.primeBranch, "vanished"}
		slices.Sort(want)

		moveReconcile(t, hub, fabricengine.ReconcileScope{JudgeOnly: true}, fabricengine.OriginSeams{})
		if got := branches(); !slices.Equal(got, want) {
			t.Errorf("failed-push branches after a judge-only reconcile = %v; want %v, none cleared", got, want)
		}

		moveReconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{})
		if got := branches(); !slices.Equal(got, []string{hub.primeBranch}) {
			t.Errorf("failed-push branches after a moving reconcile = %v; want only %s", got, hub.primeBranch)
		}
	})
}
