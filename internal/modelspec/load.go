// load.go implements LoadRegistry, the models.yaml loader. It reads the
// optional per-hub registry file via hubgeometry.ConfigFile, validates every
// entry against the same closed vocabularies Parse and Resolve use, and
// merges the file onto the built-in fallback set by whole-entry replacement.

package modelspec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"gopkg.in/yaml.v3"
)

// LoadRegistry loads the models.yaml registry rooted at baseDir. An absent file
// returns builtins() unchanged; a present file is decoded with strict validation
// and merged onto builtins() by whole-entry replacement (no field-level merge).
func LoadRegistry(baseDir string) (Registry, error) {
	path := hubgeometry.ConfigFile(baseDir, "models")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// models.yaml is optional — no file means "use the built-ins",
			// not a failure.
			return builtins(), nil
		}
		return nil, fmt.Errorf("modelspec: read %s: %w", path, err)
	}

	var fileEntries map[string]Entry
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&fileEntries); err != nil {
		// An empty or comments-only file yields io.EOF from Decode with no
		// fileEntries set — that is a valid "no entries" file, not malformed
		// YAML.
		if errors.Is(err, io.EOF) {
			return builtins(), nil
		}
		return nil, fmt.Errorf("modelspec: parse %s: %w", path, err)
	}

	registry := builtins()
	for alias, entry := range fileEntries {
		if err := validateAlias(alias, entry, path); err != nil {
			return nil, err
		}
		// Whole-entry replacement: the file's entry for this alias replaces
		// the built-in (or absent) entry outright — no field-level merge,
		// so an override can never leak a stale built-in default.
		registry[alias] = entry
	}

	return registry, nil
}

// validateAlias checks one models.yaml entry against closed vocabularies and charset.
func validateAlias(alias string, entry Entry, path string) error {
	if err := validateCharset(alias, "alias", isIdentChar); err != nil {
		return fmt.Errorf("modelspec: %s: invalid alias %q: %w", path, alias, err)
	}
	if entry.Engine == "" {
		return fmt.Errorf("modelspec: %s: alias %q has no engine", path, alias)
	}
	if !knownEngines[entry.Engine] {
		return fmt.Errorf("modelspec: %s: alias %q has unknown engine %q", path, alias, entry.Engine)
	}
	if entry.Model == "" {
		return fmt.Errorf("modelspec: %s: alias %q has no model", path, alias)
	}
	for key, value := range entry.Defaults {
		if !knownParams[key] {
			return fmt.Errorf("modelspec: %s: alias %q has unknown defaults key %q", path, alias, key)
		}
		if value == "" {
			return fmt.Errorf("modelspec: %s: alias %q has empty defaults value for key %q", path, alias, key)
		}
	}
	return nil
}
