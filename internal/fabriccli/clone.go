// clone.go implements the fabriccli handler half for the fabric clone subcommand.
// runCloneWithReset delegates into fabricengine.CloneHub after optionally tearing
// down an existing hub when --reset is given, then drives the config
// materialization + weft:main commit + junction wiring + reconcile sequence
// that makes "clone does everything" true at the command level — CloneHub
// itself stays git/geometry-focused so fabricengine never imports configsync
// (see fabricengine/clone.go's CloneResult doc comment for the import-cycle
// rationale).

package fabriccli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
)

// runCloneWithReset executes the clone subcommand.
//
// When reset is true it tears down any existing hub at the derived path before
// cloning, making the operation idempotent. The teardown uses fabricengine.RemoveAll
// so tests can inject errors by swapping that exported var.
//
// After fabricengine.CloneHub succeeds, this handler drives the sequence that
// used to be lyx init's job: materialize the repo-wide fabric.yaml, commit the
// anchor marker + config onto weft:main, wire host junctions, maintain the
// .gitignore .lyx/ block, and reconcile per-worktree module configs. On any
// step erroring, the hub's git clone is left intact (per the batch's
// partial-failure-recovery decision) — the operator completes wiring with
// `lyx fabric reconcile` rather than this handler destructively tearing down
// a good clone.
func runCloneWithReset(out io.Writer, args []string, reset bool, subpath string) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	if len(args) != 2 {
		return output.Err(out, "usage: lyx fabric clone [--reset] [--subpath <rel>] <host-url> <weft-url>")
	}
	hostURL := args[0]
	weftURL := args[1]

	if reset {
		// Derive the hub path so we can remove it before cloning (idempotent re-clone).
		// DeriveHostName returns "" for blank/unparseable URLs; guard defensively.
		name := fabricengine.DeriveHostName(hostURL)
		if name == "" {
			return output.Err(out, fmt.Sprintf("could not derive repo name from host URL %s", hostURL))
		}
		hubPath := hubgeometry.HubPath(cwd, name)
		if err := fabricengine.RemoveAll(hubPath); err != nil {
			return output.Err(out, fmt.Sprintf("reset: remove hub at %s: %v", hubPath, err))
		}
	}

	res, err := fabricengine.CloneHub(cwd, hostURL, weftURL, subpath)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Materialize the repo-wide fabric.yaml at <BoardDir>/_lyx/config/fabric.yaml.
	// Idempotent: a no-op on the adopt path where it is already present.
	if _, err := configsync.ReconcileFabricAt(res.BoardDir, true); err != nil {
		return output.Err(out, err.Error())
	}

	// Commit + push the .fabric-anchor marker and fabric.yaml onto weft:main
	// through the Bolt handle. Its wildcard-stage commit covers both files;
	// this is a clean no-op on the adopt path.
	b := fabricengine.NewBolt(res.BoardDir)
	if _, _, err := b.Commit("fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{}); err != nil {
		return output.Err(out, err.Error())
	}
	if err := b.Push(fabricengine.SyncOptions{}); err != nil {
		return output.Err(out, err.Error())
	}

	// Wire host junctions for the prime worktree: the weft worktree already
	// exists (CloneHub materialized it), so every junction target resolves.
	l, err := hubgeometry.Resolve(res.PrimeCwd)
	if err != nil {
		return output.Err(out, err.Error())
	}
	names, err := fabricengine.WiredNames(res.BoardDir)
	if err != nil {
		return output.Err(out, err.Error())
	}
	if err := fabricengine.WireJunctions(l, filepath.Base(l.WorktreeRoot), names); err != nil {
		return output.Err(out, err.Error())
	}

	// Maintain the .gitignore .lyx/ managed block on the host worktree.
	if _, err := gitignore.Ensure(l.Cwd, ".lyx/"); err != nil {
		return output.Err(out, err.Error())
	}

	// Materialize per-worktree module configs on the weft side (fabric is
	// already skipped by ReconcileAll — see configsync.ReconcileAll's doc
	// comment).
	if _, err := configsync.ReconcileAll(res.WeftBase, true); err != nil {
		return output.Err(out, err.Error())
	}

	return output.Ok(out, map[string]any{
		"hub":    res.HubPath,
		"anchor": res.Anchor,
	})
}
