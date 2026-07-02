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
	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// ModuleResult reports the reconciliation outcome for one module's config file.
type ModuleResult struct {
	Module  string
	Status  string // "created" or "exists"
	Applied bool
}

// InitResult summarizes what Init changed.
type InitResult struct {
	LyxDir    string // "created" or "exists"
	Gitignore string // "updated" or "unchanged"
	Modules   []ModuleResult
}

// Init activates the warp topology by wiring cwd-keyed junctions, then
// reconciles the config layer in cwd by:
//  1. Resolving the layout from cwd
//  2. Checking for a weft pairing; if absent, returning an error early
//  3. Wiring the host _lyx junction via warpengine.WireJunctions
//  4. Creating _lyx and _lyx/config directories
//  5. Maintaining the managed .gitignore block for .lyx/
//  6. Reconciling all module config files against their templates via ReconcileAll
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
		return InitResult{}, fmt.Errorf("no weft pairing — run `lyx warp add` or `lyx warp clone` first")
	}

	// Wire junctions for the current worktree (keyed by its slug: filepath.Base(WorktreeRoot)).
	slug := filepath.Base(l.WorktreeRoot)
	if err := warpengine.WireJunctions(l, slug); err != nil {
		return InitResult{}, fmt.Errorf("failed to wire junctions: %w", err)
	}

	var result InitResult

	// Create _lyx directory (activation completed above).
	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
	info, err := os.Stat(lyxDir)
	if err != nil && !os.IsNotExist(err) {
		return InitResult{}, fmt.Errorf("failed to stat _lyx: %w", err)
	}

	if os.IsNotExist(err) {
		// Directory doesn't exist, create it.
		if err := os.MkdirAll(lyxDir, 0o755); err != nil {
			return InitResult{}, fmt.Errorf("failed to create _lyx directory: %w", err)
		}
		result.LyxDir = "created"
	} else if info.IsDir() {
		// Directory already exists.
		result.LyxDir = "exists"
	} else {
		// Exists but is not a directory.
		return InitResult{}, fmt.Errorf("_lyx exists but is not a directory")
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

	// Reconcile all module configs.
	// Note: init uses cwd as baseDir (where the user runs 'lyx init'), while update uses
	// WorktreeRoot+RelPath. This is intentional—init is user-driven from any directory,
	// update is file-based from repo root.
	results, err := configsync.ReconcileAll(cwd, true)
	if err != nil {
		return InitResult{}, fmt.Errorf("failed to reconcile configs: %w", err)
	}

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
