// load.go implements ConfigSpec and LoadRegistry, the models.yaml loader.
// It reads the optional registry layers via configengine.StackLayers, validates every entry
// against the same closed vocabularies Parse and Resolve use, and merges the stack onto the built-in
// fallback set by whole-entry replacement.

package modelspec

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// ConfigSpec returns models's layered-load spec.
// Each top-level key is one alias, carried whole through the stack, and the four template aliases are classified as meeting no category, so the local layer may hold them and any alias of its own.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:         "models",
		Template:     []byte(ConfigTemplate()),
		WholeEntries: true,
		Categories: map[string]configengine.Category{
			"sonnet": configengine.CategoryNone,
			"opus":   configengine.CategoryNone,
			"haiku":  configengine.CategoryNone,
			"fable":  configengine.CategoryNone,
		},
	}
}

// LoadRegistry loads the models.yaml registry through the layer stack under dirs.
// No layer file returns builtins() unchanged;
// each present layer is decoded with strict validation, naming that layer's file in an error,
// and the stack's merged entries are laid over builtins() by whole-entry replacement (no field-level merge).
func LoadRegistry(dirs configengine.Dirs) (Registry, error) {
	stack, err := configengine.StackLayers(dirs, ConfigSpec())
	if err != nil {
		return nil, fmt.Errorf("modelspec: %w", err)
	}
	for _, layer := range stack.Layers {
		entries, err := decodeEntries(layer.Bytes, layer.Path)
		if err != nil {
			return nil, err
		}
		for alias, entry := range entries {
			if err := validateAlias(alias, entry, layer.Path); err != nil {
				return nil, err
			}
		}
	}

	entries, err := decodeEntries(stack.Doc, "the merged layers")
	if err != nil {
		return nil, err
	}
	registry := builtins()
	for alias, entry := range entries {
		// Whole-entry replacement: the stack's entry for this alias replaces
		// the built-in (or absent) entry outright — no field-level merge,
		// so an override can never leak a stale built-in default.
		registry[alias] = entry
	}

	return registry, nil
}

// decodeEntries strictly decodes models.yaml content into its alias entries, with an empty or comments-only document yielding none.
// source names the content in a parse error.
func decodeEntries(data []byte, source string) (map[string]Entry, error) {
	var entries map[string]Entry
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&entries); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("modelspec: parse %s: %w", source, err)
	}
	return entries, nil
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
