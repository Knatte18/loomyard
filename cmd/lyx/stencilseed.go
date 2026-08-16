// stencilseed.go implements the once-per-process stencil seed/refresh pass run from newRoot's
// PersistentPreRunE: seedStencils resolves geometry and is a deliberate no-op under go test,
// seedStencilsAt does the work of reconciling the board's stencils against the shipped registry and
// committing whatever it wrote.
// Neither function references internal/output or any envelope key: the mutation record this pass
// produces is logged, never surfaced in a command's JSON envelope, per the
// mutation-record-is-logged-not-enveloped-at-the-pre-run Shared Decision.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// buildChannel is set by tools/deploy -dev via -ldflags "-X main.buildChannel=dev".
// An unstamped binary -- a plain `go build`, a `go install`, or a `go test` binary -- leaves it
// empty, and empty means production.
// Production is the conservative default because it keeps shipped defaults converging;
// dev is the exception and must opt in explicitly.
var buildChannel string

// seedStencils is the thin pre-run wrapper newRoot's PersistentPreRunE calls: it resolves this
// process' hub and worktree geometry and delegates to seedStencilsAt.
func seedStencils(ctx context.Context) {
	// Return immediately under go test, before resolving anything: lyxcwd.Resolve spawns `git
	// rev-parse --show-toplevel`, and cobra runs this root PersistentPreRunE for every Runnable
	// command -- every parent group included, since each carries RunE: clihelp.GroupRunE. Without
	// this guard, dozens of existing untagged cmd/lyx tests that drive a Runnable command would
	// newly spawn git as a side effect, breaking the Test Tier Purity Invariant for the whole
	// package.
	if testing.Testing() {
		return
	}

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		// No geometry to resolve for this invocation; the root pre-run resolves no hub for
		// commands that legitimately have none (e.g. lyx fabric clone), so the pass is skipped
		// rather than failing.
		return
	}
	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return
	}

	seedStencilsAt(l.HubPath, l.WorktreePath())
}

// seedStencilsAt reconciles the board's stencils directory against the shipped registry and commits
// whatever it wrote.
// It takes no context, so a test can drive it directly against a real hub without going through
// seedStencils' testing.Testing() guard.
func seedStencilsAt(hub, worktree string) {
	baseDir := fabricengine.StencilsDir(hub)

	sourceDir := filepath.Join(worktree, "contracts", "stencils")
	if _, err := os.Stat(sourceDir); err != nil {
		// The empty string means "no source tree here", which is what keeps the port-back drift
		// warning silent in a consumer repo instead of firing on every run forever.
		sourceDir = ""
	}

	mode := stencilstore.ModeProduction
	if buildChannel == "dev" {
		mode = stencilstore.ModeDev
	}

	written, err := stencilstore.Reconcile(baseDir, stencils.Registry(), mode, sourceDir)
	if err != nil {
		logger.Warn("stencilseed: reconcile stencils failed", "error", err)
		return
	}
	if len(written) == 0 {
		return
	}

	res, err := fabricengine.CommitSeededStencils(hub, written, "lyx: seed stencils", fabricengine.NewMutations(filepath.Dir(hub)))
	if err != nil {
		logger.Warn("stencilseed: commit seeded stencils failed", "error", err)
		return
	}
	logger.Info("stencilseed: seeded stencils", "written", len(written), "committed", res.Committed, "sha", res.SHA)
}
