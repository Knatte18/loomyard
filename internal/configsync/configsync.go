// configsync.go implements reconciliation of all module configs against their templates.
//
// It provides atomic writes and per-module reconciliation via yamlengine.Reconcile, tracking
// added/removed keys and applying changes when requested.
// ReconcileAll covers the per-worktree modules at a worktree's anchor;
// ReconcileHubWideAt covers the hub-wide modules at the board dir.
// Their failures are *FileError values naming the file that failed.

package configsync

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// legacyConfigModules maps a module to the pre-cutover config modules whose values it now covers.
// fabric covers warp.yaml, which held branch_prefix, and weft.yaml, which held pathspec.
// Both are flat, single-key top-level documents that fabric.yaml's two-key template subsumes.
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
	// Applied reports whether the file was written to disk, or deleted when Retired.
	Applied bool
	// MigratedFrom names the pre-cutover legacy config modules (e.g. "warp",
	// "weft") whose on-disk values were folded into this reconcile instead of
	// the bare template default. Non-empty only for "fabric", only when its
	// own config file is absent AND at least one legacy file is present and
	// parseable -- see legacyConfig. Populated on a dry run too (apply
	// false), so the operator can see the migration is pending before it
	// applies; the legacy files themselves are only pruned when Applied.
	MigratedFrom []string
	// Migrated lists the rewrites the module's Migrate hook made to a present file, in the hook's order.
	// Set on a dry run too; the file is rewritten only when Applied.
	Migrated []string
	// Seed names the input ReconcileHubWideAt reconciled: SeedHub, SeedPrime, SeedLegacy or SeedTemplate.
	// Set on a dry run too; empty for ReconcileAll results.
	Seed string
	// Retired reports that a hub-wide module's leftover per-worktree copy adds nothing to the hub file, so it is removable.
	// Applied reports the deletion happened.
	// Set only by ReconcileAll, on a dry run too.
	Retired bool
	// Divergent lists, as "<dotted.key>: <value>" sorted, each entry of a hub-wide module's per-worktree copy that the hub file lacks or holds with a different value.
	// The copy is left in place.
	// Set only by ReconcileAll.
	Divergent []string
}

// legacyConfig reads the pre-cutover config files the module covers, present under baseDir's config dir,
// and concatenates them into a single YAML document.
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

// FileError is a reconcile failure for one module, naming the config file it is about.
type FileError struct {
	// Module is the config module whose reconcile failed.
	Module string
	// Path is the absolute path of the file that failed to read, parse, write or remove.
	Path string
	// Err is the underlying failure.
	Err error
}

// Error renders the path followed by the underlying failure.
func (e *FileError) Error() string {
	return e.Path + ": " + e.Err.Error()
}

// Unwrap returns the underlying failure.
func (e *FileError) Unwrap() error {
	return e.Err
}

// leafValues flattens a decoded YAML document into dotted key path to leaf value.
// Maps are descended, an empty map contributes nothing, and a list is one leaf compared whole.
func leafValues(prefix string, node any, into map[string]any) {
	mapping, isMap := node.(map[string]any)
	if !isMap {
		into[prefix] = node
		return
	}
	for key, child := range mapping {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		leafValues(path, child, into)
	}
}

// decodeLeafValues reads a YAML file into its leaf values.
func decodeLeafValues(data []byte) (map[string]any, error) {
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	leaves := map[string]any{}
	for key, child := range document {
		leafValues(key, child, leaves)
	}
	return leaves, nil
}

// retireHubWideCopy decides the outcome of a hub-wide module's per-worktree copy against the hub file.
// A missing copy or a missing hub file yields an empty result and leaves the copy alone.
// A copy whose every leaf is present in the hub file at the same key path with an equal value is Retired, and apply deletes it.
// Any other copy is Divergent, listing each leaf the hub file lacks or holds differently, and stays.
func retireHubWideCopy(module, baseDir, boardDir string, apply bool) (Result, error) {
	result := Result{Module: module}

	copyPath := configengine.ConfigFile(baseDir, module)
	copyData, err := os.ReadFile(copyPath)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return Result{}, &FileError{Module: module, Path: copyPath, Err: fmt.Errorf("read config for %s: %w", module, err)}
	}

	hubPath := configengine.ConfigFile(boardDir, module)
	hubData, err := os.ReadFile(hubPath)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return Result{}, &FileError{Module: module, Path: hubPath, Err: fmt.Errorf("read hub config for %s: %w", module, err)}
	}

	copyLeaves, err := decodeLeafValues(copyData)
	if err != nil {
		return Result{}, &FileError{Module: module, Path: copyPath, Err: err}
	}
	hubLeaves, err := decodeLeafValues(hubData)
	if err != nil {
		return Result{}, &FileError{Module: module, Path: hubPath, Err: err}
	}

	for path, value := range copyLeaves {
		if hubValue, present := hubLeaves[path]; !present || !reflect.DeepEqual(hubValue, value) {
			result.Divergent = append(result.Divergent, fmt.Sprintf("%s: %v", path, value))
		}
	}
	sort.Strings(result.Divergent)
	if len(result.Divergent) > 0 {
		return result, nil
	}

	result.Retired = true
	if apply {
		if err := os.Remove(copyPath); err != nil && !os.IsNotExist(err) {
			return Result{}, &FileError{Module: module, Path: copyPath, Err: fmt.Errorf("remove retired config for %s: %w", module, err)}
		}
		result.Applied = true
	}
	return result, nil
}

// ReconcileAll reconciles the per-worktree module config files under baseDir against their templates,
// returning the slice of results and any I/O or YAML parsing error.
// Seed-only modules (e.g. "models") with present files are reported untouched;
// absent files materialize the template verbatim.
// A hub-wide module is never reconciled or written under baseDir.
// Its result reports whether a leftover per-worktree copy is Retired (the hub file at boardDir holds everything the copy holds, and apply deletes it)
// or Divergent (the copy holds something the hub file lacks, and it stays).
// When apply is false, files are never written or removed.
func ReconcileAll(baseDir, boardDir string, apply bool) ([]Result, error) {
	var results []Result

	for _, m := range configreg.Modules() {
		if m.HubWide {
			result, err := retireHubWideCopy(m.Name, baseDir, boardDir, apply)
			if err != nil {
				return nil, err
			}
			results = append(results, result)
			continue
		}

		cfgPath := configengine.ConfigFile(baseDir, m.Name)

		existing, err := os.ReadFile(cfgPath)
		fileAbsent := os.IsNotExist(err)
		if err != nil && !os.IsNotExist(err) {
			return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("read config for %s: %w", m.Name, err)}
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
				return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("reconcile %s: %w", m.Name, err)}
			}

			result := Result{Module: m.Name, Added: added, Applied: false}
			if apply {
				if err := fsx.AtomicWriteBytes(cfgPath, []byte(m.Template())); err != nil {
					return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("write config for %s: %w", m.Name, err)}
				}
				result.Applied = true
			}
			results = append(results, result)
			continue
		}

		var migrated []string
		if m.Migrate != nil && !fileAbsent {
			existing, migrated, err = m.Migrate(existing)
			if err != nil {
				return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("migrate %s: %w", m.Name, err)}
			}
		}

		merged, added, removed, err := yamlengine.Reconcile([]byte(m.Template()), existing, m.OpenMaps...)
		if err != nil {
			return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("reconcile %s: %w", m.Name, err)}
		}

		result := Result{
			Module:       m.Name,
			Added:        added,
			Removed:      removed,
			Applied:      false,
			MigratedFrom: migratedFrom,
			Migrated:     migrated,
		}

		hasChanges := len(added)+len(removed)+len(migrated) > 0

		if apply && (fileAbsent || hasChanges) {
			if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
				return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("write config for %s: %w", m.Name, err)}
			}
			result.Applied = true
			for _, legacy := range migratedFrom {
				legacyPath := configengine.ConfigFile(baseDir, legacy)
				if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
					return nil, &FileError{Module: m.Name, Path: legacyPath, Err: fmt.Errorf("remove migrated legacy config %s: %w", legacy, err)}
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

// ReconcileHubWideAt reconciles every hub-wide module's file at configengine.ConfigFile(boardDir, name),
// in registry order, and returns one Result per module.
// primeBaseDir is the prime worktree's anchor dir; empty means no prime is resolvable.
//
// A present hub file is the reconcile input and is never replaced.
// An absent file of a module with open maps, or one marked SeedsFromPrime, is seeded from the prime's copy when that copy exists and parses;
// any other module never reads the prime, since its per-worktree copy was never in effect.
// Otherwise an absent file folds in the module's pre-cutover legacy files, and failing that starts from the template.
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
			return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("read config for %s: %w", m.Name, err)}
		}

		seed := SeedHub
		var migratedFrom []string
		if fileAbsent {
			existing = nil
			seed = SeedTemplate
			if len(m.OpenMaps) > 0 || m.SeedsFromPrime {
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
			return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("reconcile %s: %w", m.Name, err)}
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
				return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("mkdir config dir for %s: %w", m.Name, err)}
			}
			if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
				return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("write config for %s: %w", m.Name, err)}
			}
			result.Applied = true

			for _, legacy := range migratedFrom {
				legacyPath := configengine.ConfigFile(boardDir, legacy)
				if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
					return nil, &FileError{Module: m.Name, Path: legacyPath, Err: fmt.Errorf("remove migrated legacy config %s: %w", legacy, err)}
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}
