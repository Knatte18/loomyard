//go:build tier0

// land_test.go covers the Land row's pure wiring helpers: which board tasks count as ready and which orch each newly ready one is routed to.

package battencli

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestPullStopOutcome asserts a lone records-side stop comes back as its message with a nil error, and a stop joined with a code-side error or any other error comes back as that error with no stop.
func TestPullStopOutcome(t *testing.T) {
	t.Parallel()

	stop := fabricengine.NewRecordsStopError("main", errors.New("replay conflict"), "settle it")
	codeSideErr := errors.New("code side fetch failed")
	joined := errors.Join(stop, codeSideErr)
	unrelated := errors.New("open fabric failed")

	tests := []struct {
		name        string
		err         error
		wantStop    string
		wantPullErr error
	}{
		{"LoneRecordsStop", stop, stop.Error(), nil},
		{"RecordsStopJoinedWithCodeSideError", joined, "", joined},
		{"UnrelatedError", unrelated, "", unrelated},
		{"NoError", nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStop, gotErr := pullStopOutcome(tt.err)
			if gotStop != tt.wantStop || gotErr != tt.wantPullErr {
				t.Errorf("pullStopOutcome(%v) = %q, %v; want %q, %v", tt.err, gotStop, gotErr, tt.wantStop, tt.wantPullErr)
			}
		})
	}
}

// TestReadySlugs asserts readySlugs lists, sorted, only the unclaimed tasks with no dependency, and skips a note, a claimed task, a done task and a task with a dependency.
func TestReadySlugs(t *testing.T) {
	t.Parallel()

	claimed, done := "running", "done"
	tasks := []boardengine.Task{
		{Slug: "zeta", Kind: boardengine.KindTask},
		{Slug: "alpha", Kind: boardengine.KindTask, DependsOn: []string{}},
		{Slug: "a-note", Kind: boardengine.KindNote},
		{Slug: "claimed", Kind: boardengine.KindTask, Status: &claimed},
		{Slug: "finished", Kind: boardengine.KindTask, Status: &done},
		{Slug: "blocked", Kind: boardengine.KindTask, DependsOn: []string{"alpha"}},
	}

	if got, want := readySlugs(tasks), []string{"alpha", "zeta"}; !slices.Equal(got, want) {
		t.Errorf("readySlugs() = %v; want %v", got, want)
	}
}

// TestReadyRoutes asserts readyRoutes sends a task outside any beam to the prime, a member of a beam with a live orch to that beam, and a member of a beam whose orch is stopped or whose worktree is absent to no one, and returns a liveness error.
func TestReadyRoutes(t *testing.T) {
	t.Parallel()

	parents := map[string]string{"member": "b", "stopped-member": "stopped", "absent-member": "absent", "broken-member": "broken"}
	parentBeam := func(slug string) string { return parents[slug] }
	livenessErr := errors.New("reed unreachable")
	beamOrchLive := func(beam string) (bool, error) {
		switch beam {
		case "b":
			return true, nil
		case "broken":
			return false, livenessErr
		default:
			return false, nil
		}
	}

	tests := []struct {
		name      string
		ready     []string
		wantPrime []string
		wantBeams map[string][]string
		wantErr   error
	}{
		{name: "PrimeTaskGoesToThePrime", ready: []string{"plain"}, wantPrime: []string{"plain"}, wantBeams: map[string][]string{}},
		{name: "MemberOfALiveBeamGoesToTheBeam", ready: []string{"member"}, wantBeams: map[string][]string{"b": {"member"}}},
		{name: "MemberOfAStoppedBeamGoesNowhere", ready: []string{"stopped-member"}, wantBeams: map[string][]string{}},
		{name: "MemberOfAnAbsentBeamGoesNowhere", ready: []string{"absent-member"}, wantBeams: map[string][]string{}},
		{name: "LivenessErrorIsReturned", ready: []string{"broken-member"}, wantErr: livenessErr},
		{name: "MixedReadySplitsByDestination", ready: []string{"member", "plain", "stopped-member"}, wantPrime: []string{"plain"}, wantBeams: map[string][]string{"b": {"member"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prime, beams, err := readyRoutes(tt.ready, parentBeam, beamOrchLive)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("readyRoutes(%v) error = %v; want %v", tt.ready, err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if !slices.Equal(prime, tt.wantPrime) {
				t.Errorf("readyRoutes(%v) prime = %v; want %v", tt.ready, prime, tt.wantPrime)
			}
			if !maps.EqualFunc(beams, tt.wantBeams, slices.Equal) {
				t.Errorf("readyRoutes(%v) beams = %v; want %v", tt.ready, beams, tt.wantBeams)
			}
		})
	}
}
