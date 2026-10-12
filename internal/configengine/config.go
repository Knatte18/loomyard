// config.go implements two loading policies over one shared body for layered YAML configuration backed by yamlengine and envsource.
// Load is strict and used by hub-scoped modules where an absent base config means a broken hub.
// LoadOrTemplate resolves the caller's embedded template on proven absence of the base layer and is used by modules with a standalone entry point.
// PATTERN-config-strictness is the rule that decides which policy a new caller adopts.

package configengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/envsource"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// configDirName is the subdirectory name within lyxdirs.LyxDirName that holds
// configuration files.
const configDirName = "config"

// boardDirWayForwardClause ends every refusal that names a verb writing under the board dir.
// A session lyx refuses that verb from cannot run it itself.
const boardDirWayForwardClause = "; a session lyx refuses the verb from reports status: FAILED and the orch runs it"

// ErrNotInitialized marks a provably-absent _lyx/ directory: FindBaseDir wraps it into the error it
// returns when os.Stat confirms the directory does not exist.
// Callers wanting to distinguish absence from a stat failure should use errors.Is(err,
// ErrNotInitialized) rather than matching error text.
var ErrNotInitialized = errors.New("not initialized")

// ErrInvalid marks the failure of a present config file's content to parse or validate.
// An error without the mark is an unreadable file or an absent one, never the file's content.
// Test with errors.Is(err, ErrInvalid).
var ErrInvalid = errors.New("invalid config content")

// invalidError wraps an error so errors.Is matches ErrInvalid while Error stays the wrapped text.
type invalidError struct {
	err error
}

func (e invalidError) Error() string { return e.err.Error() }

func (e invalidError) Unwrap() []error { return []error{e.err, ErrInvalid} }

// MarkInvalid returns err marked as ErrInvalid, its text unchanged.
// A nil err stays nil.
func MarkInvalid(err error) error {
	if err == nil {
		return nil
	}
	return invalidError{err: err}
}

// FindBaseDir checks if <cwd>/_lyx exists, performing a strict check without walking up to parent
// directories.
// Returns cwd on success or an error on failure.
// An absent _lyx/ yields an error satisfying errors.Is(err, ErrNotInitialized);
// a stat failure (permission, IO) does not.
func FindBaseDir(cwd string) (string, error) {
	lyxDir := filepath.Join(cwd, lyxdirs.LyxDirName)
	_, err := os.Stat(lyxDir)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("%w: %s/ directory not found", ErrNotInitialized, lyxdirs.LyxDirName)
	} else if err != nil {
		return "", fmt.Errorf("stat %s: %w", lyxdirs.LyxDirName, err)
	}
	return cwd, nil
}

// ConfigDir returns the path to the config directory within a baseDir.
func ConfigDir(baseDir string) string {
	return filepath.Join(baseDir, lyxdirs.LyxDirName, configDirName)
}

// ScratchDir returns the never-tracked mirror of ConfigDir under the .lyx counterpart, for files a caller keeps beside the config that must never be committed.
func ScratchDir(baseDir string) string {
	return filepath.Join(baseDir, lyxdirs.DotLyxDirName, configDirName)
}

// ConfigFile returns the path to a module-specific configuration YAML file within a baseDir.
func ConfigFile(baseDir, module string) string {
	return filepath.Join(ConfigDir(baseDir), module+".yaml")
}

// StagingFile returns the path of a module's staging copy for an interactive edit:
// the ephemeral .lyx counterpart of ConfigFile, at the mirrored subpath, so a staged edit is never tracked.
func StagingFile(baseDir, module string) string {
	return filepath.Join(ScratchDir(baseDir), module+".yaml")
}

// ConfigFileRel returns a module's configuration YAML file path relative to a worktree's anchor --
// the same shape fabricengine.OriginRecordRel returns and the shape fabricengine.CommitAnchoredPaths
// expects for its relPaths argument.
// It is deliberately not derivable from ConfigFile by cancelling a base path: the two accessors join
// their shared segments independently, so ConfigFileRel has no baseDir parameter to cancel out.
func ConfigFileRel(module string) string {
	return filepath.Join(lyxdirs.LyxDirName, configDirName, module+".yaml")
}

// sparseMarkerName is the marker file's name; it has no .yaml suffix, so no module reader takes it for a module file.
const sparseMarkerName = "sparse"

// SparseMarker returns the tracked marker file under baseDir that records a worktree's config files were migrated to sparse layers.
func SparseMarker(baseDir string) string {
	return filepath.Join(ConfigDir(baseDir), sparseMarkerName)
}

// SparseMarkerRel returns the sparse marker's path relative to a worktree's anchor, the shape ConfigFileRel returns.
func SparseMarkerRel() string {
	return filepath.Join(lyxdirs.LyxDirName, configDirName, sparseMarkerName)
}

// Load loads and resolves a module's configuration from its layer stack under dirs.
// The base layer is required: a template key the stack lacks resolves to its template default and is logged,
// and no file is ever written.
// Returns the resolved bytes, or an error if the base layer is absent, a layer has a shape the template does not allow, lacks a key inside a list element, or cannot be resolved.
func Load(dirs Dirs, spec Spec) ([]byte, error) {
	return load(dirs, spec, false)
}

// LoadOrTemplate behaves identically to Load except that a provably-absent base-layer _lyx/ directory or base-layer file resolves the spec's template as the base instead of erroring, still stacking the upper layers present.
// A config file that exists but is invalid still errors.
// Any non-absence failure (permission, IO, a stat error) propagates unchanged, exactly as it does on the Load path.
func LoadOrTemplate(dirs Dirs, spec Spec) ([]byte, error) {
	return load(dirs, spec, true)
}

// load is the shared body behind Load and LoadOrTemplate.
// fallbackOnAbsent selects the policy: false is Load's strict behaviour, true is LoadOrTemplate's
// degrading behaviour.
// It consults fallbackOnAbsent at exactly two points, and at each one only on proven absence:
// an absent base-layer _lyx/ directory (errors.Is(err, ErrNotInitialized)) or an absent base-layer file.
// A permission or IO failure at either point always propagates unchanged regardless of fallbackOnAbsent.
// The layers merge unexpanded, the template keys the merge lacks are filled in memory and logged under both policies, and the merged document is expanded once.
// lyx config reconcile stays the only writer of config files.
func load(dirs Dirs, spec Spec, fallbackOnAbsent bool) ([]byte, error) {
	baseDir := dirs.Board
	if baseDir == "" {
		baseDir = dirs.Anchor
	}
	if baseDir == "" {
		return nil, fmt.Errorf("%s config: neither an anchor nor a board dir was told", spec.Name)
	}

	degradeReason := ""
	if _, err := FindBaseDir(baseDir); err != nil {
		if !fallbackOnAbsent || !errors.Is(err, ErrNotInitialized) {
			return nil, err
		}
		degradeReason = "absent _lyx/ directory"
	}

	// A worktree without _lyx/ beside a told board dir has no worktree or local layer; the board dir's base is not uninitialised for it.
	layerDirs := dirs
	if dirs.Board != "" && dirs.Anchor != "" {
		if _, err := FindBaseDir(dirs.Anchor); err != nil {
			if !errors.Is(err, ErrNotInitialized) {
				return nil, err
			}
			layerDirs.Anchor = ""
		}
	}

	stack, err := StackLayers(layerDirs, spec)
	if err != nil {
		return nil, err
	}

	hasBase := false
	for _, layer := range stack.Layers {
		if layer.Layer == BaseLayer(layerDirs, spec) {
			hasBase = true
		}
	}
	if !hasBase {
		if !fallbackOnAbsent {
			return nil, missingBaseError(layerDirs, spec)
		}
		if degradeReason == "" {
			degradeReason = "absent config file"
		}
		if len(stack.Layers) == 0 {
			return fallbackToTemplate(dirs, spec, degradeReason)
		}
		logger.Info("configengine: degrading to embedded template", "module", spec.Name, "reason", degradeReason)
	}

	label := stack.Layers[len(stack.Layers)-1].Path
	filled, filledKeys, err := yamlengine.FillMissing(spec.Template, stack.Doc, spec.OpenMaps...)
	if err != nil {
		return nil, MarkInvalid(fmt.Errorf("config file %s: %w", label, err))
	}
	if hasBase && len(filledKeys) > 0 {
		logger.Info("configengine: filled missing keys from template", "module", spec.Name, "layer", string(LayerTemplate), "file", label, "keys", strings.Join(filledKeys, ","))
	}

	// FillMissing never descends into lists, so a template key missing inside a list element is the one gap left;
	// reconcile carries lists whole and cannot add it either, so no reconcile hint.
	missing, err := yamlengine.MissingKeys(spec.Template, filled, spec.OpenMaps...)
	if err != nil {
		return nil, MarkInvalid(fmt.Errorf("config file %s: %w", label, err))
	}
	if len(missing) > 0 {
		return nil, MarkInvalid(fmt.Errorf("config file %s: missing keys: %s", label, strings.Join(missing, ", ")))
	}

	env, err := envsource.Build(expansionDir(dirs, spec))
	if err != nil {
		return nil, fmt.Errorf("config file %s: build environment: %w", label, err)
	}

	resolved, err := yamlengine.Resolve(filled, env)
	if err != nil {
		return nil, MarkInvalid(fmt.Errorf("config file %s: %w", label, err))
	}

	return resolved, nil
}

// expansionDir returns the directory whose .env and process environment expand the merged document:
// the board dir for a hub-wide module, else the anchor, else whichever is told.
func expansionDir(dirs Dirs, spec Spec) string {
	if spec.HubWide && dirs.Board != "" {
		return dirs.Board
	}
	if dirs.Anchor != "" {
		return dirs.Anchor
	}
	return dirs.Board
}

// missingBaseError names the absent base-layer file and the way forward for its kind:
// a worktree base is written by lyx config reconcile, a hub file by lyx fabric reconcile,
// and a per-worktree repo default only by the hub reconcile a run start performs.
func missingBaseError(dirs Dirs, spec Spec) error {
	path, _ := LayerFile(dirs, spec, BaseLayer(dirs, spec))
	switch {
	case dirs.Board == "":
		return fmt.Errorf("config file %s not found; run \"lyx config reconcile\"", path)
	case spec.HubWide:
		return fmt.Errorf("config file %s not found; run \"lyx fabric reconcile\"%s", path, boardDirWayForwardClause)
	default:
		return fmt.Errorf("config file %s not found; re-run the verb that starts the run, as \"lyx loom resume\"; its hub reconcile seeds the default from the prime%s", path, boardDirWayForwardClause)
	}
}

// fallbackToTemplate is load's tail when no layer file is present, reached only on proven absence of either the
// _lyx/ directory or the module's base config file.
// It skips yamlengine.MissingKeys entirely -- a template compared against itself is vacuously
// satisfied -- and resolves the template through the same env-marker pair the on-disk path uses,
// envsource.Build then yamlengine.Resolve, so a template default and an on-disk value expand by
// identical rules.
// reason names which of the two absence conditions fired, so the two cases are distinguishable in
// the durable trace.
func fallbackToTemplate(dirs Dirs, spec Spec, reason string) ([]byte, error) {
	logger.Info("configengine: degrading to embedded template", "module", spec.Name, "reason", reason)

	env, err := envsource.Build(expansionDir(dirs, spec))
	if err != nil {
		return nil, fmt.Errorf("%s config template: %w", spec.Name, err)
	}

	resolved, err := yamlengine.Resolve(spec.Template, env)
	if err != nil {
		return nil, fmt.Errorf("%s config template: %w", spec.Name, err)
	}

	return resolved, nil
}
