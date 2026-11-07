// clone.go implements the fabric clone subcommand and the wiring a new clone needs once its hub exists.
// The wiring lives here because fabricengine must not import configsync, which would close an import cycle.

package fabriccli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// CloneAndWire clones a hub and wires it, so that a successful clone needs no reconcile.
// It materializes the hub-wide config, then commits it on weft:main and pushes it.
// It then wires the warp junctions and reconciles the per-worktree configs.
// It commits the resulting per-worktree module configs on the weft primary branch, with no push.
// It then tells the worktree seam that the prime was made.
// A failed event's warning goes in SeamWarning, or in the error when the walk that follows fails.
// The anchor marker is already committed on the prime's weft branch when the wiring starts.
//
// Keeping the sequence in one place stops a second copy from drifting away from it.
//
// On error, the clone is left intact, and the operator completes the wiring with `lym fabric reconcile`.
// The result's Mutations records every change the call made.
func CloneAndWire(cwd string, opts fabricengine.CloneOptions) (res fabricengine.CloneResult, err error) {
	res, err = fabricengine.CloneHub(cwd, opts)
	if err != nil {
		return res, err
	}

	rec := fabricengine.NewMutations(res.HubPath)
	rec.Extend(res.Mutated())
	defer func() { res.Mutations = rec.Snapshot() }()

	// No warning is raised for a template-seeded module: no vocabulary can predate a fresh clone.
	// The whole-repo commit below carries every file this call wrote.
	boardSeededFromNothing := !anyBaseFilePresent(res.BoardDir)
	hubWideResults, err := configsync.ReconcileHubWideAt(res.BoardDir, res.WeftBase, configsync.HubWideOptions{Apply: true})
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	for _, hubWideResult := range hubWideResults {
		if hubWideResult.Applied {
			rec.Append(fabricengine.KindFileWritten, configengine.ConfigFile(res.BoardDir, hubWideResult.Module), "")
		}
		if hubWideResult.WroteTemplateBase {
			rec.Append(fabricengine.KindFileWritten, configengine.TemplateBaseFile(res.BoardDir, hubWideResult.Module), "")
		}
	}
	// A board seeded from nothing holds only sparse files, so it is marked here.
	// The board commit below carries the marker.
	// A board that already held a base file is left unmarked:
	// the walk at the end strips it once and commits the marker itself.
	markerPath := configengine.BaseSparseMarker(res.BoardDir)
	if boardSeededFromNothing {
		if err := configsync.WriteBaseSparseMarker(res.BoardDir); err != nil {
			return fabricengine.CloneResult{}, err
		}
		rec.Append(fabricengine.KindFileWritten, markerPath, "")
	}

	b := fabricengine.NewBolt(res.BoardDir)
	sha, committed, err := b.Commit("fabric clone: record repo-wide config", fabricengine.SyncOptions{})
	if err != nil {
		// An untracked marker would make the next start verb's walk see a marked board and skip its one-time strip.
		if boardSeededFromNothing {
			_ = os.Remove(markerPath)
		}
		return fabricengine.CloneResult{}, err
	}
	if committed {
		rec.Append(fabricengine.KindCommitCreated, res.BoardDir, sha)
	}
	// The push records no branch_pushed entry, and that is deliberate.
	// The push returns nil both when a push landed and when nothing was unpushed.
	// Such an entry would claim a push this call did not observe.
	// The commit above is already recorded.
	// A seed-commit drop behind the push is recorded in rec as commits_dropped.
	if err := b.PushRecorded(fabricengine.SyncOptions{}, rec); err != nil {
		return fabricengine.CloneResult{}, err
	}

	l, err := lymcwd.Resolve(res.PrimeCwd)
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	names, err := fabricengine.WiredNames(configengine.Dirs{Anchor: res.WeftBase, Board: res.BoardDir})
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	// WireJunctionsWith records its own link_created entries in rec, so none is added here.
	if err := fabricengine.WireJunctionsWith(rec, l, filepath.Base(l.WorktreePath()), names); err != nil {
		return fabricengine.CloneResult{}, err
	}
	// .lym is excluded through the warp's .git/info/exclude by WireJunctionsWith above, not a .gitignore entry.
	// A committed .gitignore entry in the user's own repo would advertise that LYM is in use,
	// and a warp→weft junction must never leave a tracked artifact behind in the user's repo.
	//
	// ReconcileAll returns the results of its declared key moves first, then one Result per registered module.
	// Each Result reports whether that module's own file was written, or, for a retired hub-wide copy, deleted.
	// The loop below records one KindFileWritten per applied per-worktree Result,
	// so every changed per-worktree config file is covered individually.
	results, err := configsync.ReconcileAll(res.WeftBase, res.BoardDir, configsync.Options{Apply: true})
	if err != nil {
		return fabricengine.CloneResult{}, err
	}
	var relPaths []string
	for _, r := range results {
		if r.Applied && r.Layer == configengine.LayerWorktree {
			rec.Append(fabricengine.KindFileWritten, configengine.ConfigFile(res.WeftBase, r.Module), "")
			relPaths = append(relPaths, configengine.ConfigFileRel(r.Module))
		}
	}

	// Commit the per-worktree module configs ReconcileAll just materialised or retired.
	// The commit goes on the weft primary branch, with no push.
	// An adopt-path re-clone leaves relPaths empty.
	// The commit then is a legitimate no-op, taking no lock and recording nothing.
	// The commit records its own KindCommitCreated entry at its success site, so nothing is recorded here.
	if _, _, err := fabricengine.CommitAnchoredPaths(rec, l, relPaths, "fabric clone: record module configs", fabricengine.SyncOptions{}); err != nil {
		return fabricengine.CloneResult{}, err
	}

	// The prime is now set up as an added pair is when its seam fires:
	// wired, its exclude lines seeded, the Bolt checkout present and its module configs committed.
	// The render reads only the Bolt checkout's heddle store and the shipped layer, so it needs nothing the walk writes.
	// A seam failure never fails the clone; its warning reaches the operator with the render verb that repairs it.
	// A walk failure below returns that warning in its error, since the failure envelope has no field for it.
	res.SeamWarning = fabricengine.NotifyWorktreeMade(l)

	// A new hub has no build stamp, so the walk seeds every absent per-worktree repo default sparse.
	// The header-only base comes from the template, and the prime's non-template units come from the prime.
	// The walk commits them on the board and pushes it.
	// A walk failure leaves the clone intact, and the next start verb's walk seeds the defaults.
	// The same walk migrates the prime, so it arrives with the sparse marker committed.
	absentDefaults := absentRepoDefaults(res.BoardDir)
	markerBefore := fileExists(configengine.SparseMarker(res.WeftBase))
	if err := hubreconcile.Ensure(hubreconcile.Geometry{BoardDir: res.BoardDir, WorktreePath: l.WorktreePath()}, hubreconcile.Options{}); err != nil {
		if res.SeamWarning != "" {
			err = fmt.Errorf("%w; the prime was not rendered: %s", err, res.SeamWarning)
		}
		return fabricengine.CloneResult{}, err
	}
	for _, module := range absentDefaults {
		if _, statErr := os.Stat(configengine.ConfigFile(res.BoardDir, module)); statErr == nil {
			rec.Append(fabricengine.KindFileWritten, configengine.ConfigFile(res.BoardDir, module), "")
		}
	}
	if marker := configengine.SparseMarker(res.WeftBase); !markerBefore && fileExists(marker) {
		rec.Append(fabricengine.KindFileWritten, marker, "")
	}

	return res, nil
}

// fileExists reports whether path names an existing file.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// anyBaseFilePresent reports whether any registered module's base file exists under boardDir.
func anyBaseFilePresent(boardDir string) bool {
	for _, module := range configreg.Modules() {
		if fileExists(configengine.ConfigFile(boardDir, module.Name)) {
			return true
		}
	}
	return false
}

// absentRepoDefaults returns the per-worktree modules whose repo default is absent under boardDir.
func absentRepoDefaults(boardDir string) []string {
	var absent []string
	for _, module := range configreg.Modules() {
		if module.HubWide {
			continue
		}
		if _, err := os.Stat(configengine.ConfigFile(boardDir, module.Name)); os.IsNotExist(err) {
			absent = append(absent, module.Name)
		}
	}
	return absent
}

// runCloneWithReset executes the clone subcommand.
// The hub teardown of an idempotent re-clone is driven through CloneOptions.Reset inside CloneHub.
// CloneHub can derive the hub path in either the one- or two-argument form.
// The envelope carries "hub" and "anchor" from the resolved geometry.
// "warp" is the effective warp URL, supplied or derived.
// "warp_binding_recorded" reports whether this clone wrote the .lym-warp record.
// "shortname" is the effective repo shortname.
// "shortname_recorded" reports whether this clone wrote the .lym-shortname record.
// "warning" is empty when there is none.
// "seam_warning" is the warning of a failed render into the prime, naming `lym heddle render` there; empty when none.
// Every field is always present, so a consumer never has to tell an absent field from a false one.
func runCloneWithReset(ctx context.Context, out io.Writer, args []string, reset bool, subpath string, forceBootstrap bool, into string, shortname string) int {
	// Nothing has been mutated yet at cwd resolution: a bare output.Err carries no record.
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	if len(args) != 1 && len(args) != 2 {
		return output.Err(out, "usage: lym fabric clone [--shortname <shortname>] [--reset] [--subpath <rel>] [--force-bootstrap] [--into <dir>] <weft-url> [<warp-url>]")
	}
	weftURL := args[0]
	warpURL := ""
	if len(args) == 2 {
		warpURL = args[1]
	}

	// The new hub is created in this directory, never the process cwd.
	// An empty --into yields the seam cwd unchanged, an absolute --into is cleaned as given,
	// and a relative --into resolves against the seam cwd.
	// Resolving against the process cwd would reintroduce the dependency lymcwd exists to remove.
	// Here cwd is a destination rather than a lookup anchor.
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
		Shortname:      shortname,
	})
	if err != nil {
		// CloneAndWire's defer sets res.Mutations before it returns,
		// so this failure path carries every mutation made before the error.
		return errWithRecord(out, res.Mutated(), err)
	}

	return okWithRecord(out, res.Mutated(), map[string]any{
		"hub":                   res.HubPath,
		"anchor":                res.Anchor,
		"warp":                  res.WarpURL,
		"warp_binding_recorded": res.WarpBindingRecorded,
		"shortname":             res.Shortname,
		"shortname_recorded":    res.ShortnameRecorded,
		"warning":               res.Warning,
		"seam_warning":          res.SeamWarning,
	})
}
