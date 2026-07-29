// init.go implements the core logic for lyx init.
//
// Init scaffolds the _lyx directory structure, creates all module config files
// via reconciliation, and maintains the managed .gitignore block. It is
// idempotent and never clobbers existing user-edited config files.

// Package initengine implements the core logic behind lyx init and
// lyx init --undo. It has no dependency on cobra, io.Writer, or exit codes;
// internal/initcli is a thin CLI wrapper around this package.
package initengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// ModuleResult reports the reconciliation outcome for one module's config file.
type ModuleResult struct {
	Module  string
	Status  string // "created" or "exists"
	Applied bool
}

// InitResult summarizes what Init changed.
type InitResult struct {
	LyxDir     string // "created" or "exists"
	PatternDir string // "created" or "exists"
	Gitignore  string // "updated" or "unchanged"
	Modules    []ModuleResult
}

// Init activates the fabric topology by wiring cwd-keyed junctions, then
// reconciles the config layer in cwd by:
//  1. Resolving the layout from cwd
//  2. Checking for a weft pairing; if absent, returning an error early
//  3. Observing whether each HOST junction path already exists, BEFORE wiring
//  4. Materialising fabric.yaml on the weft base (configsync.ReconcileAll), then
//     loading the wired name-set from it, then wiring the host junctions via
//     fabricengine.WireJunctions — reordered from a bare WireJunctions call because
//     WireJunctions itself no longer reads config, so Init must obtain names before
//     calling it, and on a genesis first-ever init no fabric.yaml exists yet
//  5. Creating _lyx and _pattern directories (and _lyx/config)
//  6. Maintaining the managed .gitignore block for .lyx/
//  7. Feeding InitResult.Modules from step 4's pre-wire ReconcileAll results (no
//     second ReconcileAll call — see below)
//
// Step 4's config write targets the WEFT base (filepath.Join(l.WeftWorktree(),
// l.RelPath)), never cwd: writing to cwd before wiring would trip seedLyxJunction's
// host-pristine guard, and after wiring cwd/_lyx and the weft base's _lyx are the
// same physical directory (junction) anyway, so the reconcile result is identical
// either way. There is no chicken-and-egg here — WireJunctions itself never reads
// config; the pre-seed exists solely so Init has a pathspec to read names from.
//
// Idempotent: junction wiring is idempotent (via fslink.IsLink/PointsTo); a second
// run does not clobber existing config files (Reconcile preserves user values) and
// does not duplicate the .gitignore block.
func Init(cwd string) (InitResult, error) {
	// Resolve layout from cwd (needed for weft sibling derivation and slug).
	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		// hubgeometry.Resolve's error is already self-describing; pass it
		// through bare rather than restating it with a redundant prefix.
		return InitResult{}, err
	}

	// Check for weft pairing before activating topology.
	// If no weft sibling exists, the host is unpaired (dormant Add); report and exit.
	weftWorktree := l.WeftWorktree()
	if _, statErr := os.Stat(weftWorktree); os.IsNotExist(statErr) {
		return InitResult{}, fmt.Errorf("no weft pairing — run `lyx fabric add` or `lyx fabric clone` first")
	}

	slug := filepath.Base(l.WorktreeRoot)
	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
	patternDir := filepath.Join(cwd, hubgeometry.PatternDirName)

	// Observe both HOST junction paths BEFORE WireJunctions runs. Since batch 3,
	// WireJunctions' seeder unconditionally materialises each junction's weft-side
	// target (os.MkdirAll), so a POST-wiring stat of the host junction path always
	// succeeds through the freshly-created junction — silently reporting "exists"
	// on a first-ever init. The host path itself is also the only reliable signal:
	// the weft-side target cannot be used instead, because a weft branch forks from
	// its parent's weft branch (see weftwiring.go) and so may already carry _lyx/
	// content inherited from that history even though THIS host worktree's Init has
	// never run. The host junction path, by contrast, is genuinely local to this
	// worktree and does not exist until some Init call creates it.
	lyxDirStatus, err := preWiringHostDirStatus(lyxDir)
	if err != nil {
		return InitResult{}, err
	}
	patternDirStatus, err := preWiringHostDirStatus(patternDir)
	if err != nil {
		return InitResult{}, err
	}

	// Materialise fabric.yaml on the WEFT base before wiring — on a genesis
	// first-ever init no config exists yet anywhere, and WireJunctions itself
	// no longer reads config, so Init must obtain a name-set to pass it. The
	// weft base (not cwd) is the write target: writing to cwd first would trip
	// seedLyxJunction's host-pristine guard, and after wiring cwd/_lyx and the
	// weft base's _lyx are the same physical directory via the junction, so
	// the reconcile result is identical either way. ReconcileAll (not a raw
	// template write) preserves the legacy warp/weft→fabric migration.
	weftBase := filepath.Join(l.WeftWorktree(), l.RelPath)
	if err := os.MkdirAll(hubgeometry.ConfigDir(weftBase), 0o755); err != nil {
		return InitResult{}, fmt.Errorf("failed to create weft _lyx/config directory: %w", err)
	}
	results, err := configsync.ReconcileAll(weftBase, true)
	if err != nil {
		return InitResult{}, fmt.Errorf("failed to reconcile configs: %w", err)
	}

	// Load the wired name-set from the config just materialised, then wire
	// the host junctions for the current worktree (keyed by its slug). There
	// is no chicken-and-egg: WireJunctions itself never reads config; the
	// pre-seed above exists only so Init has a pathspec to read names from.
	names, err := fabricengine.WiredNames(weftBase)
	if err != nil {
		return InitResult{}, fmt.Errorf("failed to load wired junction names: %w", err)
	}
	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		return InitResult{}, fmt.Errorf("failed to wire junctions: %w", err)
	}

	var result InitResult
	result.LyxDir = lyxDirStatus
	result.PatternDir = patternDirStatus

	// Ensure the host _lyx and _pattern directories exist. Both are redundant with
	// WireJunctions having just created a junction that resolves to a real
	// directory, but keep Init self-contained even if a future caller's contract
	// with WireJunctions ever changes.
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		return InitResult{}, fmt.Errorf("failed to create _lyx directory: %w", err)
	}
	if err := os.MkdirAll(patternDir, 0o755); err != nil {
		return InitResult{}, fmt.Errorf("failed to create _pattern directory: %w", err)
	}

	// Create _lyx/config/ subdirectory to hold configuration files.
	configDir := hubgeometry.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return InitResult{}, fmt.Errorf("failed to create _lyx/config directory: %w", err)
	}

	// Maintain managed block in .gitignore.
	changed, err := gitignore.Ensure(cwd, ".lyx/")
	if err != nil {
		return InitResult{}, fmt.Errorf("failed to update .gitignore: %w", err)
	}
	if changed {
		result.Gitignore = "updated"
	} else {
		result.Gitignore = "unchanged"
	}

	// Feed InitResult.Modules from the pre-wire ReconcileAll results computed
	// above (against the weft base) — no second ReconcileAll call here. After
	// wiring, cwd's _lyx is the same physical directory as the weft base's
	// _lyx (via the junction), so a second reconcile against cwd would only
	// re-observe the identical on-disk state the pre-wire call already wrote.
	result.Modules = make([]ModuleResult, len(results))
	for i, r := range results {
		// Determine if module was "created" (Applied && file absent at start)
		// or "exists" (file was already there, possibly updated).
		status := "exists"
		if r.Applied && len(r.Added) > 0 && len(r.Removed) == 0 {
			// Heuristic: if applied and has added keys but no removed, likely first creation.
			status = "created"
		}
		result.Modules[i] = ModuleResult{Module: r.Module, Status: status, Applied: r.Applied}
	}

	return result, nil
}

// preWiringHostDirStatus reports whether the host junction path dir (e.g. cwd/_lyx
// or cwd/_pattern) already exists, observed BEFORE WireJunctions runs. This is the
// pre-wiring observation Init's InitResult vocabulary is built on: see Init's godoc
// for why the host path — not the weft-side target — is the only reliable signal.
//
// Returns "created" if dir does not yet exist (this Init invocation is the one that
// will bring it into being via WireJunctions), "exists" if it is already a
// directory (a prior Init already wired it), or an error if dir exists but is not a
// directory — a real, non-directory file occupying a path Init expects to be (or
// become) a junction, which fabric must never silently paper over — or if the stat
// itself fails for a reason other than not-exist.
func preWiringHostDirStatus(dir string) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "created", nil
		}
		return "", fmt.Errorf("failed to stat %s: %w", filepath.Base(dir), err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s exists but is not a directory", filepath.Base(dir))
	}
	return "exists", nil
}
