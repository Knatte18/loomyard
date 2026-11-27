// mutate_test.go covers which changed packages the mutation check judges and how it lays out what it found.

package gatecli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFreshMaps_SortsPackagesIntoJudgedAndNotJudged(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()
	mapsDir := t.TempDir()
	source := []byte("package calc\n")
	for _, pkg := range []string{"calc", "stale"} {
		if err := os.MkdirAll(filepath.Join(root, pkg), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, pkg, pkg+".go"), source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	freshMap := testcover.Map{Files: map[string]string{"calc.go": testcover.ContentHash(source)}}
	if err := testcover.WriteMap(mapsDir, "calc", freshMap); err != nil {
		t.Fatal(err)
	}
	if err := testcover.WriteMap(mapsDir, "stale", testcover.Map{Files: map[string]string{"stale.go": "other-hash"}}); err != nil {
		t.Fatal(err)
	}
	module := goModule{dir: root, rel: ".", path: "example.com/m"}
	packages := []string{"calc", "stale", "unmapped", "unrefreshed"}
	refresh := ReachReport{Unrefreshed: []string{"unrefreshed: the build failed"}}

	tests := []struct {
		name          string
		prime         bool
		wantFresh     []string
		wantNotJudged []MutationNotJudged
	}{
		{
			name:      "AWorktreeJudgesTheFreshMapAndGivesEachOtherPackageItsOwnReason",
			wantFresh: []string{"calc"},
			wantNotJudged: []MutationNotJudged{
				{Package: "stale", Reason: reasonWorktreeMapStale},
				{Package: "unmapped", Reason: reasonRefreshWroteNoMap},
				{Package: "unrefreshed", Reason: "the build failed"},
			},
		},
		{
			name:      "ThePrimeReadsTheSharedMapsAndIgnoresTheRefreshsUnrefreshedList",
			prime:     true,
			wantFresh: []string{"calc"},
			wantNotJudged: []MutationNotJudged{
				{Package: "stale", Reason: reasonSharedMapStale},
				{Package: "unmapped", Reason: reasonNoMapToMutate},
				{Package: "unrefreshed", Reason: reasonNoMapToMutate},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fresh, notJudged, err := freshMaps(module, mapsDir, tt.prime, packages, refresh)
			if err != nil {
				t.Fatalf("freshMaps() error = %v; want none", err)
			}
			var gotFresh []string
			for pkg := range fresh {
				gotFresh = append(gotFresh, pkg)
			}
			if diff := cmp.Diff(tt.wantFresh, gotFresh); diff != "" {
				t.Errorf("freshMaps() fresh packages mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantNotJudged, notJudged); diff != "" {
				t.Errorf("freshMaps() not judged mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFillMutateAnswer_ListsTheCheckAndTheNotJudgedPackagesOfBothStages(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	found := mutationFindings{
		outcome: mutateOutcome{
			survivors: []MutationSurvivor{{MutationMutant: MutationMutant{File: "calc/calc.go", Line: 3, Operator: "negate", Function: "Clamp"}}},
			unjudged:  []MutationUnjudgedMutant{},
			uncovered: []MutationUncoveredBlock{{Package: "calc", File: "calc.go", Function: "Clamp"}},
			notJudged: []MutationNotJudged{{Package: "calc/sub", Reason: "plan failed"}},
			counts:    MutationCounts{Run: 2, Killed: 1, Survived: 1},
			runs:      5,
		},
		notJudged:     []MutationNotJudged{{Package: "calc", Reason: "pre-check"}},
		refreshErrors: []string{"refresh: one package failed"},
	}
	want := map[string]any{
		"base":             "abc",
		"errors":           []string{"refresh: one package failed"},
		"mutants":          found.outcome.counts,
		"runs":             5,
		"survivors":        found.outcome.survivors,
		"unjudged_mutants": found.outcome.unjudged,
		"uncovered_blocks": found.outcome.uncovered,
		"not_judged":       []MutationNotJudged{{Package: "calc", Reason: "pre-check"}, {Package: "calc/sub", Reason: "plan failed"}},
	}
	got := fillMutateAnswer(map[string]any{"base": "abc"}, found)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(MutationSurvivor{})); diff != "" {
		t.Errorf("fillMutateAnswer() mismatch (-want +got):\n%s", diff)
	}
}
