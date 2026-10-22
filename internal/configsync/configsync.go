// configsync.go implements reconciliation of all module configs against their templates.
//
// It provides atomic writes and per-module reconciliation via yamlengine.Reconcile, tracking
// added/removed keys and applying changes when requested.
// In a hub every non-seed-only base file is sparse over the template, seeded sparse and pruned but never filled, and stripped once by the walk behind the board marker;
// only a standalone worktree base and a seed-only file stay full, and every layer above a base is sparse.
// ReconcileHubWideAt covers every module's file at the board dir, hub files and per-worktree repo defaults;
// ReconcileAll covers a worktree's sparse and local layers.
// Every strip, prune, reconcile and template-equal drop judges a file against the effective template its base layer picks, the module's template with the overlay its selector names laid over it;
// key-move scaffolds and the seed-only paths use the base template.
// Every reconcile entry point, and run-settings preparation through configreg, applies the key-move table to each layer as its first step, before any module of the layer is seeded or reconciled.
// Reconcile failures are *FileError values naming the file that failed.

package configsync

import (
	"bytes"
	"errors"
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

// formerTemplateValues maps a module name to a top-level key to the template value its replaced units held immediately before the tuned defaults replaced them, as YAML text.
// An open map is one unit, so a whole map is compared whole; under any other mapping, a per-name map included, each leaf unit is compared alone.
// Only the replaced units are listed; an older template generation's value is kept as tuned.
var formerTemplateValues = map[string]map[string]string{
	"loom": formerLoomValues(),
	"shed": {"driver_kind": "llm"},
	"darn": {
		"darn_reviewer_model": "sonnet[high]",
		"darn_fixer_model":    `""`,
		"darn_reviewer_fan":   `""`,
		"darn_fixer_fan":      `""`,
	},
	"claude": {"prompt_cache_ttl_roles": "{driver: 1h, webster: 1h}"},
	"orch": {
		"soft_threshold_tokens": "300000",
		"threshold_tokens":      "400000",
	},
	"batcher": {"profiles": "cautious:\n  budget: 450000\n"},
}

// formerLoomValues returns loom's former template values.
func formerLoomValues() map[string]string {
	former := map[string]string{
		"webster_reviewer_model": "sonnet[high]",
		"merge_reviewer_model":   "sonnet[high]",
		"driver":                 "sonnet[medium]",
		"discussion_max_bounces": "~",
		"webster_max_bounces":    "~",
		"webster_auto_accept":    "false",
	}
	for _, unit := range []string{
		"discussion_lenses", "plan_lenses",
		"discussion_reviewer_model", "plan_reviewer_model", "webster_fixer_model", "merge_fixer_model",
	} {
		former[unit] = `""`
	}
	for _, segment := range []string{"discussion", "plan", "webster", "merge"} {
		former[segment+"_reviewer_fan"] = `""`
		former[segment+"_fixer_fan"] = `""`
	}
	return former
}

// formerValuesDoc renders a module's former template values as one YAML document over its top-level keys, empty for a module with none.
func formerValuesDoc(module string) []byte {
	units := formerTemplateValues[module]
	if len(units) == 0 {
		return nil
	}
	names := make([]string, 0, len(units))
	for name := range units {
		names = append(names, name)
	}
	sort.Strings(names)

	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range names {
		var value yaml.Node
		if err := yaml.Unmarshal([]byte(units[name]), &value); err != nil {
			panic(fmt.Sprintf("configsync: former value of %s.%s is not YAML: %v", module, name, err))
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, value.Content[0])
	}
	doc, err := yaml.Marshal(root)
	if err != nil {
		panic(fmt.Sprintf("configsync: render the former values of %s: %v", module, err))
	}
	return doc
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
	// A hub base seed copies no template key, so it reports none; only a full base file and a seed-only seed do.
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
	// A seed of a hub base file holds only the units that differ from the template.
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
	// Stripped lists, sorted, the dotted keys of a worktree file that equal what the layers below resolve to, and are therefore redundant in a sparse layer, and the keys equal to a former template value.
	// Reported as pending unless the run migrates.
	// ReconcileAll sets it on a worktree that carries no sparse marker;
	// ReconcileHubWideAt sets it on a board that carries no base-sparse marker, for the keys equal to the template or a former template value.
	Stripped []string
	// Inherited lists, sorted, the dotted keys the repo default holds that the worktree file lacked and whose value differs from the template's or that the template lacks.
	// After the strip the worktree resolves each of them to the default's value, so the operator can pin the old value with lyx config.
	// Set only by ReconcileAll, together with Stripped.
	Inherited []string
	// Moved lists, sorted, the declared key moves that changed this file, each as "<source module>.<source key> -> <target module>.<target key>".
	// Set only by a key move, on a dry run too.
	Moved []string
	// KeptTarget lists, sorted, each move whose source value was dropped because the target already held its own, as "<source>: <value> dropped, <target>: <value> kept".
	// A move that keeps whatever the target holds renders "<source> dropped, <target> kept", without the values.
	// Set only by a key move, on a dry run too.
	KeptTarget []string
	// Overwritten lists, sorted, each move whose source value replaced the target's template default, as "<source>: <value> replaced <target>'s template default <value>".
	// Set only by a key move, on a dry run too.
	Overwritten []string
	// KeptExtra lists, sorted, each "<path>.<name>" a per-name map of the file holds and the template lacks.
	// Such a name is kept in place and never pruned.
	// Set on a dry run too.
	KeptExtra []string
	// Dropped lists, sorted, each "<path>.<name>" a per-name map of a base-layer file held with the template's value, left out of the file because the load resolves it from the template.
	// Set on a dry run too.
	Dropped []string
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
	// StripBase lets an Apply run strip a present base file of the units equal to the template or a former template value, and write the board marker after the files.
	// Only hubreconcile's walk sets it; without it the strip is only reported as pending.
	StripBase bool
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

// effectiveTemplateOf composes the template m's base document picks, failing as a *FileError that names path, the file holding the selector value.
func effectiveTemplateOf(m configreg.Module, base []byte, hub bool, path string) ([]byte, error) {
	template, err := configengine.EffectiveTemplate(m.Spec(), base, hub)
	if err != nil {
		return nil, &FileError{Module: m.Name, Path: path, Err: err}
	}
	return template, nil
}

// templateUnderBase composes the template the base layer's file under dirs picks, failing as a *FileError that names that file.
func templateUnderBase(m configreg.Module, dirs configengine.Dirs) ([]byte, error) {
	spec := m.Spec()
	template, err := configengine.TemplateUnder(dirs, spec)
	if err != nil {
		basePath, _ := configengine.LayerFile(dirs, spec, configengine.BaseLayer(dirs, spec))
		return nil, &FileError{Module: m.Name, Path: basePath, Err: err}
	}
	return template, nil
}

// primeSeedError names the prime's copy of m in a seed failure its selector caused, and adds the step the refusal's way forward lacks:
// a prime worktree file holding a selector the base does not hold is refused at load, so its line must go too.
// The delete step is added only for a module with an overlay selector.
// It returns err unchanged for any other failure.
func primeSeedError(m configreg.Module, primeBaseDir string, err error) error {
	if !errors.Is(err, configengine.ErrInvalid) {
		return err
	}
	primePath := configengine.ConfigFile(primeBaseDir, m.Name)
	selector := m.Spec().Overlay.Selector
	if selector == "" {
		return &FileError{Module: m.Name, Path: primePath, Err: err}
	}
	return &FileError{Module: m.Name, Path: primePath, Err: fmt.Errorf("%w; also delete the %s line from %s in an editor", err, selector, primePath)}
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

// retireHubWideCopy decides the outcome of a hub-wide module's per-worktree copy against what the hub resolves to: the template merged under the sparse hub file.
// A missing copy, a missing hub file or an empty boardDir yields an empty result and leaves the copy alone.
// A copy whose every leaf is present in that merge at the same key path with an equal value is Retired, and apply deletes it.
// Any other copy is Divergent, listing each leaf the merge lacks or holds differently, and stays.
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
	if _, err := os.Stat(hubPath); os.IsNotExist(err) {
		return result, nil
	} else if err != nil {
		return Result{}, &FileError{Module: module, Path: hubPath, Err: fmt.Errorf("read hub config for %s: %w", module, err)}
	}
	registered, ok := configreg.Lookup(module)
	if !ok {
		return Result{}, &FileError{Module: module, Path: hubPath, Err: fmt.Errorf("module %q is not registered", module)}
	}
	hubData, err := configengine.BelowLayer(configengine.Dirs{Board: boardDir}, registered.Spec(), configengine.LayerWorktree)
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
	dirs := configengine.Dirs{Anchor: baseDir, Board: boardDir}
	for _, m := range configreg.Modules() {
		var result Result
		var err error
		switch {
		case m.HubWide:
			result, err = retireHubWideCopy(m.Name, baseDir, boardDir, opts.Apply)
		case boardDir == "":
			result, err = reconcileBaseWorktreeFile(m, baseDir, opts.Apply)
		default:
			result, _, err = reconcileSparseFile(m, dirs, configengine.ConfigFile(baseDir, m.Name), configengine.LayerWorktree, opts.Apply, pending)
		}
		if err != nil {
			return nil, err
		}
		results = append(results, result)

		localResult, present, err := reconcileSparseFile(m, dirs, configengine.LocalFile(baseDir, m.Name), configengine.LayerLocal, opts.Apply, nil)
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

	template, err := effectiveTemplateOf(m, existing, false, cfgPath)
	if err != nil {
		return Result{}, err
	}
	merged, added, removed, kept, dropped, err := yamlengine.Reconcile(template, existing, m.PerNameMaps, m.OpenMaps...)
	if err != nil {
		return Result{}, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("reconcile %s: %w", m.Name, err)}
	}

	result := Result{
		Module:    m.Name,
		Added:     added,
		Removed:   removed,
		KeptExtra: kept,
		Dropped:   dropped,
		Migrated:  migrated,
		Layer:     configengine.LayerWorktree,
	}
	if apply && (fileAbsent || len(added)+len(removed)+len(dropped)+len(migrated) > 0) {
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
func reconcileSparseFile(m configreg.Module, dirs configengine.Dirs, path string, layer configengine.Layer, apply bool, strip *migration) (result Result, present bool, err error) {
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
		template, err := templateUnderBase(m, dirs)
		if err != nil {
			return Result{}, true, err
		}
		current, result.Removed, result.KeptExtra, err = yamlengine.Prune(template, current, m.PerNameMaps, m.OpenMaps...)
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

// stripToSparse strips current, a worktree file of m, of every key that equals what the layers below the worktree layer resolve to or equals a former template value of m, and returns the stripped bytes with the removed keys.
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
	stripped, strippedKeys, err = stripFormerValues(m, stripped, strippedKeys)
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
	effective, err := configengine.EffectiveTemplate(spec, repoDefault, true)
	if err != nil {
		return nil, nil, nil, err
	}
	differing, _, err := yamlengine.Strip(effective, repoDefault, spec.OpenMaps, spec.WholeEntries)
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

// stripFormerValues removes from current every unit equal to a former template value of m, and returns the stripped bytes with removedKeys followed by the keys it removed, sorted.
func stripFormerValues(m configreg.Module, current []byte, removedKeys []string) ([]byte, []string, error) {
	former := formerValuesDoc(m.Name)
	if former == nil || len(bytes.TrimSpace(current)) == 0 {
		return current, removedKeys, nil
	}
	spec := m.Spec()
	stripped, formerKeys, err := yamlengine.Strip(former, current, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, nil, err
	}
	removedKeys = append(removedKeys, formerKeys...)
	sort.Strings(removedKeys)
	return stripped, removedKeys, nil
}

// stripBase removes from current, units of a base file of m, every unit equal to the effective template's or to a former template value of m, and returns the stripped bytes with the removed keys, sorted.
// base is the base document whose selector picks the effective template; it is current itself unless current is only part of the file.
// An open map is compared whole, and a base left with no unit comes back empty.
func stripBase(m configreg.Module, base, current []byte) ([]byte, []string, error) {
	spec := m.Spec()
	effective, err := configengine.EffectiveTemplate(spec, base, true)
	if err != nil {
		return nil, nil, err
	}
	stripped, removedKeys, err := yamlengine.Strip(effective, current, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, nil, err
	}
	return stripFormerValues(m, stripped, removedKeys)
}

// sparseSeed returns the sparse base file a seed of m becomes: the seed after the module's Migrate hook and a prune, holding only the units that differ from the template and, with dropFormer, from the module's former template values.
// A seed with no such unit becomes the header-only base.
func sparseSeed(m configreg.Module, seed []byte, dropFormer bool) ([]byte, error) {
	current := seed
	if m.Migrate != nil {
		var err error
		if current, _, err = m.Migrate(current); err != nil {
			return nil, fmt.Errorf("migrate %s: %w", m.Name, err)
		}
	}
	spec := m.Spec()
	effective, err := configengine.EffectiveTemplate(spec, current, true)
	if err != nil {
		return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
	}
	migrated := current
	current, _, _, err = yamlengine.Prune(effective, current, m.PerNameMaps, m.OpenMaps...)
	if err != nil {
		return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
	}
	if dropFormer {
		current, _, err = stripBase(m, migrated, current)
	} else {
		current, _, err = yamlengine.Strip(effective, current, spec.OpenMaps, spec.WholeEntries)
	}
	if err != nil {
		return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
	}
	if len(bytes.TrimSpace(current)) == 0 {
		return []byte(configengine.SparseBaseHeader), nil
	}
	return current, nil
}

// WriteBaseSparseMarker writes the tracked marker under boardDir that says the board's base files are sparse over the template.
// A failure is a *FileError naming the marker path with an empty Module, since the marker belongs to no config module.
func WriteBaseSparseMarker(boardDir string) error {
	marker := configengine.BaseSparseMarker(boardDir)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		return &FileError{Path: marker, Err: fmt.Errorf("mkdir config dir for the base-sparse marker: %w", err)}
	}
	if err := fsx.AtomicWriteBytes(marker, []byte("# This board's base config files are sparse over lyx's template.\n")); err != nil {
		return &FileError{Path: marker, Err: fmt.Errorf("write the base-sparse marker: %w", err)}
	}
	return nil
}

// splitTopLevel splits a document into the entries whose top-level key is in keys and the rest, each as bytes empty when it holds no entry.
func splitTopLevel(data []byte, keys []string) (picked, rest []byte, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, nil, fmt.Errorf("parse: %w", err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil, data, nil
	}
	pickedRoot := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	restRoot := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		side := restRoot
		if slices.Contains(keys, root.Content[i].Value) {
			side = pickedRoot
		}
		side.Content = append(side.Content, root.Content[i], root.Content[i+1])
	}
	marshal := func(root *yaml.Node) ([]byte, error) {
		if len(root.Content) == 0 {
			return []byte{}, nil
		}
		return yaml.Marshal(root)
	}
	if picked, err = marshal(pickedRoot); err != nil {
		return nil, nil, err
	}
	if rest, err = marshal(restRoot); err != nil {
		return nil, nil, err
	}
	return picked, rest, nil
}

// stripMovedUnits strips from current, a base file of m that a key move created, only the units of the top-level keys named by movedKeys, and returns the file with the removed keys.
// The units a prime seed put in the file are left, since the seed already dropped what a strip drops.
// The moved units are judged against the effective template the whole file's selector picks.
func stripMovedUnits(m configreg.Module, current []byte, movedKeys []string) ([]byte, []string, error) {
	picked, rest, err := splitTopLevel(current, movedKeys)
	if err != nil {
		return nil, nil, err
	}
	strippedPicked, removedKeys, err := stripBase(m, current, picked)
	if err != nil {
		return nil, nil, err
	}
	if len(removedKeys) == 0 {
		return current, nil, nil
	}
	spec := m.Spec()
	merged, err := yamlengine.Merge(rest, strippedPicked, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, nil, err
	}
	return merged, removedKeys, nil
}

// movedTargetKeys returns, per target module, the top-level keys the move results show moved into it.
func movedTargetKeys(moves []Result) map[string][]string {
	targets := map[string][]string{}
	for _, move := range moves {
		for _, line := range move.Moved {
			_, target, ok := strings.Cut(line, " -> ")
			if !ok {
				continue
			}
			module, key, ok := strings.Cut(target, ".")
			if ok && !slices.Contains(targets[module], key) {
				targets[module] = append(targets[module], key)
			}
		}
	}
	return targets
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
// A seed-only module is reconciled as a full file: a present one is never rewritten, and an absent one is seeded whole.
// Every other module's file is a sparse base over the template: a present file runs the module's Migrate hook and is pruned of keys the template lacks, and nothing is ever added.
// Its per-name entries equal to the template's are dropped on every pass.
// An absent file of a module with open maps, or one marked SeedsFromPrime, is seeded from the prime's copy when that copy exists and parses;
// any other hub-wide module never reads the prime, since its per-worktree copy was never in effect.
// Otherwise an absent hub file folds in the module's pre-cutover legacy files, and failing that starts as the header-only base.
// A seed holds only the units that differ from the template, and from the module's former template values while the prime carries no sparse marker.
// An absent per-worktree default is seeded only under opts.SeedDefaults, from the prime's copy when it exists and parses, else as the header-only base;
// without it the default stays absent.
// A file the prune leaves with no unit is written as the header-only base, never deleted.
// While the board's base-sparse marker is absent, each base file present when the pass began also reports the units equal to the template or a former template value as Stripped;
// with opts.StripBase and opts.Apply the pass removes them, and writes the marker once every registered module's file exists.
// Legacy files are pruned only when opts.Apply succeeds.
// The declared key moves run first at the board dir as a base layer, and their results lead the returned slice;
// under opts.SeedDefaults an absent board source is the prime's copy, so a default seeded in the same pass carries the moved values.
// When opts.Apply is false, nothing is written or removed.
func ReconcileHubWideAt(boardDir, primeBaseDir string, opts HubWideOptions) ([]Result, error) {
	seedDir := ""
	if opts.SeedDefaults {
		seedDir = primeBaseDir
	}
	_, markerErr := os.Stat(configengine.BaseSparseMarker(boardDir))
	unmarked := os.IsNotExist(markerErr)
	presentBefore := map[string]bool{}
	for _, m := range configreg.Modules() {
		if _, err := os.Stat(configengine.ConfigFile(boardDir, m.Name)); err == nil {
			presentBefore[m.Name] = true
		}
	}

	results, err := moveKeys(boardDir, seedDir, configengine.LayerDefault, true, opts.Apply)
	if err != nil {
		return nil, err
	}
	movedKeys := movedTargetKeys(results)

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

		var added, removed, keptExtra, dropped, stripped []string
		var mergedBytes []byte
		var reconcileErr error
		changed := fileAbsent
		switch {
		case m.SeedOnly:
			// An absent seed-only default is the prime's copy or the template, verbatim.
			mergedBytes = existing
			if mergedBytes == nil {
				mergedBytes = []byte(m.Template())
				added, reconcileErr = yamlengine.MissingKeys(mergedBytes, nil)
			}
		case fileAbsent:
			switch seed {
			case SeedPrime:
				_, markerStat := os.Stat(configengine.SparseMarker(primeBaseDir))
				mergedBytes, reconcileErr = sparseSeed(m, existing, os.IsNotExist(markerStat))
				if reconcileErr != nil {
					reconcileErr = primeSeedError(m, primeBaseDir, reconcileErr)
				}
			case SeedLegacy:
				mergedBytes, reconcileErr = sparseSeed(m, existing, false)
			default:
				mergedBytes = []byte(configengine.SparseBaseHeader)
			}
		default:
			mergedBytes = existing
			if m.Migrate != nil {
				mergedBytes, migrated, err = m.Migrate(mergedBytes)
				if err != nil {
					return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("migrate %s: %w", m.Name, err)}
				}
			}
			var template []byte
			template, reconcileErr = effectiveTemplateOf(m, mergedBytes, true, cfgPath)
			if reconcileErr == nil {
				mergedBytes, removed, keptExtra, reconcileErr = yamlengine.Prune(template, mergedBytes, m.PerNameMaps, m.OpenMaps...)
			}
			changed = len(removed)+len(migrated) > 0
			// The strip runs before the per-name drop, so a per-name map the strip empties leaves no empty mapping behind.
			if reconcileErr == nil && unmarked {
				var strippedBytes []byte
				switch {
				case presentBefore[m.Name]:
					strippedBytes, stripped, reconcileErr = stripBase(m, mergedBytes, mergedBytes)
				case len(movedKeys[m.Name]) > 0:
					strippedBytes, stripped, reconcileErr = stripMovedUnits(m, mergedBytes, movedKeys[m.Name])
				}
				if opts.StripBase && opts.Apply && len(stripped) > 0 {
					mergedBytes = strippedBytes
					changed = true
				}
			}
			if reconcileErr == nil && len(m.PerNameMaps) > 0 {
				mergedBytes, dropped, reconcileErr = yamlengine.DropTemplateEqualNames(template, mergedBytes, m.PerNameMaps)
				changed = changed || len(dropped) > 0
			}
		}
		if reconcileErr != nil {
			var fileErr *FileError
			if errors.As(reconcileErr, &fileErr) {
				return nil, fileErr
			}
			return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("reconcile %s: %w", m.Name, reconcileErr)}
		}

		result := Result{
			Module:       m.Name,
			Added:        added,
			Removed:      removed,
			KeptExtra:    keptExtra,
			Dropped:      dropped,
			Stripped:     stripped,
			MigratedFrom: migratedFrom,
			Migrated:     migrated,
			Seed:         seed,
			Layer:        configengine.LayerDefault,
		}

		if opts.Apply && changed {
			if !m.SeedOnly && len(bytes.TrimSpace(mergedBytes)) == 0 {
				mergedBytes = []byte(configengine.SparseBaseHeader)
			}
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

	// The marker follows the files, so an interrupted pass leaves the board unmarked and the next walk strips again.
	if opts.StripBase && opts.Apply && unmarked && everyBaseFilePresent(boardDir) {
		if err := WriteBaseSparseMarker(boardDir); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// everyBaseFilePresent reports whether every registered module has its file at the board dir.
func everyBaseFilePresent(boardDir string) bool {
	for _, m := range configreg.Modules() {
		if _, err := os.Stat(configengine.ConfigFile(boardDir, m.Name)); err != nil {
			return false
		}
	}
	return true
}
