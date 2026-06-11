// config.go — generic two-layer configuration loader.
//
// Provides Load function to merge defaults with YAML-based configuration,
// supporting environment variable expansion with required ($env:NAME) and
// optional ($env:NAME ? fallback) syntax.

package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envOptRe matches $env:NAME ? fallback tokens where NAME is [A-Za-z_][A-Za-z0-9_]*
// and fallback is any trailing content on the line.
var envOptRe = regexp.MustCompile(`\$env:([A-Za-z_][A-Za-z0-9_]*)\s*\?\s*(.*)$`)

// envReqRe matches $env:NAME tokens where NAME is [A-Za-z_][A-Za-z0-9_]*
var envReqRe = regexp.MustCompile(`\$env:([A-Za-z_][A-Za-z0-9_]*)`)

// FindBaseDir checks if <cwd>/_mhgo exists and returns cwd, or an error if not found.
//
// It performs a strict check without walking up to parent directories.
// Returns the cwd on success, empty string and an error on failure.
func FindBaseDir(cwd string) (string, error) {
	mhgoDir := filepath.Join(cwd, "_mhgo")
	_, err := os.Stat(mhgoDir)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("not initialized: _mhgo/ directory not found in %s", cwd)
	} else if err != nil {
		return "", fmt.Errorf("stat _mhgo: %w", err)
	}
	return cwd, nil
}

// Load loads configuration for a module from defaults and layered configuration files.
//
// Merges defaults with <baseDir>/_mhgo/<module>.yaml (if present) and expands
// environment variables using $env:NAME and $env:NAME ? fallback syntax.
//
// If <baseDir>/_mhgo/ does not exist, returns an error.
// If <baseDir>/.env is present, it is loaded and used for env var lookups.
// OS environment takes precedence over .env values.
func Load(baseDir, module string, defaults map[string]string) (map[string]string, error) {
	// Check if _mhgo/ directory exists
	_, err := FindBaseDir(baseDir)
	if err != nil {
		return nil, err
	}

	// Load .env file
	dotenv, err := loadDotEnv(filepath.Join(baseDir, ".env"))
	if err != nil {
		return nil, err
	}

	// Start with defaults
	result := make(map[string]string, len(defaults))
	for k, v := range defaults {
		result[k] = v
	}

	// Load and merge YAML layer
	yamlMap, err := loadYAMLLayer(filepath.Join(baseDir, "_mhgo", module+".yaml"))
	if err != nil {
		return nil, err
	}
	for k, v := range yamlMap {
		result[k] = v
	}

	// Expand environment variables in all values
	for key, val := range result {
		expanded, err := expandEnv(val, dotenv)
		if err != nil {
			return nil, fmt.Errorf("config key %q: %w", key, err)
		}
		result[key] = expanded
	}

	return result, nil
}

// loadDotEnv loads a .env file into a map.
//
// Returns an empty map if the file does not exist.
// Skips empty lines, comment lines (starting with #), and lines without =.
func loadDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.Index(line, "=")
		if idx == -1 {
			// No = found, skip this line
			continue
		}

		key := line[:idx]
		val := line[idx+1:]
		result[key] = val
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// loadYAMLLayer loads a YAML file into a string map.
//
// Returns an empty map if the file does not exist.
func loadYAMLLayer(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	err = yaml.Unmarshal(content, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// expandEnv expands environment variables in a value string.
//
// Supports two syntaxes:
// - $env:NAME — required variable; error if unset
// - $env:NAME ? fallback — optional variable; use fallback if unset
//
// Uses a three-phase algorithm:
// 1. Check for optional syntax
// 2. Expand required tokens in prefix (before optional marker)
// 3. Handle optional token if present
//
// OS environment is checked first, then the dotenv map.
func expandEnv(value string, dotenv map[string]string) (string, error) {
	// Phase 1: Check for optional syntax
	optMatch := envOptRe.FindStringSubmatchIndex(value)
	var prefix string
	var optionalName string
	var fallback string

	if optMatch != nil {
		matchStart := optMatch[0]
		prefix = value[:matchStart]
		optionalName = value[optMatch[2]:optMatch[3]]
		fallback = value[optMatch[4]:optMatch[5]]
	} else {
		prefix = value
	}

	// Phase 2: Expand required tokens in prefix
	var expandErr error
	expandedPrefix := envReqRe.ReplaceAllStringFunc(prefix, func(tok string) string {
		// Extract the variable name from $env:NAME
		name := tok[5:] // Skip "$env:"

		// Look up in OS env first, then dotenv
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
		if val, ok := dotenv[name]; ok {
			return val
		}

		// Not found, mark error and return unchanged
		if expandErr == nil {
			expandErr = fmt.Errorf("unset required env var %q", name)
		}
		return tok
	})

	if expandErr != nil {
		return "", expandErr
	}

	// Phase 3: Handle optional token if present
	if optMatch != nil {
		// Look up optional variable
		if val, ok := os.LookupEnv(optionalName); ok {
			return expandedPrefix + val, nil
		}
		if val, ok := dotenv[optionalName]; ok {
			return expandedPrefix + val, nil
		}

		// Not set, use fallback
		return expandedPrefix + strings.TrimSpace(fallback), nil
	}

	// No optional token, return the expanded prefix
	return expandedPrefix, nil
}
