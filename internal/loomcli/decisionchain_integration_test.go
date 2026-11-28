// decisionchain_integration_test.go proves over a real hub which orchs may record a decision on a run:
// a beam's project record keeps the beam's own orch, and a task record leaves out the pair's own.

package loomcli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDecisionOrchChain(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "m", fabricengine.AddOptions{SkipPush: true, Parent: "b"})

	tests := []struct {
		name   string
		slug   string
		record runRecord
		want   []string
	}{
		{"A beam's project record keeps the beam's own orch", "b", runRecord{project: true}, []string{"tst:b:orch", "tst:orch"}},
		{"A task record leaves out the pair's own orch", "m", runRecord{}, []string{"tst:b:orch", "tst:orch"}},
		{"A beam's task record is its parents' alone", "b", runRecord{}, []string{"tst:orch"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			target, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(tt.slug))
			if err != nil {
				t.Fatalf("ResolveWorktree(%q) = %v; want nil", tt.slug, err)
			}
			chain, err := decisionOrchChain(target, tt.record)
			if err != nil {
				t.Fatalf("decisionOrchChain(%s) = _, %v; want nil error", tt.slug, err)
			}
			var got []string
			for _, orch := range chain {
				got = append(got, orch.Name)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("decisionOrchChain(%s) = %q; want %q", tt.slug, got, tt.want)
			}
		})
	}
}
