// redundantmutants_guard_test.go covers how the mutation check treats a run the memory guard killed.
// The test binary re-executes itself as the stand-in coverage binary, so no real build or mutant runs.

package gatecli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMutantStandIn is the test the stand-in binary runs as a candidate: it fails once mutant 1 is on.
func TestMutantStandIn(t *testing.T) {
	tiergate.Require(t, 0)
	if os.Getenv(testcover.MutantEnv) == "1" {
		t.Fatal("mutant 1 is on")
	}
}

// TestMutantStandInCovering is the test the stand-in binary runs as the candidate's covering test.
func TestMutantStandInCovering(t *testing.T) {
	tiergate.Require(t, 0)
}

// standInMarkerEnv names the file TestMutantStandInHangsOnce creates on its first run with mutant 1 on.
const standInMarkerEnv = "GATECLI_STAND_IN_MARKER"

// TestMutantStandInHangsOnce is a stand-in test that hangs on its first run with mutant 1 on and fails on each later one.
func TestMutantStandInHangsOnce(t *testing.T) {
	tiergate.Require(t, 0)
	marker := os.Getenv(standInMarkerEnv)
	if os.Getenv(testcover.MutantEnv) != "1" || marker == "" {
		return
	}
	if file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err == nil {
		file.Close()
		// The check's time bound kills this run long before the hour passes.
		<-time.After(time.Hour)
	}
	t.Fatal("mutant 1 is on")
}

// TestRunMutantsRerunsACoveringTestThatTimedOutAsACandidate pins the rerun of a covering test.
// Its candidate run of a mutant ended at the time bound, so it runs that mutant again.
// The rerun's answer replaces the timeout.
func TestRunMutantsRerunsACoveringTestThatTimedOutAsACandidate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const killer, hangsOnce = "TestMutantStandIn", "TestMutantStandInHangsOnce"
	block := testcover.BlockKey{File: "x.go", Func: "F", Hash: "h"}
	m := testcover.Map{
		Blocks: []testcover.BlockKey{block},
		Tests:  []testcover.MapTest{{Name: killer, Blocks: []int{0}}, {Name: hangsOnce, Blocks: []int{0}}},
	}
	holderEnv := append(os.Environ(), standInMarkerEnv+"="+filepath.Join(t.TempDir(), "marker"))
	check := &mutationCheck{
		ctx:         context.Background(),
		request:     redundantRequest{secondUnit: time.Second, grace: time.Second},
		tiers:       map[int]gateslot.Tier{0: {}},
		pkgDir:      t.TempDir(),
		testTimeout: time.Minute,
		sideBySide:  1,
		env:         holderEnv,
		holderEnv:   holderEnv,
		mutants:     map[int]testcover.Mutant{1: {ID: 1, Block: block.String()}},
	}
	candidates := []testcover.MutantCandidate{
		{Name: killer, Covering: []string{hangsOnce}, Mutants: []int{1}},
		{Name: hangsOnce, Covering: []string{killer}, Mutants: []int{1}},
	}

	kills, unjudged, err := check.runMutants(bin, m, mapEntries(m), candidates)

	if err != nil {
		t.Fatalf("runMutants() error = %v; want none", err)
	}
	if !kills[1][hangsOnce] {
		t.Errorf("runMutants() kill of mutant 1 by %s = false; want true, from its rerun", hangsOnce)
	}
	if unjudged[1][hangsOnce] {
		t.Errorf("runMutants() holds %s unjudged on mutant 1; want its candidate-run timeout cleared by the rerun", hangsOnce)
	}
}

// TestRunMutantsGuardKilledRunIsNoKill pins what the mutation check records of a run that fails with a mutant on.
// A failure the memory guard caused counts as no kill and leaves the candidate not judged, never as a failing candidate.
// A failure it did not cause is the candidate's kill of the mutant.
func TestRunMutantsGuardKilledRunIsNoKill(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		guardKilled bool
		wantKill    bool
	}{
		{name: "AGuardKilledRunRecordsNoKillAndMarksTheCandidate", guardKilled: true, wantKill: false},
		{name: "AFailureTheGuardDidNotCauseIsAKill", guardKilled: false, wantKill: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bin, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			block := testcover.BlockKey{File: "x.go", Func: "F", Hash: "h"}
			m := testcover.Map{
				Blocks: []testcover.BlockKey{block},
				Tests: []testcover.MapTest{
					{Name: "TestMutantStandIn", Blocks: []int{0}},
					{Name: "TestMutantStandInCovering", Blocks: []int{0}},
				},
			}
			check := &mutationCheck{
				ctx:         context.Background(),
				request:     redundantRequest{secondUnit: time.Second},
				tiers:       map[int]gateslot.Tier{0: {}},
				pkgDir:      t.TempDir(),
				testTimeout: time.Minute,
				sideBySide:  1,
				env:         os.Environ(),
				holderEnv:   os.Environ(),
				mutants:     map[int]testcover.Mutant{1: {ID: 1, Block: block.String()}},
				guardKilled: func(int) bool { return tt.guardKilled },
			}
			candidates := []testcover.MutantCandidate{
				{Name: "TestMutantStandIn", Covering: []string{"TestMutantStandInCovering"}, Mutants: []int{1}},
			}

			kills, _, err := check.runMutants(bin, m, mapEntries(m), candidates)

			if err != nil {
				t.Fatalf("runMutants() error = %v; want none", err)
			}
			if got := kills[1]["TestMutantStandIn"]; got != tt.wantKill {
				t.Errorf("runMutants() kill of mutant 1 by the candidate = %v; want %v", got, tt.wantKill)
			}
			if got := candidates[0].GuardKilled; got != tt.guardKilled {
				t.Errorf("runMutants() candidate GuardKilled = %v; want %v", got, tt.guardKilled)
			}
			if candidates[0].FailsUnmutated {
				t.Error("runMutants() candidate FailsUnmutated = true; want false, as the candidate passes with no mutant on")
			}
		})
	}
}
