//go:build integration

// add_branch_exists_test.go proves Add's branch-already-exists rejection names
// a way forward. Remove deliberately leaves the host branch behind (it may
// carry unmerged work), so the everyday remove-then-re-add cycle hits this
// rejection — a bare "already exists" left the operator stuck without
// out-of-band git knowledge.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestAdd_ExistingBranchErrorNamesRemedy creates the host branch a slug would
// claim, calls Add with that slug, and asserts the rejection names both
// remedies (checkout onto it, or delete the leftover).
func TestAdd_ExistingBranchErrorNamesRemedy(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	const slug = "leftover-pair"
	lyxtest.MustRun(t, l.WorktreeRoot, "git", "branch", slug)

	_, err := topology.Add(l, slug, fabricengine.AddOptions{SkipGit: true})
	if err == nil {
		t.Fatalf("Add(%q) error = nil; want branch-already-exists rejection", slug)
	}
	for _, want := range []string{"already exists", "lyx fabric checkout " + slug, "git branch -D " + slug} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Add(%q) error = %q; want substring %q", slug, err.Error(), want)
		}
	}
}
