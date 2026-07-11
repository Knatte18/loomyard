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
	"github.com/Knatte18/loomyard/internal/hubgeometry"
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
//   - Computes cfgPath := hubgeometry.ConfigFile(baseDir, m.Name)
//   - Reads existing bytes from disk (absent file → empty []byte, not an error)
//   - Calls yamlengine.Reconcile([]byte(m.Template()), existing)
//   - When apply && (fileAbsent || len(added)+len(removed) > 0):
//     writes merged via fsx.AtomicWriteBytes and sets Applied=true
//   - Returns a Result for each module
//
// Seed-only modules (m.SeedOnly, e.g. "models") take a different branch: they
// have an open-ended key set the operator owns, so they never route through
// yamlengine.Reconcile (whose merged output is only *equivalent* to the
// template — not byte-identical — which would degrade an annotated seed's
// comments/formatting). When the file is present, it is reported untouched
// (Applied: false, no Added/Removed — the file is never parsed, diffed, or
// written). When the file is absent, the template is written VERBATIM via
// fsx.AtomicWriteBytes (when apply) and every template leaf key-path is
// reported as Added via yamlengine.MissingKeys, so initengine's
// Applied && len(Added) > 0 && len(Removed) == 0 "created" heuristic still
// fires correctly.
//
// When apply is false, files are never written and Applied is always false.
// The function returns the slice of results and any error encountered during
// reconciliation (I/O or YAML parsing).
func ReconcileAll(baseDir string, apply bool) ([]Result, error) {
	var results []Result

	for _, m := range configreg.Modules() {
		cfgPath := hubgeometry.ConfigFile(baseDir, m.Name)

		// Read existing config file (missing file → empty bytes)
		existing, err := os.ReadFile(cfgPath)
		fileAbsent := os.IsNotExist(err)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config for %s: %w", m.Name, err)
		}
		if fileAbsent {
			existing = []byte{}
		}

		if m.SeedOnly {
			if !fileAbsent {
				// Present seed-only file is operator-owned: never parsed,
				// diffed, or written.
				results = append(results, Result{Module: m.Name, Applied: false})
				continue
			}

			// Absent seed-only file: materialize the template verbatim
			// rather than routing through yamlengine.Reconcile, whose
			// marshalled output normalizes indentation/blank
			// lines/comment placement and would degrade the annotated
			// seed.
			added, err := yamlengine.MissingKeys([]byte(m.Template()), nil)
			if err != nil {
				return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
			}

			result := Result{Module: m.Name, Added: added, Applied: false}
			if apply {
				if err := fsx.AtomicWriteBytes(cfgPath, []byte(m.Template())); err != nil {
					return nil, fmt.Errorf("write config for %s: %w", m.Name, err)
				}
				result.Applied = true
			}
			results = append(results, result)
			continue
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
