// stencilseed.go implements the once-per-process stencil and deployed-specs seed/refresh pass run
// from newRoot's PersistentPreRunE: seedStencils resolves geometry and is a deliberate no-op under go
// test, stencilSeedTarget decides whether this process should seed at all and against which hub and
// worktree, and seedStencilsAt does the work of reconciling the board's stencils and specs subtrees
// against their shipped registries.
// The shared seedSubtree helper yields each subtree's write, and both land through one pull-first Bolt write.
// Neither function references internal/output or any envelope key: the mutation record this pass
// produces is logged, never surfaced in a command's JSON envelope, per the
// mutation-record-is-logged-not-enveloped-at-the-pre-run Shared Decision.

package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/contracts/specs"
	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/buildvcs"
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
	// it -- and skipping also keeps a long-lived process (e.g. lyx reed watchdog, the detached
	// per-hub daemon) from ever reaching fabricengine.Bolt.PullThenCommitWritten and performing a git
	// commit in the hub. This
	// early return sits ahead of stencilSeedTarget so an opted-out command resolves no geometry and
	// spawns no `git rev-parse`.
	if skipStencilSeed(cmd) {
		return
	}

	hub, worktree, ok := stencilSeedTarget(cmd.Context())
	if !ok {
		return
	}
	running := buildvcs.Running()
	seedStencilsAt(hub, worktree, stencilstore.ModeFor(buildinfo.IsDev(), buildinfo.IsProduction(), running.Clean()), running)
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

// seedStencilsAt reconciles the board's stencils and deployed-specs subtrees against their shipped
// registries and commits whatever each one wrote.
// It takes no context, so a test can drive it directly against a real hub without going through seedStencils' testing.Testing() guard.
// It takes the mode and the running identity as parameters, so a test can drive a production mode under a chosen identity without a stamped test binary.
//
// With neither subtree due it runs no git and touches no network.
// With either due, one pull-first Bolt write carries both subtrees' writes, stencils then specs:
// one pull and one seed-commit drop precede both writes, so the specs write never drops the stencils commit the same pass made.
// The pass is best-effort: a skipped pull or a failed write is logged and never fails the command.
func seedStencilsAt(hub, worktree string, mode stencilstore.Mode, running buildvcs.Identity) {
	sourceDir := filepath.Join(worktree, "contracts", "stencils")
	if _, err := os.Stat(sourceDir); err != nil {
		// The empty string means "no source tree here", which is what keeps the port-back drift
		// warning silent in a consumer repo instead of firing on every run forever.
		sourceDir = ""
	}

	var writes []fabricengine.BoltWrite
	if write, due := seedSubtree(fabricengine.StencilsDir(hub), fabricengine.StencilsSubtreeRel(), stencils.Registry(), mode, fabricengine.StencilSource(worktree, sourceDir, running), "stencils"); due {
		writes = append(writes, write)
	}

	// sourceDir is deliberately empty here rather than derived from worktree: sourceDir exists only
	// to drive the port-back drift warning, which serves an authoring workflow specs do not have --
	// the loomyard-side file is the single source of truth and a deployed copy is never authored.
	// A per-name source mapping is deliberately not built either: the two travelling docs live in
	// different directories and one's basename differs from its registered name, so no single
	// sourceDir shape fits.
	if write, due := seedSubtree(fabricengine.SpecsDir(hub), fabricengine.SpecsSubtreeRel(), specs.Registry(), mode, fabricengine.StencilSource(worktree, "", running), "specs"); due {
		writes = append(writes, write)
	}

	if len(writes) == 0 {
		return
	}

	res, err := fabricengine.NewBolt(fabricengine.BoardDir(hub)).PullThenCommitWritten(writes, fabricengine.NewMutations(hub))
	if err != nil {
		logger.Warn("stencilseed: seeding the board failed", "error", err)
		return
	}
	if res.Skipped != "" {
		logger.Info("stencilseed: seeding skipped, the board could not be brought up to date", "skip", string(res.Skipped))
		return
	}
	logger.Info("stencilseed: seeded", "commits", len(res.SHAs), "shas", res.SHAs)
}

// seedSubtree reports whether reconciling baseDir (the subtreeRel-rooted subtree of the board) against registry would write anything,
// and returns the board write that does it, committed under the seed subject for label ("stencils" or "specs") and the running binary.
// The write reconciles against the copy the pull-first Bolt write has just brought up to date, and returns the paths it wrote prefixed with subtreeRel.
// A due-check failure is logged at Warn and reports nothing due, since the root pre-run runs before every lyx invocation.
func seedSubtree(baseDir, subtreeRel string, registry stencilstore.Registry, mode stencilstore.Mode, source stencilstore.Source, label string) (fabricengine.BoltWrite, bool) {
	due, err := stencilstore.WritesDue(baseDir, registry, mode, source)
	if err != nil {
		logger.Warn("stencilseed: checking for due writes failed", "subtree", label, "error", err)
		return fabricengine.BoltWrite{}, false
	}
	if !due {
		return fabricengine.BoltWrite{}, false
	}

	return fabricengine.BoltWrite{
		Message: fabricengine.SeedCommitMessage(label, fabricengine.BinaryLabel()),
		Write: func() ([]string, error) {
			written, err := stencilstore.Reconcile(baseDir, registry, mode, source)
			if err != nil {
				return nil, fmt.Errorf("reconcile the %s subtree: %w", label, err)
			}
			paths := make([]string, len(written))
			for i, rel := range written {
				paths[i] = path.Join(subtreeRel, rel)
			}
			return paths, nil
		},
	}, true
}
