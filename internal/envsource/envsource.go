// Package envsource reads environment variables from a .env file and OS environment.
// It provides a single source of truth for how environment variables enter the system,
// isolating env-sourcing policy from the configuration engine.
package envsource

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// Build reads the .env file at hubgeometry.DotEnv(baseDir) and overlays the
// OS environment, with OS values taking precedence. The .env file is parsed
// line-by-line, skipping blank lines and comments; each line is split on the
// first = only. Absent .env files return only OS environment. Returns the
// merged map, or an error if .env cannot be read.
func Build(baseDir string) (map[string]string, error) {
	// Read the .env file
	dotEnvPath := hubgeometry.DotEnv(baseDir)
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

// readDotEnv reads a .env file into a map, returning an empty map if the file
// does not exist. Skips empty lines, comments, and lines without =. Values are
// taken verbatim (not trimmed).
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
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		idx := strings.Index(line, "=")
		if idx == -1 {
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
