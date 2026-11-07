// Package envsource reads environment variables from a .env file and OS environment.
// It is the single source of truth for how environment variables enter the system,
// and it keeps env-sourcing policy out of `configengine`.
package envsource

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dotEnvName is the file name of the .env file whose variables the OS environment may override.
const dotEnvName = ".env"

// DotEnv returns the path to the .env file within a baseDir.
func DotEnv(baseDir string) string {
	return filepath.Join(baseDir, dotEnvName)
}

// Build reads the .env file in baseDir and overlays the OS environment, with OS values taking precedence.
// The .env file is parsed line-by-line, skipping blank lines and comments;
// each line is split on the first = only.
// An absent .env file yields only the OS environment.
// It returns the merged map, or an error if the .env file cannot be read.
func Build(baseDir string) (map[string]string, error) {
	dotEnvPath := DotEnv(baseDir)
	dotEnvMap, err := readDotEnv(dotEnvPath)
	if err != nil {
		return nil, err
	}

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

// readDotEnv reads a .env file into a map, which is empty when the file does not exist.
// It skips empty lines, comments, and lines without =.
// Values are taken verbatim, not trimmed.
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
