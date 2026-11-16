// fingerprint_test.go covers the begun-card hash move a plan re-baseline makes.
// Tier 0: no git, no files.

package runplan

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMoveBegunCardHashes proves a begun card's recorded hash moves only when it equals the pre-rewrite file hash.
// A mismatching one keeps its old hash, and a file absent from either map is left alone.
// An empty PlanFileHashes is a no-op.
func TestMoveBegunCardHashes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	newState := func() *State {
		return &State{Batches: map[int]*BatchState{
			1: {CardHashes: map[string]string{"01-a": "a0"}},
			2: {CardHashes: map[string]string{"02-b": "stale"}},
			3: {CardHashes: map[string]string{"03-c": "c0"}},
			4: {CardHashes: map[string]string{"04-d": "d0"}},
			5: nil,
			6: {},
		}}
	}
	before := map[string]string{"01-a.md": "a0", "02-b.md": "b0", "03-c.md": "c0"}
	after := map[string]string{"01-a.md": "a1", "02-b.md": "b1", "04-d.md": "d1"}

	t.Run("moves only an exact match", func(t *testing.T) {
		t.Parallel()
		st := newState()
		moveBegunCardHashes(st, before, after)
		if got := st.Batches[1].CardHashes["01-a"]; got != "a1" {
			t.Errorf("matching hash = %q; want it moved to a1", got)
		}
		if got := st.Batches[2].CardHashes["02-b"]; got != "stale" {
			t.Errorf("mismatching hash = %q; want it kept as stale", got)
		}
		if got := st.Batches[3].CardHashes["03-c"]; got != "c0" {
			t.Errorf("hash for a file absent from after = %q; want it left at c0", got)
		}
		if got := st.Batches[4].CardHashes["04-d"]; got != "d0" {
			t.Errorf("hash for a file absent from before = %q; want it left at d0", got)
		}
	})

	t.Run("empty before is a no-op", func(t *testing.T) {
		t.Parallel()
		st := newState()
		moveBegunCardHashes(st, nil, after)
		if got := st.Batches[1].CardHashes["01-a"]; got != "a0" {
			t.Errorf("hash = %q; want it unchanged with no PlanFileHashes", got)
		}
	})
}
