// configsync.go implements reconciliation of all module configs against their templates.
//
// It provides atomic writes and per-module reconciliation via yamlengine.Reconcile,
// tracking added/removed keys and applying changes when requested.

package configsync

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// Result represents the reconciliation result for a single config module.
type Result struct {
	// Module is the name of the config module (e.g., "board", "worktree", "weft").
	Module string
	// Added is the slice of key-paths newly discovered in the template.
	Added []string
	// Removed is the slice of key-paths that existed in the file but not in the template.
	Removed []string
	// Applied reports whether the file was written to disk.
	Applied bool
}

// ReconcileAll reconciles all module config files against their templates.
//
// For each module returned by configreg.Modules(), it:
//   - Computes cfgPath := paths.ConfigFile(baseDir, m.Name)
//   - Reads existing bytes from disk (absent file → empty []byte, not an error)
//   - Calls yamlengine.Reconcile([]byte(m.Template()), existing)
//   - When apply && (fileAbsent || len(added)+len(removed) > 0):
//     writes merged via fsx.AtomicWriteBytes and sets Applied=true
//   - Returns a Result for each module
//
// When apply is false, files are never written and Applied is always false.
// The function returns the slice of results and any error encountered during
// reconciliation (I/O or YAML parsing).
func ReconcileAll(baseDir string, apply bool) ([]Result, error) {
	var results []Result

	for _, m := range configreg.Modules() {
		cfgPath := paths.ConfigFile(baseDir, m.Name)

		// Read existing config file (missing file → empty bytes)
		existing, err := os.ReadFile(cfgPath)
		fileAbsent := os.IsNotExist(err)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config for %s: %w", m.Name, err)
		}
		if err != nil && os.IsNotExist(err) {
			existing = []byte{}
		}

		// Reconcile template against existing
		merged, added, removed, err := yamlengine.Reconcile([]byte(m.Template()), existing)
		if err != nil {
			return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
		}

		result := Result{
			Module:  m.Name,
			Added:   added,
			Removed: removed,
			Applied: false,
		}

		// Determine if we should write: apply flag + (file absent OR changes detected)
		hasChanges := len(added)+len(removed) > 0

		if apply && (fileAbsent || hasChanges) {
			if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
				return nil, fmt.Errorf("write config for %s: %w", m.Name, err)
			}
			result.Applied = true
		}

		results = append(results, result)
	}

	return results, nil
}
