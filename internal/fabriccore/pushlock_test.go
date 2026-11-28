// pushlock_test.go tests the rules of the records push lock: the holder clause, the progress test and the holder loop.

package fabriccore_test

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDescribePushLockHolder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	since := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	now := since.Add(95 * time.Second)
	holder := fabriccore.PushLockHolder{PID: 4242, Since: since}

	tests := []struct {
		name    string
		holder  fabriccore.PushLockHolder
		found   bool
		readErr error
		alive   bool
		want    string
	}{
		{"LiveHolder", holder, true, nil, true, "held by pid 4242 for 1m35s"},
		{"DeadHolder", holder, true, nil, false, "its holder record names pid 4242 from 1m35s ago, no running process by that pid was found, and the record may be stale"},
		{"EmptyObject", fabriccore.PushLockHolder{}, true, nil, false, "its holder record is unreadable: it names no pid or no time the lock was taken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabriccore.DescribePushLockHolder(tt.holder, tt.found, tt.readErr, now, tt.alive); got != tt.want {
				t.Errorf("DescribePushLockHolder() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestSidesProgressed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name          string
		before, after map[string]string
		want          bool
	}{
		{"NoSideMoved", map[string]string{"code": "a", "records": "b"}, map[string]string{"code": "a", "records": "b"}, false},
		{"OneSideMoved", map[string]string{"code": "a", "records": "b"}, map[string]string{"code": "a", "records": "c"}, true},
		{"ASideTurnedUnborn", map[string]string{"code": "a"}, map[string]string{}, true},
		{"EmptyMaps", map[string]string{}, map[string]string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fabriccore.SidesProgressed(tt.before, tt.after); got != tt.want {
				t.Errorf("SidesProgressed(%v, %v) = %v; want %v", tt.before, tt.after, got, tt.want)
			}
		})
	}
}

func TestFlaggedLoopNext(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name                            string
		stepRan, acquired, ownedFlagSet bool
		wantAgain, wantReportAcquired   bool
	}{
		{"MissedFirstAcquireReportsNotAcquired", false, false, false, false, false},
		{"MissedReacquireReportsAcquired", true, false, false, false, true},
		{"FlagSetAgainAfterAStepAcquiresAgain", false, true, true, true, true},
		{"NoFlagAfterAStepEnds", true, true, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			again, reportAcquired := fabriccore.FlaggedLoopNext(tt.stepRan, tt.acquired, tt.ownedFlagSet)
			if again != tt.wantAgain || reportAcquired != tt.wantReportAcquired {
				t.Errorf("FlaggedLoopNext(%v, %v, %v) = %v, %v; want %v, %v",
					tt.stepRan, tt.acquired, tt.ownedFlagSet, again, reportAcquired, tt.wantAgain, tt.wantReportAcquired)
			}
		})
	}
}
