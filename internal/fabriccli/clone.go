// clone.go implements the fabriccli handler half for the fabric clone subcommand.
// runCloneWithReset delegates into fabricengine.CloneHub after optionally tearing down an existing
// hub when --reset is given, then drives the config materialization + weft:main commit + junction
// wiring + reconcile sequence that makes "clone does everything" true at the command level —
// CloneHub itself stays git/geometry-focused so fabricengine never imports configsync (see
// fabricengine/clone.go's CloneResult doc comment for the import-cycle rationale).

package fabriccli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// runCloneWithReset executes the clone subcommand. When reset is true, it tears
// down any existing hub before cloning (idempotent re-clone). After CloneHub
// succeeds, it drives the wiring sequence: repo-wide fabric.yaml, weft:main
// commits, host junctions, and per-worktree config. On error, the clone is
// left intact; the operator completes wiring with reconcile.
func runCloneWithReset(out io.Writer, args []string, reset bool, subpath string) int {
	cwd, err := lyxcwd.Getwd()
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
		hubPath := fabricengine.HubPath(cwd, name)
		if err := fabricengine.RemoveAll(hubPath); err != nil {
			return output.Err(out, fmt.Sprintf("reset: remove hub at %s: %v", hubPath, err))
		}
	}

	res, err := fabricengine.CloneHub(cwd, hostURL, weftURL, subpath)
	if err != nil {
		return output.Err(out, err.Error())
	}

	if _, err := configsync.ReconcileFabricAt(res.BoardDir, true); err != nil {
		return output.Err(out, err.Error())
	}

	b := fabricengine.NewBolt(res.BoardDir)
	if _, _, err := b.Commit("fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{}); err != nil {
		return output.Err(out, err.Error())
	}
	if err := b.Push(fabricengine.SyncOptions{}); err != nil {
		return output.Err(out, err.Error())
	}

	l, err := lyxcwd.Resolve(res.PrimeCwd)
	if err != nil {
		return output.Err(out, err.Error())
	}
	names, err := fabricengine.WiredNames(res.BoardDir)
	if err != nil {
		return output.Err(out, err.Error())
	}
	if err := fabricengine.WireJunctions(l, filepath.Base(l.WorktreePath()), names); err != nil {
		return output.Err(out, err.Error())
	}

	// .lyx is excluded through the warp's .git/info/exclude by WireJunctions above,
	// not through a tracked .gitignore entry in the user's own repo: a committed
	// entry would advertise that LYX is in use, and a host→weft junction must never
	// leave a tracked artifact behind in the user's repo.
	if _, err := configsync.ReconcileAll(res.WeftBase, true); err != nil {
		return output.Err(out, err.Error())
	}

	return output.Ok(out, map[string]any{
		"hub":    res.HubPath,
		"anchor": res.Anchor,
	})
}
