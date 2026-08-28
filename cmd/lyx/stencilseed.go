// stencilseed.go implements the once-per-process stencil seed/refresh pass run from newRoot's
// PersistentPreRunE: seedStencils resolves geometry and is a deliberate no-op under go test,
// stencilSeedTarget decides whether this process should seed at all and against which hub and
// worktree, and seedStencilsAt does the work of reconciling the board's stencils against the
// shipped registry and committing whatever it wrote.
// Neither function references internal/output or any envelope key: the mutation record this pass
// produces is logged, never surfaced in a command's JSON envelope, per the
// mutation-record-is-logged-not-enveloped-at-the-pre-run Shared Decision.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// seedStencils is the thin pre-run wrapper newRoot's PersistentPreRunE calls: it resolves this
// process' seed target via stencilSeedTarget and delegates to seedStencilsAt when one is found.
func seedStencils(cmd *cobra.Command) {
	// Return immediately under go test, before resolving anything: lyxcwd.Resolve spawns `git
	// rev-parse --show-toplevel`, and cobra runs this root PersistentPreRunE for every Runnable
	// command -- every parent group included, since each carries RunE: clihelp.GroupRunE. Without
	// this guard, dozens of existing untagged cmd/lyx tests that drive a Runnable command would
	// newly spawn git as a side effect, breaking the Test Tier Purity Invariant for the whole
	// package.
	if testing.Testing() {
		return
	}

	// A command that carries the skip annotation reads no stencils, so the pass is pure waste for
	// it -- and skipping also keeps a long-lived pane process (e.g. reed header's keepalive) from
	// ever reaching fabricengine.CommitSeededStencils and performing a git commit in the hub. This
	// early return sits ahead of stencilSeedTarget so an opted-out command resolves no geometry and
	// spawns no `git rev-parse`.
	if skipStencilSeed(cmd) {
		return
	}

	hub, worktree, ok := stencilSeedTarget(cmd.Context())
	if !ok {
		return
	}
	seedStencilsAt(hub, worktree)
}

// skipStencilSeed reports whether cmd carries clihelp.SkipStencilSeedAnnotation set to
// clihelp.AnnotationEnabled, and is therefore declining the root pre-run's stencil-seed pass.
// It is extracted as its own directly-assertable function rather than inlined into seedStencils, for
// the reason stencilSeedTarget's own comment already records: seedStencils returns immediately under
// testing.Testing(), so a test can never observe the gate through it.
func skipStencilSeed(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return cmd.Annotations[clihelp.SkipStencilSeedAnnotation] == clihelp.AnnotationEnabled
}

// stencilSeedTarget decides whether this process should seed stencils and, when it should, against
// which hub and worktree.
// It is a separate value-returning function -- rather than being inlined into seedStencils -- because
// seedStencils returns immediately under testing.Testing(), so a test can never observe the gate
// through it; extracting the decision here is what makes the gate directly assertable, the same
// rationale seedStencilsAt already carries.
//
// The gate is preflight.HubPresent, never preflight.Wired: the write this pass performs targets the
// hub-level stencils directory, so the honest precondition is that the hub-level directory exists.
// preflight.Wired probes this worktree's own pairing instead, and would stop seeding in three real-hub
// situations that seed correctly today -- an ordinary worktree at <hub>/_board, an unpaired sibling,
// and a worktree whose paired sibling has been removed. Narrowing this gate to Wired is a regression,
// not a tightening.
func stencilSeedTarget(ctx context.Context) (hub, worktree string, ok bool) {
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		// No geometry to resolve for this invocation; the root pre-run resolves no hub for
		// commands that legitimately have none (e.g. lyx fabric clone), so the pass is skipped
		// rather than failing.
		return "", "", false
	}

	// preflight.HubPresent performs the lyxcwd.Resolve this pass needs; calling lyxcwd.Resolve
	// again here would double the `git rev-parse` spawn this pre-run performs before every single
	// command.
	l, present := preflight.HubPresent(cwd)
	if !present {
		return "", "", false
	}

	return l.HubPath, l.WorktreePath(), true
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

	mode := stencilstore.ModeFor(buildinfo.IsDev())

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
