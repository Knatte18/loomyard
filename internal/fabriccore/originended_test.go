// originended_test.go tests the rules of removing the pairs of ended runs: the teardown calls and what is left.

package fabriccore_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestEndedTeardowns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	teardownOf := func(slug string) fabriccore.Answer {
		return fabriccore.Answer{
			Actions: []fabriccore.Action{{Kind: fabriccore.ActionSeamCall, Seam: fabriccore.SeamTeardown, Ref: slug}},
			Reread:  []fabriccore.Field{fabriccore.FieldTeardownReports},
		}
	}
	twoRuns := func() fabriccore.Snapshot {
		return fabriccore.Snapshot{Ended: fabriccore.EndedFacts{Seam: true, Runs: []fabriccore.EndedRun{
			{Slug: "ask", How: "done", Local: true},
			{Slug: "neve", How: "removed", Local: true},
		}}}
	}
	ended := func(slug, how string) fabriclayout.EntryOutcome {
		return fabriclayout.EntryOutcome{Slug: slug, State: fabriclayout.EntryEnded, How: how, Local: true}
	}

	t.Run("EachReportFeedsTheNextAnswer", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			first fabriclayout.TeardownReport
			want  func() fabriclayout.EntryOutcome
		}{
			{
				"BusyOnTheDriver",
				fabriclayout.TeardownReport{Busy: true, BusyOn: fabriclayout.BusyOnDriver, Kept: []string{"the pair"}},
				func() fabriclayout.EntryOutcome {
					entry := ended("ask", "done")
					entry.Busy, entry.Kept = true, []string{"the pair"}
					entry.WayForward = "wait for the pair's driver to end, which the run's next Parent-Align or Finalize makes certain by stopping it with Halt, then re-run `lym cleanup --apply`"
					return entry
				},
			},
			{
				"BusyOnALock",
				fabriclayout.TeardownReport{Busy: true, BusyOn: fabriclayout.BusyOnLock},
				func() fabriclayout.EntryOutcome {
					entry := ended("ask", "done")
					entry.Busy = true
					entry.WayForward = "wait for the holder of the pair's teardown or weft write lock to release it, then re-run `lym cleanup --apply`"
					return entry
				},
			},
			{
				"Failed",
				fabriclayout.TeardownReport{Error: errors.New("worktree locked"), Tags: []string{"set-aside/ask/1"}},
				func() fabriclayout.EntryOutcome {
					entry := ended("ask", "done")
					entry.Error, entry.Tags = "worktree locked", []string{"set-aside/ask/1"}
					return entry
				},
			},
		}
		for _, tt := range tests {
			snapshot := twoRuns()
			if got := fabriccore.EndedTeardowns(snapshot); !reflect.DeepEqual(got, teardownOf("ask")) {
				t.Errorf("%s: EndedTeardowns() = %+v; want the teardown of ask", tt.name, got)
			}
			snapshot.Ended.Runs[0].Report = &tt.first
			if got := fabriccore.EndedTeardowns(snapshot); !reflect.DeepEqual(got, teardownOf("neve")) {
				t.Errorf("%s: EndedTeardowns() after ask = %+v; want the teardown of neve", tt.name, got)
			}
			snapshot.Ended.Runs[1].Report = &fabriclayout.TeardownReport{}
			if got := fabriccore.EndedTeardowns(snapshot); !reflect.DeepEqual(got, fabriccore.Answer{}) {
				t.Errorf("%s: EndedTeardowns() after both = %+v; want done", tt.name, got)
			}
			removed := ended("neve", "removed")
			removed.Removed = true
			want := []fabriclayout.EntryOutcome{tt.want(), removed}
			if got := fabriccore.EndedEntries(snapshot); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: EndedEntries() = %+v; want %+v", tt.name, got, want)
			}
		}
	})

	t.Run("NoCallLeavesTheEntryEndedUntouched", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			scope fabriclayout.ReconcileScope
			seam  bool
			local bool
		}{
			{"JudgeOnly", fabriclayout.ReconcileScope{JudgeOnly: true}, true, true},
			{"TheScopesOwnRun", fabriclayout.ReconcileScope{Run: "ask"}, true, true},
			{"NoLocalPart", fabriclayout.ReconcileScope{}, true, false},
			{"NoSeam", fabriclayout.ReconcileScope{}, false, true},
		}
		for _, tt := range tests {
			snapshot := fabriccore.Snapshot{Ended: fabriccore.EndedFacts{
				Scope: tt.scope,
				Seam:  tt.seam,
				Runs:  []fabriccore.EndedRun{{Slug: "ask", How: "done", Local: tt.local}},
			}}
			if got := fabriccore.EndedTeardowns(snapshot); !reflect.DeepEqual(got, fabriccore.Answer{}) {
				t.Errorf("%s: EndedTeardowns() = %+v; want no call", tt.name, got)
			}
			want := []fabriclayout.EntryOutcome{{Slug: "ask", State: fabriclayout.EntryEnded, How: "done", Local: tt.local}}
			if got := fabriccore.EndedEntries(snapshot); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: EndedEntries() = %+v; want %+v", tt.name, got, want)
			}
		}
	})

	t.Run("EndedEntriesOverrideTheClassifiedOnesAndSortBySlug", func(t *testing.T) {
		t.Parallel()
		classified := []fabriclayout.EntryOutcome{{Slug: "neve", State: fabriclayout.EntryActive}, {Slug: "task", State: fabriclayout.EntryStopped}}
		got := fabriccore.MergeEndedEntries(classified, []fabriclayout.EntryOutcome{ended("task", "done"), ended("ask", "done")})
		want := []fabriclayout.EntryOutcome{ended("ask", "done"), {Slug: "neve", State: fabriclayout.EntryActive}, ended("task", "done")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("MergeEndedEntries() = %+v; want %+v", got, want)
		}
	})

	t.Run("ARemovedPairsBranchesAreDropped", func(t *testing.T) {
		t.Parallel()
		branches := []fabriclayout.BranchOutcome{{Slug: "ask", Branch: "ask"}, {Slug: "task", Branch: "task"}, {Branch: "main"}}
		got := fabriccore.WithoutRemovedPairs(branches, []fabriclayout.EntryOutcome{{Slug: "ask", Removed: true}, {Slug: "task"}})
		want := []fabriclayout.BranchOutcome{{Slug: "task", Branch: "task"}, {Branch: "main"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("WithoutRemovedPairs() = %+v; want %+v", got, want)
		}
	})

	t.Run("OnlyAFailureOfABranchNotLocalIsStale", func(t *testing.T) {
		t.Parallel()
		records := []fabriclayout.FailedPush{
			{RepoPath: "/hub/a", Branch: "task"},
			{RepoPath: "/hub/b", Branch: "task"},
			{RepoPath: "/hub/a", Branch: "gone"},
		}
		got := fabriccore.StalePushFailures(records, map[string]bool{fabriccore.LocalBranchKey("/hub/a", "task"): true})
		want := []fabriclayout.FailedPush{{RepoPath: "/hub/b", Branch: "task"}, {RepoPath: "/hub/a", Branch: "gone"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("StalePushFailures() = %+v; want %+v", got, want)
		}
	})
}
