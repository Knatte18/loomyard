// config.go implements strict YAML configuration loading backed by yamlengine and envsource.
//
// The Load function reads a YAML config file, validates it against a template,
// resolves environment variables, and returns the resolved bytes. The typed wrappers
// (board.LoadConfig, worktree.LoadConfig, weft.LoadConfig) unmarshal the resolved bytes
// into their own structs.

package configengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/envsource"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// FindBaseDir checks if <cwd>/_lyx exists, performing a strict check without
// walking up to parent directories. Returns cwd on success or an error on failure.
func FindBaseDir(cwd string) (string, error) {
	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
	_, err := os.Stat(lyxDir)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("not initialized: _lyx/ directory not found")
	} else if err != nil {
		return "", fmt.Errorf("stat _lyx: %w", err)
	}
	return cwd, nil
}

// Load loads and resolves configuration from a YAML file using a template.
// Returns the resolved bytes or an error if the file is absent, missing keys,
// or cannot be resolved.
func Load(baseDir, module string, template []byte) ([]byte, error) {
	_, err := FindBaseDir(baseDir)
	if err != nil {
		return nil, err
	}

	cfgPath := hubgeometry.ConfigFile(baseDir, module)
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
