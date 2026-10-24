//go:build tier0

// beamfork_test.go covers beamForkPrecondition over told reads: each refusal that stops a beam member's create, the pass, and a probe error.

package battencli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
)

// TestBeamForkPrecondition asserts a beam member's create is stopped with a precondition naming the beam's create or adopt when the beam's pair is absent here, and the landing and the cancel of the member when the beam's entry is run-held, and passes otherwise.
// A failed origin probe is returned as is.
func TestBeamForkPrecondition(t *testing.T) {
	t.Parallel()

	held := boardengine.RunStatus("running", "Land")
	probeErr := errors.New("origin unreachable")
	tests := []struct {
		name        string
		entry       boardengine.Task
		entryFound  bool
		pairPresent bool
		onOrigin    bool
		probeErr    error
		// wantUnmet and wantWayForward are substrings of the precondition's two fields; both empty want a pass.
		wantUnmet      []string
		wantWayForward []string
		wantProbeErr   bool
	}{
		{
			name:           "AbsentBeamPairOffOriginNamesTheBeamCreate",
			entryFound:     true,
			wantUnmet:      []string{"m belongs to beam b", "pair is not on this machine"},
			wantWayForward: []string{`"lyx beam create b"`, `"lyx batten run m"`},
		},
		{
			name:           "AbsentBeamPairOnOriginNamesTheAdopt",
			entryFound:     true,
			onOrigin:       true,
			wantUnmet:      []string{"m belongs to beam b"},
			wantWayForward: []string{`"lyx adopt b"`, `"lyx batten run m"`},
		},
		{
			name:         "OriginProbeErrorIsReturned",
			entryFound:   true,
			probeErr:     probeErr,
			wantProbeErr: true,
		},
		{
			name:           "BeamHeldByItsLandingNamesTheLandingAndTheMembersCancel",
			entry:          boardengine.Task{Slug: "b", Status: &held},
			entryFound:     true,
			pairPresent:    true,
			wantUnmet:      []string{"m belongs to beam b", "its own landing holds", held},
			wantWayForward: []string{`"lyx batten status b"`, `"lyx batten cancel m"`},
		},
		{
			name:        "BeamWithItsPairAndNoRunPasses",
			entry:       boardengine.Task{Slug: "b"},
			entryFound:  true,
			pairPresent: true,
		},
		{
			name:        "BeamEntryNotFoundPasses",
			pairPresent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := beamForkPrecondition("m", "b", tt.entry, tt.entryFound, tt.pairPresent, func() (bool, error) { return tt.onOrigin, tt.probeErr })

			if tt.wantProbeErr {
				var precondition *battenshed.PreconditionError
				if !errors.Is(err, probeErr) || errors.As(err, &precondition) {
					t.Fatalf("beamForkPrecondition() error = %v; want the probe error, not a precondition", err)
				}
				return
			}
			if len(tt.wantUnmet) == 0 {
				if err != nil {
					t.Fatalf("beamForkPrecondition() = %v; want nil", err)
				}
				return
			}
			var precondition *battenshed.PreconditionError
			if !errors.As(err, &precondition) {
				t.Fatalf("beamForkPrecondition() = %v; want a *battenshed.PreconditionError", err)
			}
			for _, want := range tt.wantUnmet {
				if !strings.Contains(precondition.Unmet, want) {
					t.Errorf("Unmet = %q; want it to carry %q", precondition.Unmet, want)
				}
			}
			for _, want := range tt.wantWayForward {
				if !strings.Contains(precondition.WayForward, want) {
					t.Errorf("WayForward = %q; want it to carry %q", precondition.WayForward, want)
				}
			}
		})
	}
}
