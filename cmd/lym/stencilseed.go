// stencilseed.go runs the stencil and deployed-specs seed pass once per process, from `newRoot`'s `PersistentPreRunE`.
// The pass reconciles the board's stencils and specs subtrees against their shipped registries.
// It writes both in one pull-first Bolt write.
// The pass logs its mutation record and never puts it in a command's JSON envelope.

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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// seedStencils runs the seed pass for the hub this process is in.
// It does nothing under go test, for a command that opts out, or when the invocation resolves no hub.
func seedStencils(cmd *cobra.Command) {
	// Return immediately under go test, before resolving anything.
	// lymcwd.Resolve spawns `git rev-parse --show-toplevel`.
	// Cobra runs this root PersistentPreRunE for every Runnable command.
	// Parent groups count as Runnable, since each carries RunE: clihelp.GroupRunE, so the hook runs for them too.
	// Without this guard, the untagged cmd/lym tests that drive a Runnable command would spawn git as a side effect.
	// That would break the Test Tier Purity Invariant for the whole package.
	if testing.Testing() {
		return
	}

	// A command that carries the skip annotation reads no stencils, so the pass is pure waste for it.
	// Skipping also matters for a long-lived process, such as the detached per-hub daemon.
	// Without it, the process could reach fabricengine.Bolt.PullThenCommitWritten and commit in the hub.
	// This early return sits ahead of stencilSeedTarget,
	// so an opted-out command resolves no geometry and spawns no `git rev-parse`.
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

// skipStencilSeed reports whether cmd opts out of the stencil seed pass.
// It does so by carrying clihelp.SkipStencilSeedAnnotation set to clihelp.AnnotationEnabled.
func skipStencilSeed(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return cmd.Annotations[clihelp.SkipStencilSeedAnnotation] == clihelp.AnnotationEnabled
}

// stencilSeedTarget decides whether this process should seed stencils.
// When it should, it reports the hub and worktree to seed against.
// It is a separate function so that a test can observe the gate.
// The pre-run returns early under testing.Testing(), so the gate is unreachable through it.
//
// The gate checks that the hub-level directory exists, not this worktree's own pairing.
// The write targets the hub-level stencils directory.
// Checking the pairing would stop seeding in situations that seed correctly:
// an ordinary worktree at the Bolt checkout, an unpaired sibling, and a worktree whose paired sibling has been removed.
// Narrowing the gate to the pairing is a regression, not a tightening.
func stencilSeedTarget(ctx context.Context) (hub, worktree string, ok bool) {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		// No geometry to resolve for this invocation.
		// The root pre-run resolves no hub for commands that legitimately have none, such as `lym fabric clone`.
		// The pass is therefore skipped rather than failing.
		return "", "", false
	}

	// preflight.HubPresent performs the lymcwd.Resolve this pass needs;
	// calling lymcwd.Resolve again would double the `git rev-parse` spawn this pre-run makes before every command.
	l, present := preflight.HubPresent(cwd)
	if !present {
		return "", "", false
	}

	return l.HubPath, l.WorktreePath(), true
}

// seedStencilsAt reconciles the board's stencils and deployed-specs subtrees against their shipped registries.
// It commits whatever each one wrote.
// A test can drive it directly against a real hub, because the go test guard sits in the pre-run and not here.
// It takes the mode and the running identity as parameters.
// That lets a test drive a production mode under a chosen identity without a stamped test binary.
//
// With neither subtree due, it runs no git and touches no network.
// With either due, one pull-first Bolt write carries both subtrees' writes, stencils then specs:
// one pull and one seed-commit drop precede both writes,
// so the specs write never drops the stencils commit the same pass made.
// The pass is best-effort: a skipped pull or a failed write is logged and never fails the command.
func seedStencilsAt(hub, worktree string, mode stencilstore.Mode, running buildvcs.Identity) {
	sourceDir := filepath.Join(worktree, "contracts", "stencils")
	if _, err := os.Stat(sourceDir); err != nil {
		// The empty string means "no source tree here", which keeps the port-back drift warning silent in a consumer repo instead of firing on every run.
		sourceDir = ""
	}

	var writes []fabricengine.BoltWrite
	if write, due := seedSubtree(fabricengine.StencilsDir(hub), fabricengine.StencilsSubtreeRel(), stencils.Registry(), mode, fabricengine.StencilSource(worktree, sourceDir, running), "stencils"); due {
		writes = append(writes, write)
	}

	// sourceDir is empty here rather than derived from worktree.
	// sourceDir exists only to drive the port-back drift warning, which serves an authoring workflow specs do not have;
	// the loomyard-side file is the single source of truth,
	// and a deployed copy is never authored.
	// A per-name source mapping is not built either: the travelling docs live in different directories,
	// and one's basename differs from its registered name, so no single sourceDir shape fits.
	if write, due := seedSubtree(fabricengine.SpecsDir(hub), fabricengine.SpecsSubtreeRel(), specs.Registry(), mode, fabricengine.StencilSource(worktree, "", running), "specs"); due {
		writes = append(writes, write)
	}

	if len(writes) == 0 {
		return
	}

	res, err := fabricengine.NewBolt(fabricengine.BoltDir(hub)).PullThenCommitWritten(writes, fabricengine.NewMutations(hub))
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

// seedSubtree reports whether reconciling baseDir against registry would write anything.
// baseDir is the board subtree rooted at subtreeRel.
// When it would, it returns the board write.
// The write commits under the seed subject for label, "stencils" or "specs", and the running binary.
// The write reconciles against the copy the pull-first Bolt write has just brought up to date,
// and it reports the paths it wrote, prefixed with subtreeRel.
// A due-check failure is logged at Warn and reports nothing due.
// The root pre-run runs before every lym invocation, so a failure here must not fail any of them.
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
