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
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestClean_ReasonWording exercises the three shapes fabricengine.Clean can report a reason for.
func TestClean_ReasonWording(t *testing.T) {
	t.Run("CodeSideOnly", func(t *testing.T) {
		t.Parallel()

		fixture := gitkit.CopyPairedLocal(t)
		untracked := filepath.Join(fixture.Hub, "untracked.txt")
		if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
			t.Fatalf("write untracked warp file: %v", err)
		}

		ok, reason, err := fabricengine.Clean(fixture.Layout)
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
		t.Parallel()

		fixture := gitkit.CopyPairedLocal(t)
		untracked := filepath.Join(fixture.WeftPrime, "untracked.txt")
		if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
			t.Fatalf("write untracked weft file: %v", err)
		}

		ok, reason, err := fabricengine.Clean(fixture.Layout)
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
		t.Parallel()

		fixture := gitkit.CopyPairedLocal(t)
		warpUntracked := filepath.Join(fixture.Hub, "untracked.txt")
		if err := os.WriteFile(warpUntracked, []byte("new"), 0o644); err != nil {
			t.Fatalf("write untracked warp file: %v", err)
		}
		weftUntracked := filepath.Join(fixture.WeftPrime, "untracked.txt")
		if err := os.WriteFile(weftUntracked, []byte("new"), 0o644); err != nil {
			t.Fatalf("write untracked weft file: %v", err)
		}

		ok, reason, err := fabricengine.Clean(fixture.Layout)
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
