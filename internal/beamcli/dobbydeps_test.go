// dobbydeps_test.go pins which pause entries the dobby's owner-pause lift removes.
// It also pins how the dobby reads a hub reconcile's report on the ended runs it asked to tear down.

package beamcli

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
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

func TestLiftOwnerPause_LeavesAnAdoptOrHoldEntryStanding(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, caller := range []string{pauserecord.AdoptCaller, pauserecord.HoldCaller} {
		t.Run(caller, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "pause.json")
			entry := pauserecord.Entry{Scope: "alpha", Caller: caller, PausedAt: time.Unix(1000, 0)}
			if _, err := pauserecord.Update(path, time.Unix(2000, 0), func(record *pauserecord.Record) error {
				record.Live = []pauserecord.Entry{entry}
				return nil
			}); err != nil {
				t.Fatalf("Update(pause record) = %v", err)
			}

			if err := liftOwnerPause(nil, path, "alpha"); err != nil {
				t.Fatalf("liftOwnerPause() = %v, want none", err)
			}
			record, err := pauserecord.Read(path)
			if err != nil {
				t.Fatalf("Read(pause record) = %v", err)
			}
			if len(record.Live) != 1 || record.Live[0].Caller != caller || len(record.Resumed) != 0 {
				t.Errorf("record = live %+v, resumed %+v; want the %s entry standing", record.Live, record.Resumed, caller)
			}
		})
	}
}
