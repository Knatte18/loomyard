//go:build integration

// preflight_integration_test.go covers loomshed.NewPreflightProducer's real wrapper against a
// hubforge fixture hub -- the only row in loom's producer list that needs real git. It is a package
// loomshed_test file, not an in-package test, because internal/loomshed imports internal/loomengine,
// which sits inside the dependency set internal/hubforge reaches: an in-package test importing
// hubforge would close a compile cycle -- the same reason internal/loomengine's own integration test
// is an external-package file.
//
// This file stays to the wrapper's outcome mapping and nothing more: Preflight's own four checks
// are already covered exhaustively by internal/loomengine's existing integration suite, and
// duplicating them here would couple this package's tests to another package's check set.

package loomshed_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// setupPreflightWrapperFixture builds a fully-configured real hub with fabric and junction setup,
// with a fresh status.json seeded through loomshed.Seed -- never by hand-writing JSON -- so a Seed
// regression would not pass unnoticed here, mirroring the seeding discipline
// internal/loomengine's own setupPreflightFixture documents for its own hand-written seed.
func setupPreflightWrapperFixture(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	slug := filepath.Base(h.Location.WorktreePath())

	// fabricengine.ConfigTemplate() is already reconciled by fabriccli.CloneAndWire when NewHub
	// built h; the repo-wide fabric.yaml override below is the genuine change, mirroring
	// internal/loomengine's own setupPreflightFixture.
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")

	if err := fabricengine.WireJunctions(h.Location, slug, []string{"_lyx", lyxdirs.DotLyxDirName, "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	statusPath := loomengine.LoomStatusFile(h.Location)
	statusLockPath := loomengine.LoomStatusLock(h.Location)
	if err := loomshed.Seed(statusPath, statusLockPath, "loom-preflight-wrapper-fixture", "main"); err != nil {
		t.Fatalf("loomshed.Seed: %v", err)
	}

	// The seeded status.json (and its .lock sidecar) materialize through the _lyx junction into
	// the fabric worktree's own git repo, where they start out untracked. Commit them so a
	// freshly-built fixture is genuinely clean on both sides.
	gitkit.MustRun(t, h.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "commit", "-m", "seed status")

	return h
}

// TestPreflightProducer_AllPreconditionsPass asserts that a fixture whose preconditions all pass
// yields shedengine.Done.
func TestPreflightProducer_AllPreconditionsPass(t *testing.T) {
	t.Parallel()

	h := setupPreflightWrapperFixture(t)

	p := loomshed.NewPreflightProducer(h.PrimeWorktree())
	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
}

// TestPreflightProducer_BrokenPreconditionMapsToStuck asserts that a fixture with a deliberately
// broken precondition -- an untracked file left in the warp worktree, failing the worktree
// cleanliness check -- yields shedengine.Stuck.
func TestPreflightProducer_BrokenPreconditionMapsToStuck(t *testing.T) {
	t.Parallel()

	h := setupPreflightWrapperFixture(t)

	untracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
	if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	p := loomshed.NewPreflightProducer(h.PrimeWorktree())
	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
}
