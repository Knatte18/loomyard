// configsync.go implements reconciliation of all module configs against their templates.
//
// It provides atomic writes and per-module reconciliation via yamlengine.Reconcile,
// tracking added/removed keys and applying changes when requested.

package configsync

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// legacyFabricConfigModules are the pre-cutover config modules whose values
// the fabric module now covers: warp.yaml held branch_prefix, weft.yaml held
// pathspec -- both flat, single-key top-level documents that fabric.yaml's
// two-key template subsumes.
var legacyFabricConfigModules = []string{"warp", "weft"}

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
	// MigratedFrom names the pre-cutover legacy config modules (e.g. "warp",
	// "weft") whose on-disk values were folded into this reconcile instead of
	// the bare template default. Non-empty only for "fabric", only when its
	// own config file is absent AND at least one legacy file is present and
	// parseable -- see legacyFabricConfig. Populated on a dry run too (apply
	// false), so the operator can see the migration is pending before it
	// applies; the legacy files themselves are only pruned when Applied.
	MigratedFrom []string
}

// legacyFabricConfig reads whichever of the pre-cutover warp.yaml/weft.yaml
// config files under baseDir's config dir are present and parse as YAML, and
// concatenates their raw bytes into a single document. Because both files
// are flat, single-key top-level mappings whose keys (branch_prefix,
// pathspec) are disjoint and match fabric.yaml's own two-key template,
// concatenating them is a valid combined YAML mapping that yamlengine.Reconcile
// can merge against the fabric template exactly like any other existing
// config file: a present, parseable legacy value overrides the template
// default; an absent or unparseable one leaves the template default in place.
//
// A legacy file that is missing, empty, or fails to parse contributes
// nothing and is NOT named in the returned migratedFrom -- callers must not
// delete a legacy file this function did not report migrating, since an
// unparseable file's value was never actually carried forward and deleting
// it would lose data rather than migrate it.
func legacyFabricConfig(baseDir string) (existing []byte, migratedFrom []string) {
	for _, legacy := range legacyFabricConfigModules {
		data, err := os.ReadFile(hubgeometry.ConfigFile(baseDir, legacy))
		if err != nil {
			continue // absent or unreadable: nothing to migrate from this file
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		var probe any
		if err := yaml.Unmarshal(data, &probe); err != nil {
			continue // unparseable: leave it on disk, don't guess at its value
		}
		existing = append(existing, data...)
		existing = append(existing, '\n')
		migratedFrom = append(migratedFrom, legacy)
	}
	return existing, migratedFrom
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
//
// "fabric" carries one more special case, the fabric-cutover's one-shot config
// migration: when fabric.yaml is absent, legacyFabricConfig folds in whatever
// the pre-cutover warp.yaml/weft.yaml files still carry (branch_prefix,
// pathspec) instead of writing the bare template default, reports which
// legacy files contributed via Result.MigratedFrom, and — only when apply is
// true and the write lands — prunes those legacy files so the migration does
// not re-fire. See legacyFabricConfig's doc comment for exactly which legacy
// files count as migrated.
//
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

		// One-shot fabric-cutover migration: when fabric.yaml is absent,
		// fold in whatever the pre-cutover warp.yaml/weft.yaml carry instead
		// of writing the bare template default. Fires only for "fabric" --
		// this is inherently a one-off migration for this one cutover, not a
		// generic legacy-config mechanism other modules should inherit.
		var migratedFrom []string
		if fileAbsent && m.Name == "fabric" {
			existing, migratedFrom = legacyFabricConfig(baseDir)
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
			Module:       m.Name,
			Added:        added,
			Removed:      removed,
			Applied:      false,
			MigratedFrom: migratedFrom,
		}

		// Determine if we should write: apply flag + (file absent OR changes detected)
		hasChanges := len(added)+len(removed) > 0

		if apply && (fileAbsent || hasChanges) {
			if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
				return nil, fmt.Errorf("write config for %s: %w", m.Name, err)
			}
			result.Applied = true

			// Prune legacy files whose values were actually folded into this
			// write -- .yaml deletion is otherwise pointless (a re-run with
			// apply=false must not delete anything, so this only runs inside
			// the apply branch) and only for the ones legacyFabricConfig
			// confirmed it could parse and carry forward.
			for _, legacy := range migratedFrom {
				legacyPath := hubgeometry.ConfigFile(baseDir, legacy)
				if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("remove migrated legacy config %s: %w", legacy, err)
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}
