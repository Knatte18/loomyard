// landingdeps_test.go drift-guards landingDeps: it asserts every field of the returned
// landingshed.Deps is populated, via a reflection-based walk rather than an enumerated list of field
// assertions, so a newly added field is caught automatically rather than silently passing. This test
// needs no fixture, no hubforge, and no git init -- landingDeps performs no I/O -- so it stays Tier 1.

package loomcli

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestLandingDeps_EveryFieldPopulated asserts landingDeps populates every field of the
// landingshed.Deps it returns, walking the struct by reflection so a sixteenth field added later is
// caught automatically rather than silently passing an enumerated list of assertions.
//
//testtiming:keep drift guard walking every landingshed.Deps field by reflection so a field added later must be populated; the covering mark-done test populates only the fields it uses
func TestLandingDeps_EveryFieldPopulated(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	geom := websterengine.Geometry{
		WebsterDir:  "/webster",
		StencilsDir: "/stencils",
	}
	pushBranch := func() error { return nil }
	registry := modelspec.Registry{"claude/sonnet-5": {}}
	runner := &shuttleengine.Runner{}
	cfg := landingshed.Config{Squash: true}

	deps := landingDeps(
		loc,
		geom,
		"task/foo",
		"https://example.com/origin.git",
		"main",
		true,
		pushBranch,
		registry,
		runner,
		cfg,
		"parent-session",
		func(string, time.Time) error { return nil },
		func() (string, error) { return "", nil },
		planVerifySource(loc),
	)

	if deps.ParentName != "parent-session" {
		t.Errorf("landingDeps(...).ParentName = %q; want the told parent name", deps.ParentName)
	}

	v := reflect.ValueOf(deps)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.IsZero() {
			t.Errorf("landingDeps(...).%s is the zero value; want it populated", typ.Field(i).Name)
		}
	}
}

// TestDriverWaitMark asserts the callback marks the driver strand only while it is live and not retiring, is a no-op returning nil for a gone, dead or retiring one, and passes a failing strand read or mark op through.
func TestDriverWaitMark(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC)
	driver := func(live, retiring bool) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: "driver-guid", Name: loomengine.LoomDriverStrandName, Live: live, Retiring: retiring}
	}
	other := reedengine.StrandStatus{GUID: "other-guid", Name: "someone:else", Live: true}
	markErr := errors.New("tmux gone")
	statusErr := errors.New("reed down")

	tests := []struct {
		name      string
		strands   []reedengine.StrandStatus
		statusErr error
		markErr   error
		wantGUID  string
		wantErr   error
	}{
		{name: "live driver is marked", strands: []reedengine.StrandStatus{other, driver(true, false)}, wantGUID: "driver-guid"},
		{name: "gone driver is a no-op", strands: []reedengine.StrandStatus{other}},
		{name: "dead driver is a no-op", strands: []reedengine.StrandStatus{driver(false, false)}},
		{name: "retiring driver is a no-op", strands: []reedengine.StrandStatus{driver(true, true)}},
		{name: "mark failure is passed through", strands: []reedengine.StrandStatus{driver(true, false)}, markErr: markErr, wantGUID: "driver-guid", wantErr: markErr},
		{name: "status failure is passed through", statusErr: statusErr, wantErr: statusErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotGUID, gotLabel string
			var gotStart time.Time
			mark := driverWaitMark(
				func() (reedengine.StatusResult, error) {
					return reedengine.StatusResult{Strands: tt.strands}, tt.statusErr
				},
				func(guid, label string, started time.Time) error {
					gotGUID, gotLabel, gotStart = guid, label, started
					return tt.markErr
				},
			)
			err := mark("verify Publish", start)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if gotGUID != tt.wantGUID {
				t.Fatalf("marked strand = %q, want %q", gotGUID, tt.wantGUID)
			}
			if tt.wantGUID != "" && (gotLabel != "verify Publish" || !gotStart.Equal(start)) {
				t.Errorf("mark = (%q, %v), want (%q, %v)", gotLabel, gotStart, "verify Publish", start)
			}
		})
	}
}

// TestConflictSessionStopper asserts the seam stops every live, non-retiring conflict strand of the run by guid, numbered forms included.
// It stops nothing for a dead, retiring or absent one, and passes a failing table read or stop through with the guid the failure belongs to.
func TestConflictSessionStopper(t *testing.T) {
	t.Parallel()

	conflict := func(guid, role string, live, retiring bool) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: guid, Name: "hub:slug:" + role, Live: live, Retiring: retiring}
	}
	other := reedengine.StrandStatus{GUID: "other-guid", Name: "hub:slug:webster", Live: true}
	statusErr := errors.New("reed down")
	stopErr := errors.New("tmux gone")

	tests := []struct {
		name      string
		strands   []reedengine.StrandStatus
		statusErr error
		stopErr   error
		wantStops []string
		wantGUID  string
		wantErr   error
	}{
		{name: "live conflict strand is stopped", strands: []reedengine.StrandStatus{other, conflict("c1", "conflict", true, false)}, wantStops: []string{"c1"}},
		{name: "live numbered conflict strand is stopped", strands: []reedengine.StrandStatus{conflict("c2", "conflict-2", true, false)}, wantStops: []string{"c2"}},
		{name: "every live conflict strand is stopped", strands: []reedengine.StrandStatus{conflict("c1", "conflict", true, false), conflict("c2", "conflict-2", true, false)}, wantStops: []string{"c1", "c2"}},
		{name: "dead conflict strand stops nothing", strands: []reedengine.StrandStatus{conflict("c1", "conflict", false, false)}},
		{name: "retiring conflict strand stops nothing", strands: []reedengine.StrandStatus{conflict("c1", "conflict", true, true)}},
		{name: "no conflict strand stops nothing", strands: []reedengine.StrandStatus{other}},
		{name: "table read failure is passed through with an empty guid", statusErr: statusErr, wantErr: statusErr},
		{name: "stop failure ends the pass with the failing guid", strands: []reedengine.StrandStatus{conflict("c1", "conflict", true, false), conflict("c2", "conflict-2", true, false)}, stopErr: stopErr, wantStops: []string{"c1"}, wantGUID: "c1", wantErr: stopErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotStops []string
			stopper := conflictSessionStopper(
				func() (reedengine.StatusResult, error) {
					return reedengine.StatusResult{Strands: tt.strands}, tt.statusErr
				},
				func(guid string) error {
					gotStops = append(gotStops, guid)
					return tt.stopErr
				},
			)
			guid, err := stopper()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if guid != tt.wantGUID {
				t.Errorf("guid = %q, want %q", guid, tt.wantGUID)
			}
			if !reflect.DeepEqual(gotStops, tt.wantStops) {
				t.Errorf("stopped = %v, want %v", gotStops, tt.wantStops)
			}
		})
	}
}
