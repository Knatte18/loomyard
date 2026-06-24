// config.go implements strict YAML configuration loading backed by yamlengine and envsource.
//
// The Load function reads a YAML config file, validates it against a template,
// resolves environment variables, and returns the resolved bytes. The typed wrappers
// (board.LoadConfig, worktree.LoadConfig, weft.LoadConfig) unmarshal the resolved bytes
// into their own structs.

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/envsource"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// FindBaseDir checks if <cwd>/_lyx exists and returns cwd, or an error if not found.
//
// It performs a strict check without walking up to parent directories.
// Returns the cwd on success, empty string and an error on failure.
func FindBaseDir(cwd string) (string, error) {
	lyxDir := filepath.Join(cwd, paths.LyxDirName)
	_, err := os.Stat(lyxDir)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("not initialized: _lyx/ directory not found")
	} else if err != nil {
		return "", fmt.Errorf("stat _lyx: %w", err)
	}
	return cwd, nil
}

// Load loads and resolves configuration from a YAML file using a template.
//
// Flow:
// 1. Call FindBaseDir(baseDir) and propagate its error.
// 2. Compute cfgPath := paths.ConfigFile(baseDir, module) and read it.
//    If the file is absent, return an error naming the path and instructing "lyx update".
// 3. Check for missing keys in the file via yamlengine.MissingKeys(template, fileBytes).
//    If keys are missing, return an error naming cfgPath, the missing key-paths, and "lyx update".
// 4. Build the environment via envsource.Build(baseDir).
// 5. Resolve fileBytes via yamlengine.Resolve(fileBytes, env).
// 6. Return the resolved bytes.
//
// Errors from steps 3-5 wrap the underlying error with the config key/file context.
func Load(baseDir, module string, template []byte) ([]byte, error) {
	// Step 1: Check if _lyx/ directory exists
	_, err := FindBaseDir(baseDir)
	if err != nil {
		return nil, err
	}

	// Step 2: Read the config file
	cfgPath := paths.ConfigFile(baseDir, module)
	fileBytes, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("config file %s not found; run \"lyx update\"", cfgPath)
	}
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", cfgPath, err)
	}

	// Step 3: Check for missing keys
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
		return nil, fmt.Errorf("config file %s: missing keys: %s; run \"lyx update\"", cfgPath, missingStr)
	}

	// Step 4: Build the environment
	env, err := envsource.Build(baseDir)
	if err != nil {
		return nil, fmt.Errorf("config file %s: build environment: %w", cfgPath, err)
	}

	// Step 5: Resolve environment variables
	resolved, err := yamlengine.Resolve(fileBytes, env)
	if err != nil {
		return nil, fmt.Errorf("config file %s: %w", cfgPath, err)
	}

	// Step 6: Return resolved bytes
	return resolved, nil
}
