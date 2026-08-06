// configsync.go implements reconciliation of all module configs against their templates.
//
// It provides atomic writes and per-module reconciliation via yamlengine.Reconcile, tracking added/removed keys and applying changes when requested.

package configsync

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fsx"
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

// legacyFabricConfig reads pre-cutover warp.yaml/weft.yaml files present
// under baseDir's config dir and concatenates them into a single YAML document.
// A legacy file that is missing, empty, or fails to parse contributes nothing
// and is NOT named in migratedFrom — callers must not delete unparseable files,
// since their values were never carried forward.
func legacyFabricConfig(baseDir string) (existing []byte, migratedFrom []string) {
	for _, legacy := range legacyFabricConfigModules {
		data, err := os.ReadFile(configengine.ConfigFile(baseDir, legacy))
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

// ReconcileAll reconciles all module config files against their templates, returning the slice of results and any I/O or YAML parsing error.
// Seed-only modules (e.g. "models") with present files are reported untouched;
// absent files materialize the template verbatim. "fabric" is skipped (see ReconcileFabricAt for the repo-wide counterpart).
// When apply is false, files are never written.
func ReconcileAll(baseDir string, apply bool) ([]Result, error) {
	var results []Result

	for _, m := range configreg.Modules() {
		if m.Name == "fabric" {
			// fabric's config is repo-wide, not per-worktree: pathspec/branch_prefix
			// are materialized once at clone via ReconcileFabricAt, keyed on the
			// board dir at fabricengine.BoardDir(Hub), never on a worktree baseDir.
			continue
		}

		cfgPath := configengine.ConfigFile(baseDir, m.Name)

		existing, err := os.ReadFile(cfgPath)
		fileAbsent := os.IsNotExist(err)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config for %s: %w", m.Name, err)
		}
		if fileAbsent {
			existing = []byte{}
		}

		var migratedFrom []string

		if m.SeedOnly {
			if !fileAbsent {
				results = append(results, Result{Module: m.Name, Applied: false})
				continue
			}
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

		hasChanges := len(added)+len(removed) > 0

		if apply && (fileAbsent || hasChanges) {
			if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
				return nil, fmt.Errorf("write config for %s: %w", m.Name, err)
			}
			result.Applied = true
			for _, legacy := range migratedFrom {
				legacyPath := configengine.ConfigFile(baseDir, legacy)
				if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("remove migrated legacy config %s: %w", legacy, err)
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// ReconcileFabricAt reconciles the repo-wide fabric.yaml at configengine.ConfigFile(boardDir, "fabric"), the counterpart to ReconcileAll's per-worktree loop.
// fabric's config is repo-wide (never per-worktree).
// When absent, it carries the one-shot fabric-cutover migration folding in pre-cutover warp.yaml/weft.yaml values and pruning them only when apply succeeds.
// Returns a Result matching ReconcileAll's shape,
// and any I/O or YAML parsing error.
func ReconcileFabricAt(boardDir string, apply bool) (Result, error) {
	template, ok := configreg.Template("fabric")
	if !ok {
		return Result{}, fmt.Errorf("configreg: unknown module %q", "fabric")
	}

	cfgPath := configengine.ConfigFile(boardDir, "fabric")

	existing, err := os.ReadFile(cfgPath)
	fileAbsent := os.IsNotExist(err)
	if err != nil && !fileAbsent {
		return Result{}, fmt.Errorf("read config for fabric: %w", err)
	}
	if fileAbsent {
		existing = []byte{}
	}

	var migratedFrom []string
	if fileAbsent {
		existing, migratedFrom = legacyFabricConfig(boardDir)
	}

	merged, added, removed, err := yamlengine.Reconcile([]byte(template()), existing)
	if err != nil {
		return Result{}, fmt.Errorf("reconcile fabric: %w", err)
	}

	result := Result{
		Module:       "fabric",
		Added:        added,
		Removed:      removed,
		Applied:      false,
		MigratedFrom: migratedFrom,
	}

	hasChanges := len(added)+len(removed) > 0

	if apply && (fileAbsent || hasChanges) {
		if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
			return Result{}, fmt.Errorf("mkdir config dir for fabric: %w", err)
		}
		if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
			return Result{}, fmt.Errorf("write config for fabric: %w", err)
		}
		result.Applied = true

		for _, legacy := range migratedFrom {
			legacyPath := configengine.ConfigFile(boardDir, legacy)
			if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
				return Result{}, fmt.Errorf("remove migrated legacy config %s: %w", legacy, err)
			}
		}
	}

	return result, nil
}
