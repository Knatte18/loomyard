// configsync.go implements reconciliation of all module configs against their templates.
//
// It provides atomic writes and per-module reconciliation via yamlengine.Reconcile, tracking
// added/removed keys and applying changes when requested.
// The base layer of a config stack stays full and every layer above it is sparse.
// ReconcileHubWideAt covers every module's file at the board dir, hub files and per-worktree repo defaults;
// ReconcileAll covers a worktree's sparse and local layers.
// Every reconcile entry point, and run-settings preparation through configreg, applies the key-move table to each layer as its first step, before any module of the layer is seeded or reconciled.
// Reconcile failures are *FileError values naming the file that failed.

package configsync

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"slices"
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
// A key-move result instead reports one file the move changed, and a module may appear in a move result and in its own reconcile result.
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
	// Layer names the layer the result reports: the repo default or hub file, the worktree file, or the local override.
	Layer configengine.Layer
	// Stripped lists, sorted, the dotted keys of a worktree file that equal what the layers below resolve to, and are therefore redundant in a sparse layer.
	// Reported as pending unless the run migrates.
	// Set only by ReconcileAll, on a worktree that carries no sparse marker.
	Stripped []string
	// Inherited lists, sorted, the dotted keys the repo default holds that the worktree file lacked and whose value differs from the template's or that the template lacks.
	// After the strip the worktree resolves each of them to the default's value, so the operator can pin the old value with lyx config.
	// Set only by ReconcileAll, together with Stripped.
	Inherited []string
	// Moved lists, sorted, the declared key moves that changed this file, each as "<source module>.<source key> -> <target module>.<target key>".
	// Set only by a key move, on a dry run too.
	Moved []string
	// KeptTarget lists, sorted, each move whose source value was dropped because the target already held its own, as "<source>: <value> dropped, <target>: <value> kept".
	// Set only by a key move, on a dry run too.
	KeptTarget []string
	// Overwritten lists, sorted, each move whose source value replaced the target's template default, as "<source>: <value> replaced <target>'s template default <value>".
	// Set only by a key move, on a dry run too.
	Overwritten []string
}

// Options configures ReconcileAll.
type Options struct {
	// Apply writes and removes files; false reports what would change.
	Apply bool
	// Migrate lets an Apply run strip a worktree's full files to sparse overrides and write the sparse marker; without it Stripped and Inherited are only reported as pending.
	Migrate bool
}

// HubWideOptions configures ReconcileHubWideAt.
type HubWideOptions struct {
	// Apply writes and removes files; false reports what would change.
	Apply bool
	// SeedDefaults seeds a per-worktree module's absent repo default; false leaves an absent default absent.
	SeedDefaults bool
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
// A missing copy, a missing hub file or an empty boardDir yields an empty result and leaves the copy alone.
// A copy whose every leaf is present in the hub file at the same key path with an equal value is Retired, and apply deletes it.
// Any other copy is Divergent, listing each leaf the hub file lacks or holds differently, and stays.
func retireHubWideCopy(module, baseDir, boardDir string, apply bool) (Result, error) {
	result := Result{Module: module, Layer: configengine.LayerWorktree}
	if boardDir == "" {
		return result, nil
	}

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

// ReconcileAll reconciles a worktree's layers under baseDir against the module templates,
// returning the slice of results and any I/O or YAML parsing error.
// Standalone (boardDir empty) the worktree file is the base layer and is fully reconciled;
// in a hub it is sparse: a present file is migrated and its retired keys pruned, nothing is added, and an absent one stays absent.
// The local file of every module is a sparse layer reconciled the same way, and is reported with layer local only when present.
// A seed-only module's worktree and local files are never rewritten; standalone, an absent worktree file materializes the template verbatim.
// A hub-wide module is never reconciled or written as a worktree file.
// Its result reports whether a leftover per-worktree copy is Retired (the hub file at boardDir holds everything the copy holds, and apply deletes it)
// or Divergent (the copy holds something the hub file lacks, and it stays).
// A file emptied by a prune or a strip is deleted.
// In a hub, a worktree without the sparse marker has each per-worktree module's worktree file compared with the layers below it, and the keys that add nothing are reported as Stripped, with the keys the repo default adds as Inherited;
// the comparison runs only when every per-worktree module's repo default exists.
// Only with opts.Migrate and opts.Apply are the files stripped and the marker written, after them.
// The declared key moves run first, at the worktree layer and then the local layer, and their results lead the returned slice.
// When opts.Apply is false, files are never written or removed.
func ReconcileAll(baseDir, boardDir string, opts Options) ([]Result, error) {
	results, err := moveKeys(baseDir, "", configengine.LayerWorktree, boardDir == "", opts.Apply)
	if err != nil {
		return nil, err
	}
	localMoves, err := moveKeys(baseDir, "", configengine.LayerLocal, false, opts.Apply)
	if err != nil {
		return nil, err
	}
	results = append(results, localMoves...)

	pending := pendingMigration(baseDir, boardDir)
	if pending != nil {
		pending.write = opts.Migrate && opts.Apply
	}
	for _, m := range configreg.Modules() {
		var result Result
		var err error
		switch {
		case m.HubWide:
			result, err = retireHubWideCopy(m.Name, baseDir, boardDir, opts.Apply)
		case boardDir == "":
			result, err = reconcileBaseWorktreeFile(m, baseDir, opts.Apply)
		default:
			result, _, err = reconcileSparseFile(m, configengine.ConfigFile(baseDir, m.Name), configengine.LayerWorktree, opts.Apply, pending)
		}
		if err != nil {
			return nil, err
		}
		results = append(results, result)

		localResult, present, err := reconcileSparseFile(m, configengine.LocalFile(baseDir, m.Name), configengine.LayerLocal, opts.Apply, nil)
		if err != nil {
			return nil, err
		}
		if present {
			results = append(results, localResult)
		}
	}

	if pending != nil && pending.write {
		if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir config dir for the sparse marker: %w", err)
		}
		marker := configengine.SparseMarker(baseDir)
		if err := fsx.AtomicWriteBytes(marker, []byte("# This worktree's config files are sparse layers over the repo defaults.\n")); err != nil {
			return nil, &FileError{Module: "sparse", Path: marker, Err: fmt.Errorf("write the sparse marker: %w", err)}
		}
	}

	return results, nil
}

// migration describes the strip of a worktree's full files to sparse overrides.
type migration struct {
	dirs configengine.Dirs
	// write makes the strip rewrite the files; false reports it as pending.
	write bool
}

// pendingMigration returns the strip a worktree still owes, or nil when none is due:
// standalone, with the sparse marker present, or while any per-worktree module's repo default is absent under boardDir.
func pendingMigration(baseDir, boardDir string) *migration {
	if boardDir == "" {
		return nil
	}
	if _, err := os.Stat(configengine.SparseMarker(baseDir)); err == nil {
		return nil
	}
	for _, m := range configreg.Modules() {
		if m.HubWide {
			continue
		}
		if _, err := os.Stat(configengine.ConfigFile(boardDir, m.Name)); err != nil {
			return nil
		}
	}
	return &migration{dirs: configengine.Dirs{Anchor: baseDir, Board: boardDir}}
}

// reconcileBaseWorktreeFile fully reconciles a per-worktree module's worktree file, the base layer of a standalone stack.
// A seed-only module's present file is reported untouched and an absent one materializes the template verbatim.
func reconcileBaseWorktreeFile(m configreg.Module, baseDir string, apply bool) (Result, error) {
	cfgPath := configengine.ConfigFile(baseDir, m.Name)

	existing, err := os.ReadFile(cfgPath)
	fileAbsent := os.IsNotExist(err)
	if err != nil && !fileAbsent {
		return Result{}, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("read config for %s: %w", m.Name, err)}
	}
	if fileAbsent {
		existing = []byte{}
	}

	if m.SeedOnly {
		if !fileAbsent {
			return Result{Module: m.Name, Layer: configengine.LayerWorktree}, nil
		}
		added, err := yamlengine.MissingKeys([]byte(m.Template()), nil)
		if err != nil {
			return Result{}, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("reconcile %s: %w", m.Name, err)}
		}

		result := Result{Module: m.Name, Added: added, Layer: configengine.LayerWorktree}
		if apply {
			if err := fsx.AtomicWriteBytes(cfgPath, []byte(m.Template())); err != nil {
				return Result{}, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("write config for %s: %w", m.Name, err)}
			}
			result.Applied = true
		}
		return result, nil
	}

	var migrated []string
	if m.Migrate != nil && !fileAbsent {
		existing, migrated, err = m.Migrate(existing)
		if err != nil {
			return Result{}, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("migrate %s: %w", m.Name, err)}
		}
	}

	merged, added, removed, err := yamlengine.Reconcile([]byte(m.Template()), existing, m.OpenMaps...)
	if err != nil {
		return Result{}, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("reconcile %s: %w", m.Name, err)}
	}

	result := Result{
		Module:   m.Name,
		Added:    added,
		Removed:  removed,
		Migrated: migrated,
		Layer:    configengine.LayerWorktree,
	}
	if apply && (fileAbsent || len(added)+len(removed)+len(migrated) > 0) {
		if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
			return Result{}, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("write config for %s: %w", m.Name, err)}
		}
		result.Applied = true
	}
	return result, nil
}

// reconcileSparseFile reconciles one sparse layer file of a module: its Migrate hook runs over it and its retired keys are pruned, and nothing is ever added.
// An absent file stays absent and reports present false;
// a seed-only module's present file is otherwise untouched.
// With a non-nil strip, the file's keys that equal what the layers below it resolve to are reported as Stripped, and removed when strip.write is set;
// a seed-only module takes the strip as well.
// A file emptied by a prune or a strip is deleted and reported applied.
func reconcileSparseFile(m configreg.Module, path string, layer configengine.Layer, apply bool, strip *migration) (result Result, present bool, err error) {
	result = Result{Module: m.Name, Layer: layer}
	original, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, false, nil
	}
	if err != nil {
		return Result{}, false, &FileError{Module: m.Name, Path: path, Err: fmt.Errorf("read config for %s: %w", m.Name, err)}
	}
	fail := func(verb string, cause error) (Result, bool, error) {
		return Result{}, true, &FileError{Module: m.Name, Path: path, Err: fmt.Errorf("%s %s: %w", verb, m.Name, cause)}
	}

	current := original
	if !m.SeedOnly {
		if m.Migrate != nil {
			current, result.Migrated, err = m.Migrate(current)
			if err != nil {
				return fail("migrate", err)
			}
		}
		current, result.Removed, err = yamlengine.Prune([]byte(m.Template()), current, m.OpenMaps...)
		if err != nil {
			return fail("reconcile", err)
		}
	}
	changed := len(result.Removed)+len(result.Migrated) > 0

	if strip != nil {
		stripped, strippedKeys, inherited, err := stripToSparse(m, strip.dirs, original, current)
		if err != nil {
			return fail("strip", err)
		}
		result.Stripped, result.Inherited = strippedKeys, inherited
		if strip.write && len(strippedKeys) > 0 {
			current = stripped
			changed = true
		}
	}

	if !apply || !changed {
		return result, true, nil
	}
	if len(bytes.TrimSpace(current)) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return Result{}, true, &FileError{Module: m.Name, Path: path, Err: fmt.Errorf("remove emptied config for %s: %w", m.Name, err)}
		}
	} else if err := fsx.AtomicWriteBytes(path, current); err != nil {
		return Result{}, true, &FileError{Module: m.Name, Path: path, Err: fmt.Errorf("write config for %s: %w", m.Name, err)}
	}
	result.Applied = true
	return result, true, nil
}

// stripToSparse strips current, a worktree file of m, of every key that equals what the layers below the worktree layer resolve to, and returns the stripped bytes with the removed keys.
// inherited lists the keys the repo default holds that original, the file as read, lacked and whose value differs from the template's or that the template lacks.
func stripToSparse(m configreg.Module, dirs configengine.Dirs, original, current []byte) (stripped []byte, strippedKeys, inherited []string, err error) {
	spec := m.Spec()
	below, err := configengine.BelowLayer(dirs, spec, configengine.LayerWorktree)
	if err != nil {
		return nil, nil, nil, err
	}
	stripped, strippedKeys, err = yamlengine.Strip(below, current, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, nil, nil, err
	}

	layers, err := configengine.Layers(dirs, spec)
	if err != nil {
		return nil, nil, nil, err
	}
	var repoDefault []byte
	for _, layer := range layers {
		if layer.Layer == configengine.LayerDefault {
			repoDefault = layer.Bytes
		}
	}
	differing, _, err := yamlengine.Strip(spec.Template, repoDefault, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, nil, nil, err
	}
	differingKeys, err := yamlengine.LeafPaths(differing, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, nil, nil, err
	}
	held, err := yamlengine.LeafPaths(original, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, key := range differingKeys {
		if !slices.Contains(held, key) {
			inherited = append(inherited, key)
		}
	}
	return stripped, strippedKeys, inherited, nil
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

// ReconcileHubWideAt reconciles every module's file at configengine.ConfigFile(boardDir, name),
// in registry order, and returns one Result per module with Layer default:
// a hub-wide module's hub file and a per-worktree module's repo default, which is that module's full base layer.
// primeBaseDir is the prime worktree's anchor dir; empty means no prime is resolvable.
//
// A present file is the reconcile input and is never replaced; a present seed-only default is never rewritten.
// An absent hub file of a module with open maps, or one marked SeedsFromPrime, is seeded from the prime's copy when that copy exists and parses;
// any other hub-wide module never reads the prime, since its per-worktree copy was never in effect.
// Otherwise an absent hub file folds in the module's pre-cutover legacy files, and failing that starts from the template.
// An absent per-worktree default is seeded only under opts.SeedDefaults, from the prime's copy when it exists and parses, else from the template;
// without it the default stays absent.
// Legacy files are pruned only when opts.Apply succeeds.
// The declared key moves run first at the board dir as a base layer, and their results lead the returned slice;
// under opts.SeedDefaults an absent board source is the prime's copy, so a default seeded in the same pass carries the moved values.
// When opts.Apply is false, nothing is written or removed.
func ReconcileHubWideAt(boardDir, primeBaseDir string, opts HubWideOptions) ([]Result, error) {
	seedDir := ""
	if opts.SeedDefaults {
		seedDir = primeBaseDir
	}
	results, err := moveKeys(boardDir, seedDir, configengine.LayerDefault, true, opts.Apply)
	if err != nil {
		return nil, err
	}

	for _, m := range configreg.Modules() {
		cfgPath := configengine.ConfigFile(boardDir, m.Name)

		existing, err := os.ReadFile(cfgPath)
		fileAbsent := os.IsNotExist(err)
		if err != nil && !fileAbsent {
			return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("read config for %s: %w", m.Name, err)}
		}

		presentSeedOnly := !fileAbsent && m.SeedOnly
		absentUnseeded := fileAbsent && !opts.SeedDefaults
		if !m.HubWide && (presentSeedOnly || absentUnseeded) {
			untouched := Result{Module: m.Name, Layer: configengine.LayerDefault}
			if presentSeedOnly {
				untouched.Seed = SeedHub
			}
			results = append(results, untouched)
			continue
		}

		seed := SeedHub
		var migratedFrom []string
		var migrated []string
		if fileAbsent {
			existing = nil
			seed = SeedTemplate
			if len(m.OpenMaps) > 0 || m.SeedsFromPrime || !m.HubWide {
				if prime := primeConfig(primeBaseDir, m.Name); prime != nil {
					existing = prime
					seed = SeedPrime
				}
			}
			if seed == SeedTemplate && m.HubWide {
				if legacy, from := legacyConfig(m.Name, boardDir); len(from) > 0 {
					existing = legacy
					migratedFrom = from
					seed = SeedLegacy
				}
			}
		}

		var added, removed []string
		var mergedBytes []byte
		var reconcileErr error
		if m.SeedOnly {
			// An absent seed-only default is the prime's copy or the template, verbatim.
			mergedBytes = existing
			if mergedBytes == nil {
				mergedBytes = []byte(m.Template())
				added, reconcileErr = yamlengine.MissingKeys(mergedBytes, nil)
			}
		} else {
			if m.Migrate != nil && !fileAbsent {
				existing, migrated, err = m.Migrate(existing)
				if err != nil {
					return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("migrate %s: %w", m.Name, err)}
				}
			}
			mergedBytes, added, removed, reconcileErr = yamlengine.Reconcile([]byte(m.Template()), existing, m.OpenMaps...)
		}
		if reconcileErr != nil {
			return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("reconcile %s: %w", m.Name, reconcileErr)}
		}

		result := Result{
			Module:       m.Name,
			Added:        added,
			Removed:      removed,
			MigratedFrom: migratedFrom,
			Migrated:     migrated,
			Seed:         seed,
			Layer:        configengine.LayerDefault,
		}

		if opts.Apply && (fileAbsent || len(added)+len(removed)+len(migrated) > 0) {
			if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
				return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("mkdir config dir for %s: %w", m.Name, err)}
			}
			if err := fsx.AtomicWriteBytes(cfgPath, mergedBytes); err != nil {
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
