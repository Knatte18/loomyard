//go:build tier1

// windowsite_integration_test.go pins which worktree's reed session holds a run's window, over a hubforge hub with a beam.

package battencli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

func TestWindowSite(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	resolverErr := errors.New("board unreadable")

	tests := []struct {
		name       string
		parent     string
		parentErr  error
		wantSite   string
		wantEnsure bool
		wantErr    error
	}{
		{"MemberOfAPresentBeamOpensInTheBeamWithItsSessionBroughtUp", "b", nil, "b", true, nil},
		{"TaskOfThePrimeOpensInThePrimeWithNoSessionBroughtUp", "", nil, primeName, false, nil},
		{"MemberWhoseBeamIsAbsentHereOpensInThePrime", "gone-beam", nil, primeName, false, nil},
		{"ResolverErrorIsReturned", "", resolverErr, "", false, resolverErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site, ensure, err := windowSite(h.Location, "m", func(string) (string, error) { return tt.parent, tt.parentErr })

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("windowSite() error = %v; want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if site.WorktreeName != tt.wantSite || ensure != tt.wantEnsure {
				t.Errorf("windowSite() = (%q, %v); want (%q, %v)", site.WorktreeName, ensure, tt.wantSite, tt.wantEnsure)
			}
		})
	}
}
