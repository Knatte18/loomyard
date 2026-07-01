// initcli.go implements the lyx init command.
//
// Scaffolds the _lyx directory structure, creates all module config files
// via reconciliation, and maintains the managed .gitignore block.
// This is idempotent and never clobbers existing user-edited config files.

// Package initcli provides the cobra command and public seam for the lyx init command.
// It scaffolds the _lyx directory structure and wires warp junctions in the current
// working directory.
package initcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// Command returns the cobra command for lyx init.
//
// The returned command is a leaf with Use "init". It scaffolds _lyx/config/ in
// the current directory, wires warp junctions, and maintains the managed
// .gitignore block. The public RunInit seam delegates here via clihelp.Execute,
// so all in-process callers continue to work unchanged.
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "scaffold _lyx/config/ in the current directory",
		Long: `init activates the lyx topology for the current worktree.

It wires cwd-keyed warp junctions, creates _lyx/ and _lyx/config/ directories,
maintains the managed .gitignore block for .lyx/, and reconciles all module
config files against their templates (idempotent: existing user edits are
preserved). A weft pairing must already exist (run 'lyx warp add' or
'lyx warp clone' first).`,
		RunE: clihelp.WrapRun(runInit),
	}
}

// RunInit is the public seam for the lyx init command.
//
// It delegates to clihelp.Execute(Command(), out, args) so that all existing
// in-process callers and tests compile and pass unchanged. The cobra command
// carries both stdout and stderr into out for single-buffer test capture.
func RunInit(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runInit is the package-private handler that contains the actual init logic.
//
// It activates the warp topology by wiring cwd-keyed junctions, then reconciles
// the config layer in the current working directory by:
//  1. Resolving the layout from cwd
//  2. Checking for a weft pairing; if absent, report and exit early
//  3. Wiring the host _lyx junction via warp.WireJunctions
//  4. Creating _lyx and _lyx/config directories
//  5. Maintaining the managed .gitignore block for .lyx/
//  6. Reconciling all module config files against their templates via ReconcileAll
//
// Idempotent: junction wiring is idempotent (via fslink.IsLink/PointsTo); a second run
// does not clobber existing config files (Reconcile preserves user values) and does not
// duplicate the .gitignore block.
//
// Returns exit code 0 on success, 1 on error.
func runInit(out io.Writer, args []string) int {
	// Resolve current working directory
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to get working directory: %v", err))
	}

	// Resolve layout from cwd (needed for weft sibling derivation and slug)
	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		// hubgeometry.Resolve's error is already self-describing; pass it
		// through bare rather than restating it with a redundant prefix.
		return output.Err(out, err.Error())
	}

	// Check for weft pairing before activating topology.
	// If no weft sibling exists, the host is unpaired (dormant Add); report and exit.
	weftWorktree := l.WeftWorktree()
	if _, statErr := os.Stat(weftWorktree); os.IsNotExist(statErr) {
		return output.Err(out, "no weft pairing — run `lyx warp add` or `lyx warp clone` first")
	}

	// Wire junctions for the current worktree (keyed by its slug: filepath.Base(WorktreeRoot)).
	slug := filepath.Base(l.WorktreeRoot)
	if err := warpengine.WireJunctions(l, slug); err != nil {
		return output.Err(out, fmt.Sprintf("failed to wire junctions: %v", err))
	}

	// Track status for each step
	status := map[string]string{}

	// Step 4: Create _lyx directory (activation completed in steps 1-3 above)
	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
	info, err := os.Stat(lyxDir)
	if err != nil && !os.IsNotExist(err) {
		return output.Err(out, fmt.Sprintf("failed to stat _lyx: %v", err))
	}

	if os.IsNotExist(err) {
		// Directory doesn't exist, create it
		if err := os.MkdirAll(lyxDir, 0o755); err != nil {
			return output.Err(out, fmt.Sprintf("failed to create _lyx directory: %v", err))
		}
		status["lyx_dir"] = "created"
	} else if info.IsDir() {
		// Directory already exists
		status["lyx_dir"] = "exists"
	} else {
		// Exists but is not a directory
		return output.Err(out, fmt.Sprintf("_lyx exists but is not a directory"))
	}

	// Create _lyx/config/ subdirectory to hold configuration files
	configDir := hubgeometry.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return output.Err(out, fmt.Sprintf("failed to create _lyx/config directory: %v", err))
	}

	// Step 5: Maintain managed block in .gitignore
	changed, err := gitignore.Ensure(cwd, ".lyx/")
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to update .gitignore: %v", err))
	}

	if changed {
		status["gitignore"] = "updated"
	} else {
		status["gitignore"] = "unchanged"
	}

	// Step 6: Reconcile all module configs.
	// Note: init uses cwd as baseDir (where the user runs 'lyx init'), while update uses WorktreeRoot+RelPath.
	// This is intentional—init is user-driven from any directory, update is file-based from repo root.
	results, err := configsync.ReconcileAll(cwd, true)
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to reconcile configs: %v", err))
	}

	// Build module result objects for JSON output
	modules := make([]map[string]any, len(results))
	for i, result := range results {
		// Determine if module was "created" (Applied && file absent at start)
		// or "exists" (file was already there, possibly updated)
		status := "exists"
		if result.Applied && len(result.Added) > 0 && len(result.Removed) == 0 {
			// Heuristic: if applied and has added keys but no removed, likely first creation
			status = "created"
		}

		modules[i] = map[string]any{
			"module":  result.Module,
			"status":  status,
			"applied": result.Applied,
		}
	}

	// Emit JSON output with ok=true
	return output.Ok(out, map[string]any{
		"lyx_dir":   status["lyx_dir"],
		"gitignore": status["gitignore"],
		"modules":   modules,
	})
}
