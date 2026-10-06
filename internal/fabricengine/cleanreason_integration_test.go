//go:build integration

// cleanreason_integration_test.go is card 6's regression guard for Clean's reworded reason string:
// no earlier test asserted its exact wording, and loomengine prints it verbatim to an operator, so
// all three shapes — code-side only, state-side only, and both joined — are pinned here.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestClean_ReasonWording exercises the three shapes fabricengine.Clean can report a reason for.
// The steps share one hub and run in order: each step leaves the hub with neither side dirty, so the
// next one starts from a clean pair.
func TestClean_ReasonWording(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	warpUntracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
	weftUntracked := filepath.Join(h.PrimeWeft(), "untracked.txt")

	// dirty writes an untracked file at each path and removes it when the step ends.
	dirty := func(t *testing.T, paths ...string) {
		t.Helper()
		for _, path := range paths {
			if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
				t.Fatalf("write untracked file %s: %v", path, err)
			}
			t.Cleanup(func() { _ = os.Remove(path) })
		}
	}

	t.Run("CodeSideOnly", func(t *testing.T) {
		dirty(t, warpUntracked)

		ok, reason, err := fabricengine.Clean(h.Location)
		if err != nil {
			t.Fatalf("Clean: %v", err)
		}
		if ok {
			t.Fatalf("Clean = true; want false (code-side dirty)")
		}
		wantPrefix := "uncommitted code changes: "
		if len(reason) < len(wantPrefix) || reason[:len(wantPrefix)] != wantPrefix {
			t.Errorf("Clean reason = %q; want prefix %q", reason, wantPrefix)
		}
	})

	t.Run("StateSideOnly", func(t *testing.T) {
		dirty(t, weftUntracked)

		ok, reason, err := fabricengine.Clean(h.Location)
		if err != nil {
			t.Fatalf("Clean: %v", err)
		}
		if ok {
			t.Fatalf("Clean = true; want false (state-side dirty)")
		}
		wantPrefix := "uncommitted state changes under `_lyx`: "
		if len(reason) < len(wantPrefix) || reason[:len(wantPrefix)] != wantPrefix {
			t.Errorf("Clean reason = %q; want prefix %q", reason, wantPrefix)
		}
	})

	t.Run("Both", func(t *testing.T) {
		dirty(t, warpUntracked, weftUntracked)

		ok, reason, err := fabricengine.Clean(h.Location)
		if err != nil {
			t.Fatalf("Clean: %v", err)
		}
		if ok {
			t.Fatalf("Clean = true; want false (both sides dirty)")
		}
		wantCodePrefix := "uncommitted code changes: "
		wantStateFragment := "; uncommitted state changes under `_lyx`: "
		if len(reason) < len(wantCodePrefix) || reason[:len(wantCodePrefix)] != wantCodePrefix {
			t.Errorf("Clean reason = %q; want prefix %q", reason, wantCodePrefix)
		}
		if !strings.Contains(reason, wantStateFragment) {
			t.Errorf("Clean reason = %q; want it to contain %q (both sides joined with \"; \")", reason, wantStateFragment)
		}
	})
}
