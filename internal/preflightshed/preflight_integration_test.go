//go:build integration

// preflight_integration_test.go covers preflightshed.NewPreflight's wrapper against a hubforge
// fixture hub. It is in-package because nothing in hubforge's dependency set reaches this new leaf
// (go list -deps ./internal/hubforge contains internal/preflight but not internal/preflightshed),
// unlike internal/loomshed's own former copy of this file, which stayed external because
// internal/loomshed imports internal/loomengine, itself inside hubforge's dependency set.
//
// This file stays to the wrapper's outcome mapping and nothing more: preflight.Check's own checks
// are already covered exhaustively by internal/preflight's own integration suite, and duplicating
// them here would couple this package's tests to another package's check set.

package preflightshed

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// setupPreflightWrapperFixture builds a fully-configured real hub with fabric and junction setup.
// Post-split, row 1's own test fixture never reads loom's status.json, so there is nothing to seed
// here -- unlike internal/loomshed's former copy of this fixture, which seeded a status.json through
// loomshed.Seed.
func setupPreflightWrapperFixture(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	slug := filepath.Base(h.Location.WorktreePath())

	// fabricengine.ConfigTemplate() is already reconciled by fabriccli.CloneAndWire when NewHub
	// built h; the repo-wide fabric.yaml override below is the genuine change, mirroring
	// internal/preflight's own setupFixture.
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")

	if err := fabricengine.WireJunctions(h.Location, slug, []string{"_lyx", lyxdirs.DotLyxDirName, "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	// The wired junctions materialize through the _lyx junction into the paired-sibling worktree's
	// own git repo, where they start out untracked. Commit them so a freshly-built fixture is
	// genuinely clean on both sides -- fabricengine.WireJunctions leaves untracked entries behind
	// that the cleanliness check would otherwise report, mirroring internal/preflight's own
	// setupFixture, which performs this same add-and-commit with no seed step at all.
	// The commit is --allow-empty because after the clone-commit change the weft prime already
	// arrives clean, .lyx is excluded through the weft repo's .git/info/exclude, and the _extra
	// junction target materializes as an empty directory git does not track -- so this pair becomes
	// a no-op that must be allowed to succeed rather than deleted, because deleting it would silently
	// drop the guarantee if a future fixture change reintroduces untracked weft content.
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "--allow-empty", "-m", "seed junctions")

	return h
}

// TestPreflight_AllPreconditionsPass asserts that a fixture whose preconditions all pass yields
// shedengine.Done.
func TestPreflight_AllPreconditionsPass(t *testing.T) {
	t.Parallel()

	h := setupPreflightWrapperFixture(t)

	p := NewPreflight("Preflight", h.PrimeWorktree())
	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
}

// TestPreflight_BrokenPreconditionMapsToStuck asserts that a fixture with a deliberately broken
// precondition -- an untracked file left in the prime worktree, failing the worktree cleanliness
// check -- yields shedengine.Stuck.
func TestPreflight_BrokenPreconditionMapsToStuck(t *testing.T) {
	t.Parallel()

	h := setupPreflightWrapperFixture(t)

	untracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
	if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	p := NewPreflight("Preflight", h.PrimeWorktree())
	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
}

// TestPreflight_CancelledDuringRunReturnsError is Tier 2, not Tier 1, because preflight.Check
// reaches lyxcwd.Resolve's git spawn unconditionally, so every path that calls Check at all is
// Tier 2 regardless of what it returns -- the producer holds no injectable seam between entryErr
// and the Check call that would let this case run without a real fixture.
func TestPreflight_CancelledDuringRunReturnsError(t *testing.T) {
	t.Parallel()

	h := setupPreflightWrapperFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewPreflight("Preflight", h.PrimeWorktree())
	outcome, _, err := p.Call(ctx)
	if err == nil {
		t.Fatalf("Call(cancelled) error = nil; want non-nil error")
	}
	if outcome == shedengine.Done || outcome == shedengine.Stuck {
		t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
	}
}
