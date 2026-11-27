// remove_test.go covers the verdict `lym fabric remove` takes from an origin reconcile report.
// It also covers the remover's choice between the local removal and the ordinary one over fake seams.

package fabriccli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const goneWayForward = "`lym fabric remove` of the pair removes the pair and clears this record"

func branch(repo, slug string, prime bool, state fabriclayout.BranchState, wayForward string) fabriclayout.BranchOutcome {
	return fabriclayout.BranchOutcome{Branch: repo + "/" + string(state), Repo: repo, Slug: slug, Prime: prime, State: state, WayForward: wayForward}
}

func reportOf(entry fabriclayout.EntryOutcome, branches ...fabriclayout.BranchOutcome) fabriclayout.OriginReport {
	return fabriclayout.OriginReport{Entries: []fabriclayout.EntryOutcome{entry}, Branches: branches}
}

func TestRemoveVerdict(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stopped := fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryStopped, WayForward: goneWayForward}
	goneCode := branch("code", "s", false, fabriclayout.BranchGone, goneWayForward)
	divergedRecords := branch("records", "s", false, fabriclayout.BranchDiverged, "WF-RECORDS")
	divergedBolt := branch("bolt", "", false, fabriclayout.BranchDiverged, "WF-BOLT")
	busyBolt := branch("bolt", "", false, fabriclayout.BranchBusy, "WF-BUSY-BOLT")
	busyOwnRecords := branch("records", "s", false, fabriclayout.BranchBusy, "WF-BUSY-RECORDS")

	tests := []struct {
		name    string
		report  fabriclayout.OriginReport
		local   localReading
		want    fabriclayout.EntryState
		refusal []string
		// notInRefusal are texts the refusal must not carry.
		notInRefusal []string
	}{
		{
			name:    "StoppedRefusesNamingTheWayForwardAndTheRerun",
			report:  reportOf(stopped, branch("code", "s", false, fabriclayout.BranchDiverged, "WF-CODE")),
			refusal: []string{"WF-CODE", "lym fabric remove s again"},
		},
		{
			name:   "StoppedOnlyByAGoneCodeBranchIsTheOrdinaryRemoval",
			report: reportOf(stopped, goneCode),
		},
		{
			name:   "GoneCodeBranchBesideABusyBoltIsTheOrdinaryRemoval",
			report: reportOf(stopped, goneCode, busyBolt),
		},
		{
			name:         "GoneThenDivergedRecordsRefusesNamingTheDivergedBranch",
			report:       reportOf(stopped, goneCode, divergedRecords),
			refusal:      []string{"WF-RECORDS", "lym fabric remove s again"},
			notInRefusal: []string{"removes the pair and clears this record"},
		},
		{
			name:         "GoneThenDivergedBoltRefusesNamingTheBoltBranch",
			report:       reportOf(stopped, goneCode, divergedBolt),
			refusal:      []string{"WF-BOLT", "lym fabric remove s again"},
			notInRefusal: []string{"removes the pair and clears this record"},
		},
		{
			name:         "GoneThenOwnBusyRecordsThenDivergedBoltRefusesNamingTheBoltBranch",
			report:       reportOf(stopped, goneCode, busyOwnRecords, divergedBolt),
			refusal:      []string{"WF-BOLT", "lym fabric remove s again"},
			notInRefusal: []string{"removes the pair and clears this record", "WF-BUSY-RECORDS"},
		},
		{
			name:   "EndedPassesThrough",
			report: reportOf(fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryEnded}),
			want:   fabriclayout.EntryEnded,
		},
		{
			name:   "OwnedElsewherePassesThrough",
			report: reportOf(fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryOwnedElsewhere, Owner: "other"}),
			want:   fabriclayout.EntryOwnedElsewhere,
		},
		{
			name:   "ActiveIsTheOrdinaryRemoval",
			report: reportOf(fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryActive}),
		},
		{
			name:   "NoEntryIsTheOrdinaryRemoval",
			report: fabriclayout.OriginReport{},
		},
		{
			name:   "ParentPairsEntryIsNotTheSlugs",
			report: reportOf(fabriclayout.EntryOutcome{Slug: "parent", State: fabriclayout.EntryStopped, WayForward: "WF-PARENT"}, divergedRecords),
		},
		{
			name:   "UnreachableWithAForeignLocalOwnerIsOwnedElsewhere",
			report: fabriclayout.OriginReport{Unreachable: true},
			local:  localReading{ownedElsewhere: true, done: true},
			want:   fabriclayout.EntryOwnedElsewhere,
		},
		{
			name:   "UnreachableWithALocalDoneEntryIsEnded",
			report: fabriclayout.OriginReport{Unreachable: true},
			local:  localReading{done: true},
			want:   fabriclayout.EntryEnded,
		},
		{
			name:   "UnreachableWithNeitherReadingIsTheOrdinaryRemoval",
			report: fabriclayout.OriginReport{Unreachable: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := removeVerdict(tt.report, "s", tt.local)
			if len(tt.refusal) == 0 {
				if err != nil {
					t.Fatalf("removeVerdict() error = %v, want none", err)
				}
				if got != tt.want {
					t.Errorf("removeVerdict() = %q, want %q", got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("removeVerdict() = %q, nil error; want a refusal", got)
			}
			for _, text := range tt.refusal {
				if !strings.Contains(err.Error(), text) {
					t.Errorf("refusal %q lacks %q", err, text)
				}
			}
			for _, text := range tt.notInRefusal {
				if strings.Contains(err.Error(), text) {
					t.Errorf("refusal %q carries %q", err, text)
				}
			}
		})
	}
}

func TestRemover(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	ordinary := pairteardown.Request{Slug: "s", Force: true, Remote: true, QuietWait: pairteardown.RemoveQuietWait, RefuseWhenBusy: true}
	endedReport := reportOf(fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryEnded})
	stopped := reportOf(
		fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryStopped},
		branch("code", "s", false, fabriclayout.BranchDiverged, "WF-CODE"),
	)
	goneOnly := reportOf(
		fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryStopped},
		branch("code", "s", false, fabriclayout.BranchGone, goneWayForward),
	)

	tests := []struct {
		name         string
		gone         bool
		report       fabriclayout.OriginReport
		reconcileErr error
		local        localReading
		// wantCalls are the seam calls between the exemption and its restore, in order.
		wantCalls []string
		// wantRequest is the request the teardown saw; nil when it was never called.
		wantRequest  *pairteardown.Request
		wantErr      string
		wantRemoteBy string
	}{
		{
			name:         "EndedVerdictRunsTheLocalRequestAndNamesTheDroppedRemote",
			report:       endedReport,
			wantCalls:    []string{"gone", "reconcile", "teardown"},
			wantRequest:  &pairteardown.Request{Slug: "s", Force: true, Local: true, Verdict: fabriclayout.EntryEnded, RefuseWhenBusy: true},
			wantRemoteBy: "the origin reconcile read the entry ended, so the removal was local and --remote was dropped",
		},
		{
			name:         "OwnedElsewhereVerdictRunsTheLocalRequest",
			report:       reportOf(fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryOwnedElsewhere}),
			wantCalls:    []string{"gone", "reconcile", "teardown"},
			wantRequest:  &pairteardown.Request{Slug: "s", Force: true, Local: true, Verdict: fabriclayout.EntryOwnedElsewhere, RefuseWhenBusy: true},
			wantRemoteBy: "the origin reconcile read the entry owned_elsewhere, so the removal was local and --remote was dropped",
		},
		{
			name:        "ActiveVerdictRunsTheOrdinaryRequest",
			report:      reportOf(fabriclayout.EntryOutcome{Slug: "s", State: fabriclayout.EntryActive}),
			wantCalls:   []string{"gone", "reconcile", "teardown"},
			wantRequest: &ordinary,
		},
		{
			name:         "ReconcileErrorRefusesWithNoTeardown",
			reconcileErr: errors.New("origin exploded"),
			wantCalls:    []string{"gone", "reconcile"},
			wantErr:      "origin exploded",
		},
		{
			name:      "StoppedVerdictRefusesWithNoTeardown",
			report:    stopped,
			wantCalls: []string{"gone", "reconcile"},
			wantErr:   "WF-CODE",
		},
		{
			name:        "GoneBranchStopRunsTheOrdinaryRequest",
			report:      goneOnly,
			wantCalls:   []string{"gone", "reconcile", "teardown"},
			wantRequest: &ordinary,
		},
		{
			name:         "GoneWorktreeWithAForeignLocalOwnerRunsNoReconcileAndTheLocalRequest",
			gone:         true,
			local:        localReading{ownedElsewhere: true},
			wantCalls:    []string{"gone", "local", "teardown"},
			wantRequest:  &pairteardown.Request{Slug: "s", Force: true, Local: true, Verdict: fabriclayout.EntryOwnedElsewhere, RefuseWhenBusy: true},
			wantRemoteBy: "the origin reconcile read the entry owned_elsewhere, so the removal was local and --remote was dropped",
		},
		{
			name:        "GoneWorktreeWithNeitherReadingRunsNoReconcileAndTheOrdinaryRequest",
			gone:        true,
			wantCalls:   []string{"gone", "local", "teardown"},
			wantRequest: &ordinary,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls []string
			var seen *pairteardown.Request
			r := remover{
				exemptAll: func() func() {
					calls = append(calls, "exempt")
					return func() { calls = append(calls, "restore") }
				},
				gone: func(string) (bool, error) {
					calls = append(calls, "gone")
					return tt.gone, nil
				},
				reconcile: func(string) (fabriclayout.OriginReport, error) {
					calls = append(calls, "reconcile")
					return tt.report, tt.reconcileErr
				},
				local: func(string) (localReading, error) {
					calls = append(calls, "local")
					return tt.local, nil
				},
				teardown: func(_ context.Context, req pairteardown.Request) (pairteardown.Result, error) {
					calls = append(calls, "teardown")
					seen = &req
					return pairteardown.Result{}, nil
				},
			}

			res, err := r.run(context.Background(), "s", true, true)

			wantCalls := append(append([]string{"exempt"}, tt.wantCalls...), "restore")
			if strings.Join(calls, ",") != strings.Join(wantCalls, ",") {
				t.Errorf("calls = %v, want %v", calls, wantCalls)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("run() error = %v, want one containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("run() error = %v, want none", err)
			}
			switch {
			case tt.wantRequest == nil && seen != nil:
				t.Errorf("teardown saw %+v, want no call", *seen)
			case tt.wantRequest != nil && (seen == nil || !reflect.DeepEqual(*seen, *tt.wantRequest)):
				t.Errorf("teardown request = %+v, want %+v", seen, *tt.wantRequest)
			}
			if got := res.Removal.RemoteSkippedReason; got != tt.wantRemoteBy {
				t.Errorf("RemoteSkippedReason = %q, want %q", got, tt.wantRemoteBy)
			}
		})
	}
}
