// configsync.go implements reconciliation of all module configs against their templates.
//
// It provides atomic writes and per-module reconciliation via yamlengine.Reconcile, tracking
// added/removed keys and applying changes when requested.
// ReconcileAll covers the per-worktree modules at a worktree's anchor;
// ReconcileHubWideAt covers the hub-wide modules at the board dir.

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

// legacyConfigModules maps a module to the pre-cutover config modules whose values it now covers.
// fabric covers warp.yaml, which held branch_prefix, and weft.yaml, which held
// pathspec -- both flat, single-key top-level documents that fabric.yaml's
// two-key template subsumes.
var legacyConfigModules = map[string][]string{
	"fabric": {"warp", "weft"},
}

// Seed values name which input a reconcile started from.
const (
	// SeedHub is a hub file already present at the board dir.
	SeedHub = "hub"
	// SeedPrime is the prime worktree's copy, read only when the hub file is absent.
	SeedPrime = "prime"
	// SeedLegacy is the pre-cutover legacy files folded in when the hub file is absent.
	SeedLegacy = "legacy"
	// SeedTemplate is the module's embedded template.
	SeedTemplate = "template"
)

// Result represents the reconciliation result for a single config module.
type Result struct {
	// Module is the name of the config module (e.g., "board", "worktree", "fabric").
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
	// parseable -- see legacyConfig. Populated on a dry run too (apply
	// false), so the operator can see the migration is pending before it
	// applies; the legacy files themselves are only pruned when Applied.
	MigratedFrom []string
	// Seed names the input ReconcileHubWideAt reconciled: SeedHub, SeedPrime, SeedLegacy or
	// SeedTemplate.
	// Set on a dry run too; empty for ReconcileAll results.
	Seed string
}

// legacyConfig reads the pre-cutover config files the module covers, present
// under baseDir's config dir, and concatenates them into a single YAML document.
// A legacy file that is missing, empty, or fails to parse contributes nothing
// and is NOT named in migratedFrom — callers must not delete unparseable files,
// since their values were never carried forward.
func legacyConfig(module, baseDir string) (existing []byte, migratedFrom []string) {
	for _, legacy := range legacyConfigModules[module] {
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

// ReconcileAll reconciles all module config files against their templates, returning the slice of
// results and any I/O or YAML parsing error.
// Seed-only modules (e.g. "models") with present files are reported untouched;
// absent files materialize the template verbatim. "fabric" is skipped (see ReconcileHubWideAt for
// the hub-wide counterpart).
// When apply is false, files are never written.
func ReconcileAll(baseDir string, apply bool) ([]Result, error) {
	var results []Result

	for _, m := range configreg.Modules() {
		if m.Name == "fabric" {
			// fabric's config is repo-wide, not per-worktree: pathspec/branch_prefix
			// are materialized once at clone via ReconcileHubWideAt, keyed on the
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

		merged, added, removed, err := yamlengine.Reconcile([]byte(m.Template()), existing, m.OpenMaps...)
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

// primeConfig reads the prime worktree's copy of a module's config under primeBaseDir.
// It returns nil when there is no prime, or the file is absent, empty or unparseable.
func primeConfig(primeBaseDir, module string) []byte {
	if primeBaseDir == "" {
		return nil
	}
	data, err := os.ReadFile(configengine.ConfigFile(primeBaseDir, module))
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	var probe any
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil
	}
	return data
}

// ReconcileHubWideAt reconciles every hub-wide module's file at configengine.ConfigFile(boardDir,
// name), in registry order, and returns one Result per module.
// primeBaseDir is the prime worktree's anchor dir; empty means no prime is resolvable.
//
// A present hub file is the reconcile input and is never replaced.
// An absent file of a module with open maps is seeded from the prime's copy when that copy exists
// and parses; a module without open maps never reads the prime, since its per-worktree copy was
// never in effect.
// Otherwise an absent file folds in the module's pre-cutover legacy files, and failing that
// starts from the template.
// Legacy files are pruned only when apply succeeds.
// When apply is false, nothing is written or removed.
func ReconcileHubWideAt(boardDir, primeBaseDir string, apply bool) ([]Result, error) {
	var results []Result

	for _, m := range configreg.Modules() {
		if !m.HubWide {
			continue
		}

		cfgPath := configengine.ConfigFile(boardDir, m.Name)

		existing, err := os.ReadFile(cfgPath)
		fileAbsent := os.IsNotExist(err)
		if err != nil && !fileAbsent {
			return nil, fmt.Errorf("read config for %s: %w", m.Name, err)
		}

		seed := SeedHub
		var migratedFrom []string
		if fileAbsent {
			existing = nil
			seed = SeedTemplate
			if len(m.OpenMaps) > 0 {
				if prime := primeConfig(primeBaseDir, m.Name); prime != nil {
					existing = prime
					seed = SeedPrime
				}
			}
			if seed == SeedTemplate {
				if legacy, from := legacyConfig(m.Name, boardDir); len(from) > 0 {
					existing = legacy
					migratedFrom = from
					seed = SeedLegacy
				}
			}
		}

		merged, added, removed, err := yamlengine.Reconcile([]byte(m.Template()), existing, m.OpenMaps...)
		if err != nil {
			return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
		}

		result := Result{
			Module:       m.Name,
			Added:        added,
			Removed:      removed,
			MigratedFrom: migratedFrom,
			Seed:         seed,
		}

		if apply && (fileAbsent || len(added)+len(removed) > 0) {
			if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
				return nil, fmt.Errorf("mkdir config dir for %s: %w", m.Name, err)
			}
			if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
				return nil, fmt.Errorf("write config for %s: %w", m.Name, err)
			}
			result.Applied = true

			for _, legacy := range migratedFrom {
				if err := os.Remove(configengine.ConfigFile(boardDir, legacy)); err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("remove migrated legacy config %s: %w", legacy, err)
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}
