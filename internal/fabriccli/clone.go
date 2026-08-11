// clone.go implements the fabriccli handler half for the fabric clone subcommand.
// runCloneWithReset delegates into fabricengine.CloneHub after optionally tearing down an existing
// hub when --reset is given, then calls CloneAndWire to drive the config materialization +
// weft:main commit + junction wiring + reconcile sequence that makes "clone does everything" true
// at the command level — CloneHub itself stays git/geometry-focused so fabricengine never imports
// configsync (see fabricengine/clone.go's CloneResult doc comment for the import-cycle rationale).

package fabriccli

import (
	"io"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// CloneAndWire clones a hub via fabricengine.CloneHub and drives the wiring sequence that makes
// "clone does everything" true: repo-wide fabric.yaml materialization, the weft:main anchor+config
// commit and push, warp junction wiring, and per-worktree config reconciliation.
//
// It is the single wiring sequence shared by the cobra clone handler and
// internal/fabricengine/fabrictest's hub factory; a second copy of this sequence anywhere is the
// drift this extraction exists to prevent.
//
// On error, the clone is left intact; the operator completes wiring with reconcile.
func CloneAndWire(cwd string, opts fabricengine.CloneOptions) (fabricengine.CloneResult, error) {
	res, err := fabricengine.CloneHub(cwd, opts)
	if err != nil {
		return fabricengine.CloneResult{}, err
	}

	if _, err := configsync.ReconcileFabricAt(res.BoardDir, true); err != nil {
		return fabricengine.CloneResult{}, err
	}

	b := fabricengine.NewBolt(res.BoardDir)
	if _, _, err := b.Commit("fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{}); err != nil {
		return fabricengine.CloneResult{}, err
	}
	if err := b.Push(fabricengine.SyncOptions{}); err != nil {
		return fabricengine.CloneResult{}, err
	}

	l, err := lyxcwd.Resolve(res.PrimeCwd)
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	names, err := fabricengine.WiredNames(res.BoardDir)
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	if err := fabricengine.WireJunctions(l, filepath.Base(l.WorktreePath()), names); err != nil {
		return fabricengine.CloneResult{}, err
	}

	// .lyx is excluded through the warp's .git/info/exclude by WireJunctions above,
	// not through a tracked .gitignore entry in the user's own repo: a committed
	// entry would advertise that LYX is in use, and a warp→weft junction must never
	// leave a tracked artifact behind in the user's repo.
	if _, err := configsync.ReconcileAll(res.WeftBase, true); err != nil {
		return fabricengine.CloneResult{}, err
	}

	return res, nil
}

// runCloneWithReset executes the clone subcommand. The hub teardown for an idempotent re-clone is no
// longer performed here: it is driven through CloneOptions.Reset inside CloneHub itself, which can
// derive the hub path in either the one- or two-argument form. It parses arguments, resolves cwd,
// and delegates the entire clone-and-wire sequence to CloneAndWire. The returned envelope carries
// "hub" and "anchor" from the resolved geometry, plus "warp" (the effective warp URL, supplied or
// derived) and "warp_binding_recorded" (whether this clone wrote the .lyx-warp record) — both
// always present so a consumer never has to distinguish absent from false.
func runCloneWithReset(out io.Writer, args []string, reset bool, subpath string, forceBootstrap bool) int {
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	if len(args) != 1 && len(args) != 2 {
		return output.Err(out, "usage: lyx fabric clone [--reset] [--subpath <rel>] [--force-bootstrap] <weft-url> [<warp-url>]")
	}
	weftURL := args[0]
	warpURL := ""
	if len(args) == 2 {
		warpURL = args[1]
	}

	res, err := CloneAndWire(cwd, fabricengine.CloneOptions{
		WeftURL:        weftURL,
		WarpURL:        warpURL,
		Subpath:        subpath,
		Reset:          reset,
		ForceBootstrap: forceBootstrap,
	})
	if err != nil {
		return output.Err(out, err.Error())
	}

	return output.Ok(out, map[string]any{
		"hub":                   res.HubPath,
		"anchor":                res.Anchor,
		"warp":                  res.WarpURL,
		"warp_binding_recorded": res.WarpBindingRecorded,
	})
}
