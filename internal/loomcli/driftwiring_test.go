// driftwiring_test.go checks the request the loomcli Drift seams hand the detached capture child's start, the records paths the judge snapshot leaves out and the seam that ends the removed drift agents' chairs.

package loomcli_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCaptureSpawnRequest_StartsInTheAnchoredDirectory covers the directory the capture child is started in for a hub anchored at a subdirectory.
func TestCaptureSpawnRequest_StartsInTheAnchoredDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := &lymcwd.Location{HubPath: filepath.Join(t.TempDir(), "hub"), WorktreeName: "pair", AnchorRel: "backend"}
	request := loomcli.CaptureSpawnRequest(location, driftengine.NewPaths(location.AnchorPath()), "lym")

	if want := filepath.Join(location.HubPath, "pair", "backend"); request.AnchorDir != want {
		t.Errorf("CaptureSpawnRequest() AnchorDir = %q; want the anchored directory %q", request.AnchorDir, want)
	}
	if request.Owner != "pair" || request.Executable != "lym" {
		t.Errorf("CaptureSpawnRequest() = %+v; want owner pair and executable lym", request)
	}
}

// TestLoomRecordsDirs_HoldTheUsageDirectoryUnderTheAnchor covers the usage records of a run's sessions, which the judge snapshot must leave out at the path they have on the records checkout.
func TestLoomRecordsDirs_HoldTheUsageDirectoryUnderTheAnchor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
		want      string
	}{
		{"AnchoredAtTheRoot", ".", "_lym/usage"},
		{"AnchoredAtASubdirectory", "src/backend", "src/backend/_lym/usage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			location := &lymcwd.Location{HubPath: filepath.Join(t.TempDir(), "hub"), WorktreeName: "pair", AnchorRel: tt.anchorRel}

			if got := loomcli.LoomRecordsDirs(location, "run1"); !slices.Contains(got, tt.want) {
				t.Errorf("LoomRecordsDirs() = %v; want it to hold %q", got, tt.want)
			}
		})
	}
}

// TestStrandRoleStopper_StopsOnlyTheToldRoles covers which strands of reed's table the seam ends: a live or present own-window strand of exactly a told role, and no other role, numbered form, dead strand or retiring one, passing a failing table read or stop through.
func TestStrandRoleStopper_StopsOnlyTheToldRoles(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	strand := func(guid, role string, live, retiring bool) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: guid, Name: "hub:slug:" + role, Live: live, Retiring: retiring}
	}
	// shellOnly is an own-window chair of a told role whose engine exited: its pane is present and not live.
	shellOnly := strand("s1", "drift-verify", false, false)
	shellOnly.OwnWindow, shellOnly.Present = true, true
	statusErr := errors.New("reed down")
	stopErr := errors.New("tmux gone")
	roles := []string{"drift-baseline", "drift-verify"}

	tests := []struct {
		name      string
		strands   []reedengine.StrandStatus
		statusErr error
		stopErr   error
		wantStops []string
		wantErr   error
	}{
		{name: "EveryLiveChairOfAToldRoleIsStopped", strands: []reedengine.StrandStatus{strand("b1", "drift-baseline", true, false), strand("v1", "drift-verify", true, false)}, wantStops: []string{"b1", "v1"}},
		{name: "AChairOfAnotherRoleOrANumberedFormIsLeftAlone", strands: []reedengine.StrandStatus{strand("e1", "drift-expect", true, false), strand("j1", "drift-judge", true, false), strand("n1", "drift-verify-2", true, false)}},
		{name: "ADeadOrRetiringChairIsLeftAlone", strands: []reedengine.StrandStatus{strand("d1", "drift-verify", false, false), strand("r1", "drift-verify", true, true)}},
		{name: "APresentShellOnlyOwnWindowChairIsStopped", strands: []reedengine.StrandStatus{shellOnly}, wantStops: []string{"s1"}},
		{name: "ATableReadFailureIsPassedThrough", statusErr: statusErr, wantErr: statusErr},
		{name: "AStopFailureEndsThePass", strands: []reedengine.StrandStatus{strand("b1", "drift-baseline", true, false), strand("v1", "drift-verify", true, false)}, stopErr: stopErr, wantStops: []string{"b1"}, wantErr: stopErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotStops []string
			stopper := loomcli.StrandRoleStopper(
				func() (reedengine.StatusResult, error) {
					return reedengine.StatusResult{Strands: tt.strands}, tt.statusErr
				},
				func(guid string) error {
					gotStops = append(gotStops, guid)
					return tt.stopErr
				},
			)

			err := stopper(roles)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("stopper() error = %v; want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(gotStops, tt.wantStops) {
				t.Errorf("stopped = %v; want %v", gotStops, tt.wantStops)
			}
		})
	}
}
