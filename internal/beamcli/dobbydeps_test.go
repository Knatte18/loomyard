// dobbydeps_test.go pins how the dobby reads a hub reconcile's report on the ended runs it asked to tear down.

package beamcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunNoticeTags(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := runNoticeTags("pay")
	want := creelengine.PostTags{Topic: "dobby-run-notice/pay", Run: "pay"}
	if got != want {
		t.Errorf("runNoticeTags(pay) = %+v; want %+v", got, want)
	}
}

func TestEndedTeardownResults(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	ended := func(mutate func(entry *fabricengine.EntryOutcome)) fabricengine.OriginReport {
		entry := fabricengine.EntryOutcome{Slug: "x", State: fabricengine.EntryEnded, How: "done", Local: true}
		mutate(&entry)
		return fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{entry}}
	}
	notEnded := "origin's board does not show x ended, so the reconcile left its pair; way forward: read lym batten status x"
	tests := []struct {
		name         string
		report       fabricengine.OriginReport
		reconcileErr error
		want         string
	}{
		{"RemovedPairIsNoFailure", ended(func(e *fabricengine.EntryOutcome) { e.Removed = true }), nil, ""},
		{"BusyPairNamesItsWayForward", ended(func(e *fabricengine.EntryOutcome) { e.Busy, e.WayForward = true, "wait for the driver" }), nil, "its pair is busy; wait for the driver"},
		{"FailedTeardownNamesItsError", ended(func(e *fabricengine.EntryOutcome) { e.Error = "boom" }), nil, "its pair's teardown failed: boom"},
		{"AnotherRunsRemovalLeavesThisOne", ended(func(e *fabricengine.EntryOutcome) { e.Slug, e.Removed = "y", true }), nil, notEnded},
		{"ActiveEntryIsLeft", fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "x", State: fabricengine.EntryActive}}}, nil, notEnded},
		{"UnreachableOriginJudgesNothing", fabricengine.OriginReport{Unreachable: true}, nil, "origin was unreachable, so the reconcile judged no run ended"},
		{"ReconcileErrorFailsEveryRun", fabricengine.OriginReport{}, errors.New("ls-remote failed"), "the hub's origin reconcile failed: ls-remote failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results := endedTeardownResults([]string{"x"}, tt.report, tt.reconcileErr)
			got, reported := results["x"]
			if !reported || len(results) != 1 {
				t.Fatalf("endedTeardownResults() = %v; want one result for x", results)
			}
			gotText := ""
			if got != nil {
				gotText = got.Error()
			}
			if gotText != tt.want {
				t.Errorf("endedTeardownResults()[x] = %q; want %q", gotText, tt.want)
			}
		})
	}
}
