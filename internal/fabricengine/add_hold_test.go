// add_hold_test.go proves Add's Hold option: the caller's lock is taken once before the worktree is made and released after it, by the time Add returns, and a hold that fails refuses the add before anything is made.
//
// Package fabricengine_test; shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAdd_HoldsTheCallersLock asserts Add takes opts.Hold once, before it makes the worktree, and releases it before it returns, after the worktree is made,
// and that a Hold error is returned as is with no worktree made and nothing to release.
func TestAdd_HoldsTheCallersLock(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	holdFailure := errors.New("lock unavailable")
	tests := []struct {
		name     string
		holdErr  error
		wantMade bool
	}{
		{name: "HeldAroundTheAdd", wantMade: true},
		{name: "FailedHoldMakesNothing", holdErr: holdFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			const slug = "held-pair"
			worktree := fabricengine.WorktreePath(h.Location, slug)
			worktreeExists := func() bool {
				_, statErr := os.Stat(worktree)
				return statErr == nil
			}
			takes, releases := 0, 0
			var existsAtHold, existsAtRelease bool
			hold := func() (func(), error) {
				takes++
				existsAtHold = worktreeExists()
				if tt.holdErr != nil {
					return nil, tt.holdErr
				}
				return func() {
					releases++
					existsAtRelease = worktreeExists()
				}, nil
			}

			_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{SkipGit: true, Hold: hold})

			if !errors.Is(err, tt.holdErr) || (tt.holdErr == nil && err != nil) {
				t.Fatalf("Add(%q) error = %v; want %v", slug, err, tt.holdErr)
			}
			if takes != 1 {
				t.Errorf("Hold calls = %d; want 1", takes)
			}
			wantReleases := 0
			if tt.holdErr == nil {
				wantReleases = 1
			}
			if releases != wantReleases {
				t.Errorf("releases = %d; want %d", releases, wantReleases)
			}
			if made := worktreeExists(); made != tt.wantMade {
				t.Errorf("worktree made = %v; want %v", made, tt.wantMade)
			}
			if existsAtHold {
				t.Error("worktree existed when Hold was called; want the hold taken before the worktree is made")
			}
			if tt.wantMade && !existsAtRelease {
				t.Error("worktree did not exist when the hold was released; want the hold to span the worktree's making")
			}
		})
	}
}
