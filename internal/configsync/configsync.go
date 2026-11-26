// configsync.go implements reconciliation of all module configs against their templates.
//
// It provides atomic writes and per-module reconciliation via yamlengine.Reconcile, tracking
// added/removed keys and applying changes when requested.
// In a hub, every non-seed-only base file is sparse over the template.
// It is seeded sparse and pruned but never filled.
// Only a standalone worktree base and a seed-only file stay full, and every layer above a base is sparse.
// A seed-only board file follows its template against the template base kept beside it.
// It keeps every value the operator set.
// ReconcileHubWideAt covers every module's file at the board dir, hub files and per-worktree repo defaults;
// ReconcileAll covers a worktree's sparse and local layers.
// A key the template lacks is pruned only under the options' PruneUnknown;
// any other run on a hub base or worktree file keeps it and reports it as Preserved,
// so a binary older than the one that wrote a key never strips it.
// A retired key is such a key like any other.
// An old key a module's move table names is moved to its new path before any merge, prune or strip.
// That holds for every file a reconcile reads, bar a seed-only module's.
// Every prune, reconcile and template-equal drop judges a file against its base layer's effective template.
// That is the module's template with the overlay its selector names laid over it.
// Reconcile failures are *FileError values naming the file that failed.

package configsync

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// Seed values name which input a reconcile started from.
const (
	// SeedHub is a hub file already present at the board dir.
	SeedHub = "hub"
	// SeedPrime is the prime worktree's copy, read only when the hub file is absent.
	SeedPrime = "prime"
	// SeedTemplate is the module's embedded template.
	SeedTemplate = "template"
)

// Result represents the reconciliation result for a single config module.
type Result struct {
	// Module is the name of the config module (e.g., "board", "worktree", "fabric").
	Module string
	// Added is the slice of key-paths newly discovered in the template.
	// A hub base seed copies no template key, so it reports none;
	// only a full base file, a seed-only seed and a seed-only board file following its template do.
	Added []string
	// Removed is the slice of key-paths that existed in the file but not in the template.
	// For a seed-only board file it lists the units the template dropped while the file held its template base's value.
	Removed []string
	// Updated lists, as "<dotted.key>: <old> -> <new>", each seed-only board unit still at its template base's value.
	// Set only by ReconcileHubWideAt, on a dry run too; the template value has changed.
	Updated []string
	// Differs lists, as "<dotted.key>: file <value>, template <value>", each operator-set unit where the template differs.
	// Set only by ReconcileHubWideAt, on a dry run too, for a seed-only board file; reconcile keeps the operator's value.
	Differs []string
	// Applied reports whether the file was written to disk, or deleted when a prune left it empty.
	Applied bool
	// Seed names the input ReconcileHubWideAt reconciled: SeedHub, SeedPrime or SeedTemplate.
	// Set on a dry run too; empty for ReconcileAll results.
	// A seed of a hub base file holds only the units that differ from the template.
	Seed string
	// Layer names the layer the result reports: the repo default or hub file, the worktree file, or the local override.
	Layer configengine.Layer
	// KeptExtra lists, sorted, each "<path>.<name>" a per-name map of the file holds and the template lacks.
	// Such a name is kept in place and never pruned.
	// Set on a dry run too.
	KeptExtra []string
	// Dropped lists, sorted, each "<path>.<name>" a per-name map of a base-layer file holds with the template's value.
	// The file leaves such a name out, because the load resolves it from the template.
	// Set on a dry run too.
	Dropped []string
	// Preserved lists, sorted, the dotted keys a sparse file holds that the template lacks.
	// They stay in place because the run did not prune unknown keys.
	// Set on a dry run too.
	Preserved []string
	// Moved lists one line per old key the module's move table moved or dropped, each naming the file and the full names.
	// Set on a dry run too.
	Moved []string
	// WroteTemplateBase reports that the apply wrote the template base beside a seed-only module's board file.
	// Set only by ReconcileHubWideAt.
	WroteTemplateBase bool
}

// ChangedFilesRel returns, relative to the board dir, each file that an applied hub-wide reconcile wrote.
// It holds the module's file, plus the template base when one was written.
func (r Result) ChangedFilesRel() []string {
	var files []string
	if r.Applied {
		files = append(files, configengine.ConfigFileRel(r.Module))
	}
	if r.WroteTemplateBase {
		files = append(files, configengine.TemplateBaseFileRel(r.Module))
	}
	return files
}

// templateBaseHeader opens a template base file, saying what it holds and that no one edits it.
const templateBaseHeader = "# lym's template for the file beside this one, as the hub reconcile last carried it in.\n# Reconcile reads it to tell a template change from a value the operator set; never edit it by hand.\n"

// templateBaseContent returns the template base a reconcile writes beside m's board file: the header over m's template.
func templateBaseContent(m configreg.Module) []byte {
	return []byte(templateBaseHeader + m.Template())
}

// readTemplateBase reads the template base beside m's board file under boardDir, reporting present false when there is none.
func readTemplateBase(m configreg.Module, boardDir string) (base []byte, present bool, err error) {
	path := configengine.TemplateBaseFile(boardDir, m.Name)
	base, err = os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, &FileError{Module: m.Name, Path: path, Err: fmt.Errorf("read the template base of %s: %w", m.Name, err)}
	}
	return base, true, nil
}

// Options holds the switches that control how a reconcile of every config module writes and removes files.
type Options struct {
	// Apply writes and removes files; false reports what would change.
	Apply bool
	// PruneUnknown removes from a hub worktree file each key the template lacks.
	// Without it such a key stays and is reported in Preserved,
	// so a binary older than the one that wrote the key never removes it.
	// A local file is pruned of every such key either way, since its load refuses one.
	PruneUnknown bool
}

// HubWideOptions holds the switches that control how a hub-wide reconcile writes and removes files.
type HubWideOptions struct {
	// Apply writes and removes files; false reports what would change.
	Apply bool
	// PruneUnknown removes from a present file each key the template lacks.
	// Without it such a key stays and is reported in Preserved,
	// so a binary older than the one that wrote the key never removes it from the committed board.
	PruneUnknown bool
	// SeedDefaults seeds a per-worktree module's absent repo default; false leaves an absent default absent.
	SeedDefaults bool
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

// effectiveTemplateOf composes the template m's base document picks.
// It fails as a *FileError that names path, the file holding the selector value.
func effectiveTemplateOf(m configreg.Module, base []byte, hub bool, path string) ([]byte, error) {
	template, err := configengine.EffectiveTemplate(m.Spec(), base, hub)
	if err != nil {
		return nil, &FileError{Module: m.Name, Path: path, Err: err}
	}
	return template, nil
}

// templateUnderBase composes the template the base layer's file under dirs picks.
// It fails as a *FileError that names that file.
func templateUnderBase(m configreg.Module, dirs configengine.Dirs) ([]byte, error) {
	spec := m.Spec()
	template, err := configengine.TemplateUnder(dirs, spec)
	if err != nil {
		basePath, _ := configengine.LayerFile(dirs, spec, configengine.BaseLayer(dirs, spec))
		return nil, &FileError{Module: m.Name, Path: basePath, Err: err}
	}
	return template, nil
}

// primeSeedError names the prime's copy of m in a seed failure its selector caused.
// It adds the step the refusal's way forward lacks:
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

// ReconcileAll reconciles the layers of a worktree under baseDir against the module templates.
// It returns one Result per reconciled file and any I/O or YAML parsing error.
// Standalone (boardDir empty), the worktree file is the base layer, fully reconciled whatever opts.PruneUnknown says.
// In a hub, the worktree file is sparse: a present file is pruned only under opts.PruneUnknown, and nothing is added.
// An absent file stays absent.
// The local file of every module is a sparse layer reconciled the same way;
// every key the template lacks is pruned from it.
// It is reported with layer local only when present.
// A seed-only module's worktree and local files are never rewritten;
// standalone, an absent worktree file materializes the template verbatim.
// A hub-wide module has no worktree file, so only its local file is reconciled.
// A file emptied by a prune is deleted.
// When opts.Apply is false, files are never written or removed.
func ReconcileAll(baseDir, boardDir string, opts Options) ([]Result, error) {
	var results []Result
	dirs := configengine.Dirs{Anchor: baseDir, Board: boardDir}
	for _, m := range configreg.Modules() {
		if !m.HubWide {
			var result Result
			var err error
			if boardDir == "" {
				result, err = reconcileBaseWorktreeFile(m, baseDir, opts.Apply)
			} else {
				result, _, err = reconcileSparseFile(m, dirs, configengine.ConfigFile(baseDir, m.Name), configengine.LayerWorktree, opts.Apply, opts.PruneUnknown)
			}
			if err != nil {
				return nil, err
			}
			results = append(results, result)
		}

		localResult, present, err := reconcileSparseFile(m, dirs, configengine.LocalFile(baseDir, m.Name), configengine.LayerLocal, opts.Apply, true)
		if err != nil {
			return nil, err
		}
		if present {
			results = append(results, localResult)
		}
	}
	return results, nil
}

// moveOldKeys moves the old keys the module's move table names in document, the content of the config file at file.
// It returns the moved bytes and one line per change.
// A module with no table gets document back unchanged and no line.
// A failure is a *FileError naming file.
func moveOldKeys(m configreg.Module, file string, document []byte) ([]byte, []string, error) {
	moves := m.Spec().MovedKeys
	if len(moves) == 0 {
		return document, nil, nil
	}
	moved, changes, err := yamlengine.MoveKeys(document, nil, moves)
	if err != nil {
		return nil, nil, &FileError{Module: m.Name, Path: file, Err: fmt.Errorf("move the old keys of %s: %w", m.Name, err)}
	}
	var lines []string
	for _, change := range changes {
		oldName, newName := configengine.KeyName(m.Name, change.Old), configengine.KeyName(m.Name, change.New)
		switch change.Outcome {
		case yamlengine.MoveDone:
			lines = append(lines, fmt.Sprintf("%s moved %s to %s", file, oldName, newName))
		case yamlengine.MoveDroppedNull:
			lines = append(lines, fmt.Sprintf("%s dropped %s, which was null", file, oldName))
		case yamlengine.MoveDroppedNewSet:
			lines = append(lines, fmt.Sprintf("%s dropped %s, since %s is set", file, oldName, newName))
		}
	}
	return moved, lines, nil
}

// reconcileBaseWorktreeFile fully reconciles a per-worktree module's worktree file.
// That file is the base layer of a standalone stack.
// Its old keys are moved first.
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

	existing, moved, err := moveOldKeys(m, cfgPath, existing)
	if err != nil {
		return Result{}, err
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
		Moved:     moved,
		Layer:     configengine.LayerWorktree,
	}
	if apply && (fileAbsent || len(added)+len(removed)+len(dropped)+len(moved) > 0) {
		if err := fsx.AtomicWriteBytes(cfgPath, merged); err != nil {
			return Result{}, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("write config for %s: %w", m.Name, err)}
		}
		result.Applied = true
	}
	return result, nil
}

// reconcileSparseFile reconciles one sparse layer file of a module.
// Its old keys are moved first, so a move is never pruned as an unknown key.
// The file is pruned of the keys the template lacks only when pruneUnknown is set, and nothing is added.
// A key kept for want of pruneUnknown is reported as preserved.
// An absent file stays absent and reports present false.
// A seed-only module's present file is untouched.
// A file emptied by a prune is deleted and reported applied.
func reconcileSparseFile(m configreg.Module, dirs configengine.Dirs, path string, layer configengine.Layer, apply, pruneUnknown bool) (result Result, present bool, err error) {
	result = Result{Module: m.Name, Layer: layer}
	original, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, false, nil
	}
	if err != nil {
		return Result{}, false, &FileError{Module: m.Name, Path: path, Err: fmt.Errorf("read config for %s: %w", m.Name, err)}
	}

	current := original
	if !m.SeedOnly {
		current, result.Moved, err = moveOldKeys(m, path, current)
		if err != nil {
			return Result{}, true, err
		}
		template, err := templateUnderBase(m, dirs)
		if err != nil {
			return Result{}, true, err
		}
		current, result.Removed, result.KeptExtra, result.Preserved, err = pruneSparse(m, template, current, pruneUnknown)
		if err != nil {
			return Result{}, true, &FileError{Module: m.Name, Path: path, Err: fmt.Errorf("reconcile %s: %w", m.Name, err)}
		}
	}

	if !apply || len(result.Removed)+len(result.Moved) == 0 {
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

// pruneSparse prunes current, a sparse file of m, of the units template lacks.
// It returns the pruned bytes with the removed, kept and preserved paths.
// Without pruneUnknown nothing is removed:
// the units the template lacks stay and come back as preserved,
// so a key a newer binary wrote survives a reconcile by an older one.
func pruneSparse(m configreg.Module, template, current []byte, pruneUnknown bool) (pruned []byte, removed, kept, preserved []string, err error) {
	pruned, removed, kept, err = yamlengine.Prune(template, current, m.PerNameMaps, m.OpenMaps...)
	if err != nil || pruneUnknown || len(removed) == 0 {
		return pruned, removed, kept, nil, err
	}
	return current, nil, kept, removed, nil
}

// sparseSeed returns the sparse base file that a seed of m becomes.
// It prunes the seed to the units the template holds and drops the units equal to the template.
// A seed with no such unit becomes the header-only base.
func sparseSeed(m configreg.Module, seed []byte) ([]byte, error) {
	spec := m.Spec()
	effective, err := configengine.EffectiveTemplate(spec, seed, true)
	if err != nil {
		return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
	}
	pruned, _, _, err := yamlengine.Prune(effective, seed, m.PerNameMaps, m.OpenMaps...)
	if err != nil {
		return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
	}
	stripped, _, err := yamlengine.Strip(effective, pruned, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, fmt.Errorf("reconcile %s: %w", m.Name, err)
	}
	if len(bytes.TrimSpace(stripped)) == 0 {
		return []byte(configengine.SparseBaseHeader), nil
	}
	return stripped, nil
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

// ReconcileHubWideAt reconciles each registered module's file in the board dir, in registry order.
// The returned slice holds one Result per module, with Layer default for the module's full base layer.
// primeBaseDir is the prime worktree's anchor dir; empty means no prime is resolvable.
//
// A seed-only module is reconciled as a full file, never pruned down to the template.
// A present file follows the template through yamlengine.FollowTemplate.
// Against the template base, a unit the base holds as it is takes the template's value;
// it is reported as Added, Updated or Removed.
// Any other unit is kept, and it is reported under Differs when the template holds a third value.
// Without a base, a unit the file lacks is added,
// and a unit it holds otherwise than the template is kept and reported under Differs.
// A module with run-settings blocks first has the old keys of its present file's and template base's blocks moved.
// The file is written only when the move or the merge rewrote it.
// An absent seed-only file is seeded from the prime's copy when one exists, and from the template otherwise.
// Apply also writes the template base when it is absent or stale, and reports WroteTemplateBase.
//
// Every other module's file is a sparse base over the template.
// A present file, or a prime copy seeding an absent one, has its old keys moved first.
// A present file is pruned only under opts.PruneUnknown.
// Otherwise its unknown keys are kept and reported as Preserved.
// Nothing is added.
// Per-name entries equal to the template's are dropped on every pass.
// The prime's copy seeds an absent file of a module with open maps, or one marked SeedsFromPrime, when it parses.
// Any other hub-wide module never reads the prime.
// Otherwise an absent file starts as the header-only base.
// A seed keeps only the units that differ from the template.
// An absent per-worktree default is seeded only under opts.SeedDefaults.
// It comes from the prime's copy when it exists and parses, else as the header-only base.
// Without opts.SeedDefaults it stays absent.
// A file the prune leaves with no unit is written as the header-only base, never deleted.
// When opts.Apply is false, nothing is written or removed.
func ReconcileHubWideAt(boardDir, primeBaseDir string, opts HubWideOptions) ([]Result, error) {
	var results []Result

	for _, m := range configreg.Modules() {
		cfgPath := configengine.ConfigFile(boardDir, m.Name)

		existing, err := os.ReadFile(cfgPath)
		fileAbsent := os.IsNotExist(err)
		if err != nil && !fileAbsent {
			return nil, &FileError{Module: m.Name, Path: cfgPath, Err: fmt.Errorf("read config for %s: %w", m.Name, err)}
		}

		if !m.HubWide && fileAbsent && !opts.SeedDefaults {
			results = append(results, Result{Module: m.Name, Layer: configengine.LayerDefault})
			continue
		}

		seed := SeedHub
		if fileAbsent {
			existing = nil
			seed = SeedTemplate
			if len(m.OpenMaps) > 0 || m.SeedsFromPrime || !m.HubWide {
				if prime := primeConfig(primeBaseDir, m.Name); prime != nil {
					existing = prime
					seed = SeedPrime
				}
			}
		}

		var templateBase []byte
		templateBasePresent := false
		if m.SeedOnly {
			if templateBase, templateBasePresent, err = readTemplateBase(m, boardDir); err != nil {
				return nil, err
			}
		}

		var added, removed, updated, differs, keptExtra, dropped, preserved, moved []string
		var mergedBytes []byte
		var reconcileErr error
		changed := fileAbsent
		switch {
		case m.SeedOnly:
			// An absent seed-only file with no prime copy is the template verbatim;
			// a present file or prime copy follows the template.
			mergedBytes = existing
			if mergedBytes == nil {
				mergedBytes = []byte(m.Template())
				added, reconcileErr = yamlengine.MissingKeys(mergedBytes, nil)
				break
			}
			// moveBlocks moves the old keys of every second-level mapping, one line per change naming its block.
			// A document that is no mapping comes back unchanged, for the follow to report.
			moveBlocks := func(document []byte) ([]byte, []string, error) {
				var tops map[string]yaml.Node
				if yaml.Unmarshal(document, &tops) != nil {
					return document, nil, nil
				}
				var lines []string
				for _, top := range slices.Sorted(maps.Keys(tops)) {
					node := tops[top]
					var blocks map[string]yaml.Node
					if node.Kind != yaml.MappingNode || node.Decode(&blocks) != nil {
						continue
					}
					for _, second := range slices.Sorted(maps.Keys(blocks)) {
						block := top + "." + second
						moved, blockLines, err := configreg.MoveSettings(document, []string{top, second})
						if err != nil {
							return nil, nil, fmt.Errorf("%s: %w", block, err)
						}
						document = moved
						for _, line := range blockLines {
							lines = append(lines, fmt.Sprintf("%s.yaml %s %s", m.Name, block, line))
						}
					}
				}
				return document, lines, nil
			}
			base := templateBase
			if m.RunSettingsBlocks && !fileAbsent {
				// The base moves too, so the follow compares file, base and template at the new paths.
				if templateBasePresent {
					var baseErr error
					if base, _, baseErr = moveBlocks(templateBase); baseErr != nil {
						reconcileErr = &FileError{Module: m.Name, Path: configengine.TemplateBaseFile(boardDir, m.Name), Err: baseErr}
						break
					}
				}
				if mergedBytes, moved, reconcileErr = moveBlocks(mergedBytes); reconcileErr != nil {
					break
				}
			}
			var follow yamlengine.FollowReport
			mergedBytes, follow, reconcileErr = yamlengine.FollowTemplate(base, []byte(m.Template()), mergedBytes, templateBasePresent && !fileAbsent)
			if errors.Is(reconcileErr, yamlengine.ErrInvalidBase) {
				reconcileErr = &FileError{Module: m.Name, Path: configengine.TemplateBaseFile(boardDir, m.Name), Err: fmt.Errorf("%w; delete it, and the next reconcile treats %s.yaml as having no base", reconcileErr, m.Name)}
			}
			added, updated, removed, differs = follow.Added, follow.Updated, follow.Removed, follow.Differs
			changed = changed || follow.Changed() || len(moved) > 0
		case fileAbsent:
			if seed == SeedPrime {
				var seeded []byte
				seeded, moved, reconcileErr = moveOldKeys(m, cfgPath, existing)
				var moveErr *FileError
				if errors.As(reconcileErr, &moveErr) {
					moveErr.Path = configengine.ConfigFile(primeBaseDir, m.Name)
				}
				if reconcileErr == nil {
					mergedBytes, reconcileErr = sparseSeed(m, seeded)
					if reconcileErr != nil {
						reconcileErr = primeSeedError(m, primeBaseDir, reconcileErr)
					}
				}
			} else {
				mergedBytes = []byte(configengine.SparseBaseHeader)
			}
		default:
			mergedBytes, moved, reconcileErr = moveOldKeys(m, cfgPath, existing)
			var template []byte
			if reconcileErr == nil {
				template, reconcileErr = effectiveTemplateOf(m, mergedBytes, true, cfgPath)
			}
			if reconcileErr == nil {
				mergedBytes, removed, keptExtra, preserved, reconcileErr = pruneSparse(m, template, mergedBytes, opts.PruneUnknown)
			}
			changed = len(removed) > 0 || len(moved) > 0
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
			Module:    m.Name,
			Added:     added,
			Removed:   removed,
			Updated:   updated,
			Differs:   differs,
			KeptExtra: keptExtra,
			Dropped:   dropped,
			Preserved: preserved,
			Moved:     moved,
			Seed:      seed,
			Layer:     configengine.LayerDefault,
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
		}
		if wantBase := templateBaseContent(m); opts.Apply && m.SeedOnly && !(templateBasePresent && bytes.Equal(templateBase, wantBase)) {
			basePath := configengine.TemplateBaseFile(boardDir, m.Name)
			if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
				return nil, &FileError{Module: m.Name, Path: basePath, Err: fmt.Errorf("mkdir config dir for the template base of %s: %w", m.Name, err)}
			}
			if err := fsx.AtomicWriteBytes(basePath, wantBase); err != nil {
				return nil, &FileError{Module: m.Name, Path: basePath, Err: fmt.Errorf("write the template base of %s: %w", m.Name, err)}
			}
			result.WroteTemplateBase = true
		}

		results = append(results, result)
	}

	return results, nil
}
