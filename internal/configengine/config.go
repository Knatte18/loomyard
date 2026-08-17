// config.go implements strict YAML configuration loading backed by yamlengine and envsource.
//
// The Load function reads a YAML config file, validates it against a template, resolves environment
// variables, and returns the resolved bytes.
// The typed wrappers (board.LoadConfig, worktree.LoadConfig, fabric.LoadConfig) unmarshal the
// resolved bytes into their own structs.

package configengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/envsource"
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

// Load loads and resolves configuration from a YAML file using a template.
// Returns the resolved bytes or an error if the file is absent, missing keys, or cannot be
// resolved.
func Load(baseDir, module string, template []byte) ([]byte, error) {
	_, err := FindBaseDir(baseDir)
	if err != nil {
		return nil, err
	}

	cfgPath := ConfigFile(baseDir, module)
	fileBytes, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
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
