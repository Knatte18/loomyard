// load.go implements LoadRegistry, the optional servers.yaml overlay loader.
// It mirrors internal/modelspec's LoadRegistry: the file is read via
// configengine.ConfigFile so its location is never hand-joined (Hub Geometry
// Invariant), an absent file falls back to builtins() with no error, and
// present entries whole-replace the corresponding built-in.

package scoutengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// LoadRegistry loads the optional servers.yaml overlay, replacing built-in entries whole.
// An absent file returns builtins(); an empty file also returns builtins() unchanged.
func LoadRegistry(baseDir string) (Registry, error) {
	path := configengine.ConfigFile(baseDir, "servers")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// servers.yaml is optional — no file means "use the built-ins",
			// not a failure.
			return builtins(), nil
		}
		return nil, fmt.Errorf("scoutengine: read %s: %w", path, err)
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
		return nil, fmt.Errorf("scoutengine: parse %s: %w", path, err)
	}

	registry := builtins()
	for name, entry := range fileEntries {
		if err := validateEntry(name, entry); err != nil {
			// validateEntry's message already names the offending entry;
			// prepend the file path so the operator knows which
			// servers.yaml to fix.
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		// Whole-entry replacement: the file's entry for this language
		// replaces the built-in (or absent) entry outright — no field-level
		// merge, so an override can never leak a stale built-in default.
		registry[name] = entry
	}

	return registry, nil
}
