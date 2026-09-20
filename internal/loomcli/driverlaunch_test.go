package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Compile-time assertions that the two production adapters, and shuttleengine.Run itself, satisfy
// their interfaces.
var (
	_ driverStarter   = runnerDriverStarter{}
	_ driverPaneProbe = reedDriverPaneProbe{}
	_ driverHandle    = (*shuttleengine.Run)(nil)
)

// TestReedDriverPaneProbe_RemoveDriverStrand_PassesFalseCascade asserts the reed adapter's remove
// passes false for the cascade argument: a driver strand is parentless and childless, so the
// recursive form is never the right one.
func TestReedDriverPaneProbe_RemoveDriverStrand_PassesFalseCascade(t *testing.T) {
	var gotRecursive bool
	probe := reedDriverPaneProbe{
		remove: func(guid string, recursive bool) (reedengine.Removed, error) {
			gotRecursive = recursive
			return reedengine.Removed{}, nil
		},
	}

	if err := probe.RemoveDriverStrand("g0"); err != nil {
		t.Fatalf("RemoveDriverStrand() error = %v; want nil", err)
	}
	if gotRecursive {
		t.Error("RemoveDriverStrand() called remove with recursive = true; want false -- the cascade is inert for a parentless, childless driver strand")
	}
}

// TestReedDriverPaneProbe_Strands_ReturnsStatusStrands asserts Strands delegates to the engine's own
// Status and returns its strand slice.
func TestReedDriverPaneProbe_Strands_ReturnsStatusStrands(t *testing.T) {
	want := []reedengine.StrandStatus{{GUID: "g0", Name: driverStrandDisplayName, Live: true}}
	probe := reedDriverPaneProbe{
		status: func() (reedengine.StatusResult, error) {
			return reedengine.StatusResult{Strands: want}, nil
		},
	}

	got, err := probe.Strands()
	if err != nil {
		t.Fatalf("Strands() error = %v; want nil", err)
	}
	if len(got) != 1 || got[0].GUID != "g0" {
		t.Errorf("Strands() = %+v; want %+v", got, want)
	}
}
