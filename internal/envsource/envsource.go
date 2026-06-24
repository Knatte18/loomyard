// Package envsource reads environment variables from a .env file and OS environment.
// It provides a single source of truth for how environment variables enter the system,
// isolating env-sourcing policy from the configuration engine.
package envsource

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/paths"
)

// Build reads the .env file at paths.DotEnv(baseDir) and overlays the OS environment,
// returning a merged map where OS values take precedence over .env values.
//
// The .env file is parsed line-by-line, skipping blank lines and lines beginning with #.
// Each line is split on the first = only, so = may appear in the value. Lines without = are skipped.
// Values are not trimmed.
//
// If the .env file does not exist, Build returns a map containing only OS environment variables.
// OS environment variables always override .env values for the same key.
//
// Returns the merged map on success, or an error if the .env file cannot be read.
func Build(baseDir string) (map[string]string, error) {
	// Read the .env file
	dotEnvPath := paths.DotEnv(baseDir)
	dotEnvMap, err := readDotEnv(dotEnvPath)
	if err != nil {
		return nil, err
	}

	// Overlay OS environment; OS values win
	for _, envPair := range os.Environ() {
		idx := strings.Index(envPair, "=")
		if idx == -1 {
			// Malformed OS env entry; skip it
			continue
		}
		key := envPair[:idx]
		val := envPair[idx+1:]
		dotEnvMap[key] = val
	}

	return dotEnvMap, nil
}

// readDotEnv reads a .env file into a map.
//
// Returns an empty map if the file does not exist.
// Skips empty lines, comment lines (starting with #), and lines without =.
// Values are not trimmed; they are taken verbatim from the file.
func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("open .env: %w", err)
	}
	defer file.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip blank lines (not trimmed yet)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Split on the FIRST = only
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
		return nil, fmt.Errorf("scan .env: %w", err)
	}

	return result, nil
}
