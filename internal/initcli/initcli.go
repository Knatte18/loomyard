// initcli.go implements the lyx init command.
//
// Scaffolds the _lyx directory structure, creates all module config files
// via reconciliation, and maintains the managed .gitignore block.
// This is idempotent and never clobbers existing user-edited config files.

package initcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// RunInit is the entry point for the lyx init command.
//
// It scaffolds the config layer in the current working directory by:
//   1. Creating _lyx and _lyx/config directories
//   2. Maintaining the managed .gitignore block for .lyx/
//   3. Reconciling all module config files against their templates via ReconcileAll
//
// Idempotent: a second run does not clobber existing config files (Reconcile
// preserves user values) and does not duplicate the .gitignore block.
//
// Returns a JSON summary with _lyx/gitignore status and per-module results,
// and exit code 0 on success, 1 on error.
func RunInit(out io.Writer, args []string) int {
	// Resolve current working directory
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to get working directory: %v", err))
	}

	// Track status for each step
	status := map[string]string{}

	// Step 1: Create _lyx directory
	lyxDir := filepath.Join(cwd, paths.LyxDirName)
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
	configDir := paths.ConfigDir(cwd)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return output.Err(out, fmt.Sprintf("failed to create _lyx/config directory: %v", err))
	}

	// Step 2: Maintain managed block in .gitignore
	changed, err := gitignore.Ensure(cwd, ".lyx/")
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to update .gitignore: %v", err))
	}

	if changed {
		status["gitignore"] = "updated"
	} else {
		status["gitignore"] = "unchanged"
	}

	// Step 3: Reconcile all module configs
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
