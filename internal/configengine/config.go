// config.go implements two loading policies over one shared body for YAML configuration backed by
// yamlengine and envsource: Load, which is strict and used by hub-scoped modules where an absent
// config means a broken hub, and LoadOrTemplate, which resolves the caller's embedded template on
// proven absence and is used by modules with a standalone entry point.
// CONSTRAINTS.md's Config Strictness Invariant is the rule that decides which policy a new caller
// adopts.

package configengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/envsource"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// configDirName is the subdirectory name within lyxdirs.LyxDirName that holds
// configuration files.
const configDirName = "config"

// ErrNotInitialized marks a provably-absent _lyx/ directory: FindBaseDir wraps it into the error it
// returns when os.Stat confirms the directory does not exist.
// Callers wanting to distinguish absence from a stat failure should use errors.Is(err,
// ErrNotInitialized) rather than matching error text.
var ErrNotInitialized = errors.New("not initialized")

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

// ConfigFile returns the path to a module-specific configuration YAML file within a baseDir.
func ConfigFile(baseDir, module string) string {
	return filepath.Join(ConfigDir(baseDir), module+".yaml")
}

// ConfigFileRel returns a module's configuration YAML file path relative to a worktree's anchor --
// the same shape fabricengine.OriginRecordRel returns and the shape fabricengine.CommitAnchoredPaths
// expects for its relPaths argument.
// It is deliberately not derivable from ConfigFile by cancelling a base path: the two accessors join
// their shared segments independently, so ConfigFileRel has no baseDir parameter to cancel out.
func ConfigFileRel(module string) string {
	return filepath.Join(lyxdirs.LyxDirName, configDirName, module+".yaml")
}

// Load loads and resolves configuration from a YAML file using a template.
// Returns the resolved bytes or an error if the file is absent, missing keys, or cannot be
// resolved.
func Load(baseDir, module string, template []byte) ([]byte, error) {
	return load(baseDir, module, template, false)
}

// LoadOrTemplate behaves identically to Load except that a provably-absent _lyx/ directory or a
// provably-absent config file resolves the caller-supplied template instead of erroring.
// A config file that exists but is invalid still errors, and any non-absence failure (permission,
// IO, a stat error) propagates unchanged, exactly as it does on the Load path.
func LoadOrTemplate(baseDir, module string, template []byte) ([]byte, error) {
	return load(baseDir, module, template, true)
}

// load is the shared body behind Load and LoadOrTemplate.
// fallbackOnAbsent selects the policy: false is Load's strict behaviour, true is LoadOrTemplate's
// degrading behaviour.
// It consults fallbackOnAbsent at exactly two points, and at each one only on proven absence -- an
// absent _lyx/ directory (errors.Is(err, ErrNotInitialized)) or an absent config file
// (os.IsNotExist(err)) -- so a permission or IO failure at either point always propagates unchanged
// regardless of fallbackOnAbsent.
func load(baseDir, module string, template []byte, fallbackOnAbsent bool) ([]byte, error) {
	_, err := FindBaseDir(baseDir)
	if err != nil {
		if fallbackOnAbsent && errors.Is(err, ErrNotInitialized) {
			return fallbackToTemplate(baseDir, module, template, "absent _lyx/ directory")
		}
		return nil, err
	}

	cfgPath := ConfigFile(baseDir, module)
	fileBytes, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		if fallbackOnAbsent {
			return fallbackToTemplate(baseDir, module, template, "absent config file")
		}
		return nil, fmt.Errorf("config file %s not found; run \"lyx config reconcile\"", cfgPath)
	}
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", cfgPath, err)
	}

	missing, err := yamlengine.MissingKeys(template, fileBytes)
	if err != nil {
		return nil, fmt.Errorf("config file %s: %w", cfgPath, err)
	}
	if len(missing) > 0 {
		missingStr := ""
		for _, key := range missing {
			if missingStr != "" {
				missingStr += ", "
			}
			missingStr += key
		}
		return nil, fmt.Errorf("config file %s: missing keys: %s; run \"lyx config reconcile\"", cfgPath, missingStr)
	}

	env, err := envsource.Build(baseDir)
	if err != nil {
		return nil, fmt.Errorf("config file %s: build environment: %w", cfgPath, err)
	}

	resolved, err := yamlengine.Resolve(fileBytes, env)
	if err != nil {
		return nil, fmt.Errorf("config file %s: %w", cfgPath, err)
	}

	return resolved, nil
}

// fallbackToTemplate is load's shared fallback tail, reached only on proven absence of either the
// _lyx/ directory or the module's config file.
// It skips yamlengine.MissingKeys entirely -- a template compared against itself is vacuously
// satisfied -- and resolves the template through the same env-marker pair the on-disk path uses,
// envsource.Build then yamlengine.Resolve, so a template default and an on-disk value expand by
// identical rules.
// reason names which of the two absence conditions fired, so the two cases are distinguishable in
// the durable trace.
func fallbackToTemplate(baseDir, module string, template []byte, reason string) ([]byte, error) {
	logger.Info("configengine: degrading to embedded template", "module", module, "reason", reason)

	env, err := envsource.Build(baseDir)
	if err != nil {
		return nil, fmt.Errorf("%s config template: %w", module, err)
	}

	resolved, err := yamlengine.Resolve(template, env)
	if err != nil {
		return nil, fmt.Errorf("%s config template: %w", module, err)
	}

	return resolved, nil
}
