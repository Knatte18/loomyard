// load.go implements LoadRegistry, the optional servers.yaml overlay loader.
// It mirrors internal/modelspec's LoadRegistry: the file is read via
// hubgeometry.ConfigFile so its location is never hand-joined (Hub Geometry
// Invariant), an absent file falls back to builtins() with no error, and
// present entries whole-replace the corresponding built-in.

package codeintelengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"gopkg.in/yaml.v3"
)

// LoadRegistry loads the servers.yaml registry rooted at baseDir. The path
// is always hubgeometry.ConfigFile(baseDir, "servers") — never hand-joined,
// per the Hub Geometry Invariant. An absent file is deliberately NOT an
// error: servers.yaml is optional, so a fresh hub with no file at all still
// resolves every built-in language via builtins(). Any other read error
// (permissions, a directory where a file is expected, …) is wrapped with the
// path for context.
//
// When the file is present, its entries are decoded with
// yaml.Decoder.KnownFields(true) into map[string]Entry — an unknown YAML
// field anywhere in an entry is a loud error — and then each is validated
// with validateEntry, naming the offending alias and the file path on
// failure.
//
// The result is built from builtins(), with each file entry overlaid as a
// WHOLE-ENTRY replacement: a file "go:" block replaces the built-in go entry
// entirely (markers, match, command, and install hint together), never
// merging field-by-field. An empty or comments-only file yields builtins()
// unchanged.
func LoadRegistry(baseDir string) (Registry, error) {
	path := hubgeometry.ConfigFile(baseDir, "servers")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// servers.yaml is optional — no file means "use the built-ins",
			// not a failure.
			return builtins(), nil
		}
		return nil, fmt.Errorf("codeintelengine: read %s: %w", path, err)
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
		return nil, fmt.Errorf("codeintelengine: parse %s: %w", path, err)
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
