// update.go implements the lyx update command for reconciling module configs.
//
// The update command reconciles all module configuration files against their
// live templates, reporting added/removed keys and optionally writing changes.
// Dry-run is the default; --apply writes atomically to disk.

package update

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// RunCLI is the entry point for the lyx update command.
//
// It parses a flag.FlagSet with a --apply bool flag (default false for dry-run),
// resolves the layout via paths.Getwd() and paths.Resolve(), computes baseDir,
// calls configsync.ReconcileAll(), and emits JSON output.
//
// On success, outputs JSON with shape {"ok":true,"applied":<bool>,"modules":[...]}.
// On error, outputs {"ok":false,"error":"..."} and returns 1.
// Dry-run reports what WOULD change without writing.
func RunCLI(out io.Writer, args []string) int {
	fs := flag.NewFlagSet("lyx update", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "apply changes to disk (default: dry-run)")

	// Suppress flag.PrintDefaults() for consistency with other CLIs
	fs.Usage = func() {}

	if err := fs.Parse(args); err != nil {
		return output.Err(out, err.Error())
	}

	// Resolve the current working directory and layout
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("getwd: %v", err))
	}

	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("resolve layout: %v", err))
	}

	// Compute baseDir as in configcli.dispatch
	baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)

	// Reconcile all modules
	results, err := configsync.ReconcileAll(baseDir, *apply)
	if err != nil {
		return output.Err(out, fmt.Sprintf("reconcile: %v", err))
	}

	// Build module result objects for JSON output
	modules := make([]map[string]any, len(results))
	for i, result := range results {
		modules[i] = map[string]any{
			"module":  result.Module,
			"added":   result.Added,
			"removed": result.Removed,
			"applied": result.Applied,
		}
	}

	// Emit JSON output with ok=true
	return output.Ok(out, map[string]any{
		"applied": *apply,
		"modules": modules,
	})
}
