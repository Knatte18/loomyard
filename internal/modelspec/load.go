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

// LoadRegistry loads the models.yaml registry rooted at baseDir. The path is
// always hubgeometry.ConfigFile(baseDir, "models") — never hand-joined, per
// the Hub Geometry Invariant. An absent file is deliberately NOT an error
// (unlike configengine.Load's pattern): models.yaml is optional, so a fresh
// hub with no file at all still resolves every built-in alias. Any other
// read error (permissions, a directory where a file is expected, …) is
// wrapped with the path for context.
//
// When the file is present, its entries are decoded with
// yaml.Decoder.KnownFields(true) into map[string]Entry — an unknown YAML
// field anywhere in an entry is a loud error — and then validated: an alias
// key must match [a-z0-9-]+; Engine must be non-empty and a known engine;
// Model must be non-empty (a free-form string, never checked against any
// model list, per the new-model-without-recompile requirement); every
// Defaults key must be a known param with a non-empty value. Every failure
// names the offending alias and the file path.
//
// The result is built from builtins(), with each file entry overlaid as a
// WHOLE-ENTRY replacement: a file "sonnet:" block replaces the built-in
// sonnet entry entirely (engine, model, and defaults together), never
// merging field-by-field. An empty or comments-only file yields builtins()
// unchanged.
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

// validateAlias checks one decoded models.yaml entry against the closed
// vocabularies and the alias charset, naming alias and path in every error.
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
