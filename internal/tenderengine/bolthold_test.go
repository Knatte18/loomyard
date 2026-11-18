// bolthold_test.go drives SyncBoltHold over the fake pairs and the temp record pause_test.go declares.

package tenderengine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSyncBoltHold(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	now := newFakeClock().Now()
	entry := func(scope, caller string) pauserecord.Entry {
		return pauserecord.Entry{Scope: scope, PausedAt: now, GraceEnd: now, Caller: caller}
	}
	boltHold := func(scope string) pauserecord.Entry {
		held := entry(scope, pauserecord.BoltHoldCaller)
		held.Machine = "oslo"
		return held
	}
	lockReadFails := errors.New("lock store unreadable")

	tests := []struct {
		name        string
		live        []pauserecord.Entry
		req         BoltHoldRequest
		wantWritten []string
		wantDropped []string
		wantLive    []string
		wantErr     string
	}{
		{
			name:        "WritesAnEntryForEachHeldSlugAndNoneForAnUnheldOne",
			req:         BoltHoldRequest{Machine: "oslo", Standing: true, Hold: []string{"p1", "p2"}, Scope: []string{"p1", "p2", "p3"}},
			wantWritten: []string{"p1", "p2"},
			wantLive:    []string{"p1:bolt-hold", "p2:bolt-hold"},
		},
		{
			name:        "WritesNoneForASlugWithALiveEntryOfItsOwn",
			live:        []pauserecord.Entry{entry("p1", "EXTERNAL")},
			req:         BoltHoldRequest{Machine: "oslo", Standing: true, Hold: []string{"p1", "p2"}, Scope: []string{"p1", "p2"}},
			wantWritten: []string{"p2"},
			wantLive:    []string{"p1:EXTERNAL", "p2:bolt-hold"},
		},
		{
			name:        "DropsTheEntryOfASlugThatLeftTheHoldAndLeavesAHubEntryAndAnOperatorsEntryStanding",
			live:        []pauserecord.Entry{boltHold("p1"), boltHold("p3"), entry(pauserecord.ScopeHub, "EXTERNAL"), entry("p2", "EXTERNAL")},
			req:         BoltHoldRequest{Machine: "oslo", Standing: true, Hold: []string{"p1"}, Scope: []string{"p1", "p2", "p3"}},
			wantDropped: []string{"p3"},
			wantLive:    []string{"hub:EXTERNAL", "p1:bolt-hold", "p2:EXTERNAL"},
		},
		{
			name:        "DropsEveryBoltHoldEntryOnceTheHoldNoLongerStands",
			live:        []pauserecord.Entry{boltHold("p1"), boltHold("p2"), entry("p3", "EXTERNAL")},
			req:         BoltHoldRequest{Machine: "oslo", Hold: []string{"p1", "p2"}, Scope: []string{"p1", "p2", "p3"}},
			wantDropped: []string{"p1", "p2"},
			wantLive:    []string{"p3:EXTERNAL"},
		},
		{
			name:        "ReplacesABoltHoldEntryOfAnotherMachineWithOneOfTheHoldsMachine",
			live:        []pauserecord.Entry{boltHold("p1")},
			req:         BoltHoldRequest{Machine: "bergen", Standing: true, Hold: []string{"p1"}, Scope: []string{"p1"}},
			wantWritten: []string{"p1"},
			wantLive:    []string{"p1:bolt-hold"},
		},
		{
			name: "ASlugHoldingItsParentLockGetsNoEntryAndItsStandingEntryIsLifted",
			live: []pauserecord.Entry{boltHold("p1")},
			req: BoltHoldRequest{
				Machine: "oslo", Standing: true, Hold: []string{"p1", "p2"}, Scope: []string{"p1", "p2"},
				HoldsParentLock: func(slug string) (bool, error) { return slug == "p1", nil },
			},
			wantWritten: []string{"p2"},
			wantDropped: []string{"p1"},
			wantLive:    []string{"p2:bolt-hold"},
		},
		{
			name: "AFailedParentLockReadLeavesThatSlugsEntriesAndIsJoinedIntoTheError",
			live: []pauserecord.Entry{boltHold("p1")},
			req: BoltHoldRequest{
				Machine: "oslo", Standing: true, Hold: []string{"p1", "p2"}, Scope: []string{"p1", "p2"},
				HoldsParentLock: func(slug string) (bool, error) {
					if slug == "p1" {
						return false, lockReadFails
					}
					return false, nil
				},
			},
			wantWritten: []string{"p2"},
			wantLive:    []string{"p1:bolt-hold", "p2:bolt-hold"},
			wantErr:     "p1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, newPair("p1"), newPair("p2"), newPair("p3"))
			e.seedLive(tt.live...)

			report, err := SyncBoltHold(e.deps(), tt.req)

			if tt.wantErr == "" && err != nil {
				t.Fatalf("SyncBoltHold() error = %v; want none", err)
			}
			if tt.wantErr != "" && (err == nil || !errors.Is(err, lockReadFails) || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("SyncBoltHold() error = %v; want the lock read's error naming %s", err, tt.wantErr)
			}
			if !slices.Equal(report.Written, tt.wantWritten) || !slices.Equal(report.Dropped, tt.wantDropped) {
				t.Errorf("SyncBoltHold() = written %v, dropped %v; want %v and %v", report.Written, report.Dropped, tt.wantWritten, tt.wantDropped)
			}
			var live []string
			for _, entry := range e.record().Live {
				live = append(live, entry.Scope+":"+entry.Caller)
				if entry.Caller == pauserecord.BoltHoldCaller && entry.Machine != tt.req.Machine {
					t.Errorf("bolt-hold entry of %s records machine %q; want %s", entry.Scope, entry.Machine, tt.req.Machine)
				}
			}
			slices.Sort(live)
			if !slices.Equal(live, tt.wantLive) {
				t.Errorf("live entries = %v; want %v", live, tt.wantLive)
			}
		})
	}
}
