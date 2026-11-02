// gatefixer_integration_test.go drives pairSlugOf against a real hub, since the pair slug is whatever the fabric origin record of the worktree answers.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPairSlugOf(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "fixerpair"
	hubforge.AddPair(t, hub, slug)

	for _, tt := range []struct {
		name     string
		worktree string
		want     string
	}{
		{"PairWorktreeAnswersItsSlug", hub.PairCodeWorktree(slug), slug},
		{"PrimeAnswersNone", hub.PrimeWorktree(), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			location, err := lymcwd.ResolveWorktree(tt.worktree)
			if err != nil {
				t.Fatalf("ResolveWorktree(%q) error = %v; want nil", tt.worktree, err)
			}
			got, err := pairSlugOf(location)
			if err != nil {
				t.Fatalf("pairSlugOf() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("pairSlugOf() = %q; want %q", got, tt.want)
			}
		})
	}
}
