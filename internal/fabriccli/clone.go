// clone.go implements the fabriccli handler half for the fabric clone subcommand.
// runCloneWithReset delegates into fabricengine.CloneHub after optionally tearing down an existing
// hub when --reset is given, then calls CloneAndWire to drive the config materialization +
// weft:main commit + junction wiring + reconcile + per-worktree config commit sequence that makes
// "clone does everything" true at the command level — CloneHub itself stays git/geometry-focused so
// fabricengine never imports configsync (see fabricengine/clone.go's CloneResult doc comment for
// the import-cycle rationale).

package fabriccli

import (
	"context"
	"io"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// CloneAndWire clones a hub via fabricengine.CloneHub and drives the wiring sequence that makes
// "clone does everything" true: repo-wide fabric.yaml materialization, the weft:main anchor+config
// commit and push, warp junction wiring, per-worktree config reconciliation, and a commit of the
// resulting per-worktree module configs on the weft primary branch, with no push.
//
// It is the single wiring sequence shared by the cobra clone handler and
// internal/hubforge's hub factory; a second copy of this sequence anywhere is the
// drift this extraction exists to prevent.
//
// On error, the clone is left intact; the operator completes wiring with reconcile.
//
// CloneAndWire owns a recorder for this CLI-layer sequence itself: CloneHub's own record is already
// on res the moment it returns, so the recorder is seeded from res.Mutated() and every step below
// appends its own entry in execution order. Named results plus a defer installed right after the
// recorder exists is what carries the accumulated record through every subsequent
// `return fabricengine.CloneResult{}, err` site without editing each one individually — see this
// package's Decision doc for the idiom. The one return that precedes the recorder's existence (the
// CloneHub failure itself) returns res, err directly instead: CloneHub's own record is already in res
// at that point, and a zero return there would discard the hub the clone had just minted.
func CloneAndWire(cwd string, opts fabricengine.CloneOptions) (res fabricengine.CloneResult, err error) {
	res, err = fabricengine.CloneHub(cwd, opts)
	if err != nil {
		return res, err
	}

	rec := fabricengine.NewMutations(res.HubPath)
	rec.Extend(res.Mutated())
	defer func() { res.Mutations = rec.Snapshot() }()

	fabricResult, err := configsync.ReconcileFabricAt(res.BoardDir, true)
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	if fabricResult.Applied {
		rec.Append(fabricengine.KindFileWritten, configengine.ConfigFile(res.BoardDir, fabricResult.Module), "")
	}

	b := fabricengine.NewBolt(res.BoardDir)
	sha, committed, err := b.Commit("fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{})
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	if committed {
		rec.Append(fabricengine.KindCommitCreated, res.BoardDir, sha)
	}
	// Bolt.Push records nothing, and that is deliberate: it returns a bare error and reaches
	// gitrepo.PushCoalesced, which returns nil both when a push landed and when nothing was
	// unpushed to begin with, so a KindBranchPushed entry here would assert an outcome this call did
	// not observe. The commit above is already recorded, and branch_pushed is a git-state kind exempt
	// from the truthfulness oracle's commission direction, so omitting it costs the cross-check
	// nothing.
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
	// WireJunctionsWith needs no entry of its own beyond what it records internally: it appends
	// link_created directly into rec.
	if err := fabricengine.WireJunctionsWith(rec, l, filepath.Base(l.WorktreePath()), names); err != nil {
		return fabricengine.CloneResult{}, err
	}

	// .lyx is excluded through the warp's .git/info/exclude by WireJunctions above,
	// not through a tracked .gitignore entry in the user's own repo: a committed
	// entry would advertise that LYX is in use, and a warp→weft junction must never
	// leave a tracked artifact behind in the user's repo.
	//
	// ReconcileAll returns one Result per registered module, each reporting whether that module's own
	// file was written — record one KindFileWritten per Result whose Applied is true, not one entry
	// for the call as a whole, so every materialised per-worktree config file is individually covered.
	results, err := configsync.ReconcileAll(res.WeftBase, true)
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	var relPaths []string
	for _, r := range results {
		if r.Applied {
			rec.Append(fabricengine.KindFileWritten, configengine.ConfigFile(res.WeftBase, r.Module), "")
			relPaths = append(relPaths, configengine.ConfigFileRel(r.Module))
		}
	}

	// Commit the per-worktree module configs ReconcileAll just materialised, on the weft primary
	// branch, with no push: an adopt-path re-clone leaves relPaths empty, which
	// CommitAnchoredPaths's own len(relPaths) == 0 guard treats as a legitimate no-op, taking no
	// lock and recording nothing. CommitWeftPaths appends its own KindCommitCreated entry at its
	// success site, so no recording happens here.
	if _, _, err := fabricengine.CommitAnchoredPaths(rec, l, relPaths, "fabric clone: record module configs", fabricengine.SyncOptions{}); err != nil {
		return fabricengine.CloneResult{}, err
	}

	return res, nil
}

// runCloneWithReset executes the clone subcommand. The hub teardown for an idempotent re-clone is no
// longer performed here: it is driven through CloneOptions.Reset inside CloneHub itself, which can
// derive the hub path in either the one- or two-argument form. It parses arguments, resolves the
// seam cwd, derives the clone destination from into, and delegates the entire clone-and-wire
// sequence to CloneAndWire. The returned envelope carries "hub" and "anchor" from the resolved
// geometry, plus "warp" (the effective warp URL, supplied or derived) and "warp_binding_recorded"
// (whether this clone wrote the .lyx-warp record) — both always present so a consumer never has to
// distinguish absent from false.
func runCloneWithReset(ctx context.Context, out io.Writer, args []string, reset bool, subpath string, forceBootstrap bool, into string) int {
	// Nothing has been mutated yet at cwd resolution: a bare output.Err carries no record.
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	if len(args) != 1 && len(args) != 2 {
		return output.Err(out, "usage: lyx fabric clone [--reset] [--subpath <rel>] [--force-bootstrap] [--into <dir>] <weft-url> [<warp-url>]")
	}
	weftURL := args[0]
	warpURL := ""
	if len(args) == 2 {
		warpURL = args[1]
	}

	// The new hub is created in this directory, never the process cwd: an empty --into
	// yields the seam cwd unchanged, an absolute --into is cleaned as given, and a
	// relative --into resolves against the seam cwd — resolving against the process
	// cwd here would reintroduce the very dependency lyxcwd exists to remove, in the
	// one verb where cwd is a destination rather than a lookup anchor.
	dest := cwd
	switch {
	case into == "":
		// dest already equals cwd.
	case filepath.IsAbs(into):
		dest = filepath.Clean(into)
	default:
		dest = filepath.Join(cwd, into)
	}

	res, err := CloneAndWire(dest, fabricengine.CloneOptions{
		WeftURL:        weftURL,
		WarpURL:        warpURL,
		Subpath:        subpath,
		Reset:          reset,
		ForceBootstrap: forceBootstrap,
	})
	if err != nil {
		// CloneAndWire's defer has already populated res by the time it returns, so this failure
		// path carries whatever the mutated-then-errored sequence accumulated.
		return errWithRecord(out, res.Mutated(), err)
	}

	return okWithRecord(out, res.Mutated(), map[string]any{
		"hub":                   res.HubPath,
		"anchor":                res.Anchor,
		"warp":                  res.WarpURL,
		"warp_binding_recorded": res.WarpBindingRecorded,
	})
}
