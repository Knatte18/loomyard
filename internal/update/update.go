// update.go implements the lyx update command for reconciling module configs.
//
// The update command reconciles all module configuration files against their
// live templates, reporting added/removed keys and optionally writing changes.
// Dry-run is the default; --apply writes atomically to disk.

// Package update provides the cobra command and public seam for the lyx update command.
// It reconciles all module configuration files against their templates and reports
// added/removed keys, with --apply to write changes to disk.
package update

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// Command returns the cobra command for lyx update.
//
// The returned command is a leaf with Use "update". It reconciles all module
// config files against their templates, reporting added/removed keys. Dry-run
// is the default; --apply writes changes to disk. The public RunCLI seam
// delegates here via clihelp.Execute so all in-process callers continue to work.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "reconcile module configs against templates",
		Long: `update reconciles all module configuration files in _lyx/config/ against
their live templates, reporting added keys (new in template) and removed keys
(deleted from template). By default it is a dry-run: no files are written.
Pass --apply to write the reconciled files to disk atomically.`,
	}

	// --apply flag: default false (dry-run); pass --apply to write to disk.
	apply := cmd.Flags().Bool("apply", false, "apply changes to disk (default: dry-run)")

	// RunE reads the --apply flag value and delegates to runUpdate via a closure
	// that satisfies clihelp.WrapRun's func(out, args) int shape.
	cmd.RunE = clihelp.WrapRun(func(out io.Writer, args []string) int {
		return runUpdate(out, *apply)
	})

	return cmd
}

// RunCLI is the public seam for the lyx update command.
//
// It delegates to clihelp.Execute(Command(), out, args) so that all existing
// in-process callers and tests compile and pass unchanged. The cobra command
// carries both stdout and stderr into out for single-buffer test capture.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runUpdate is the package-private handler that contains the actual update logic.
//
// It resolves the current working directory and layout, computes the baseDir as
// filepath.Join(WorktreeRoot, RelPath), reconciles all module configs via
// configsync.ReconcileAll, and emits JSON output. When apply is false (dry-run),
// no files are written; when apply is true, changes are committed to disk.
//
// Returns exit code 0 on success, 1 on error.
func runUpdate(out io.Writer, apply bool) int {
	// Resolve the current working directory and layout.
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("getwd: %v", err))
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("resolve layout: %v", err))
	}

	// Compute baseDir as in configcli.dispatch: the host _lyx parent.
	baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)

	// Reconcile all modules; apply controls whether changes are written to disk.
	results, err := configsync.ReconcileAll(baseDir, apply)
	if err != nil {
		return output.Err(out, fmt.Sprintf("reconcile: %v", err))
	}

	// Build module result objects for JSON output.
	modules := make([]map[string]any, len(results))
	for i, result := range results {
		modules[i] = map[string]any{
			"module":  result.Module,
			"added":   result.Added,
			"removed": result.Removed,
			"applied": result.Applied,
		}
	}

	// Emit JSON output with ok=true.
	return output.Ok(out, map[string]any{
		"applied": apply,
		"modules": modules,
	})
}
