// parse.go implements Parse, the byte-slice recipe decoder, and Load, its told-path wrapper.
// Both perform every shape check a Recipe must pass before Build may consume it; neither validates
// routing, which shedengine and internal/shedcheck already own.

package shedbuild

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// supportedVersion is the only Recipe.Version value Parse accepts.
const supportedVersion = 1

// Parse decodes data into a Recipe, then runs every shape check the decoded value must pass before
// Build may consume it.
//
// A Decode error that wraps io.EOF -- what an empty, whitespace-only, or comments-only document
// yields -- becomes the distinct error "shedbuild: recipe is empty"; every other Decode error is
// wrapped as "shedbuild: %w", passing yaml's own text and its line-number position through
// verbatim.
//
// A recipe file is exactly ONE YAML document: a second one is a refusal, never a silent truncation
// of the producer graph.
//
// Parse validates nothing about routing: it never checks that OnDone or OnStuck names an existing
// row, that segments agree, that the graph is acyclic, or that any row is reachable, because
// shedengine's own validation and internal/shedcheck already own those and a third copy is what
// drifts.
func Parse(data []byte) (Recipe, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var recipe Recipe
	if err := dec.Decode(&recipe); err != nil {
		if errors.Is(err, io.EOF) {
			return Recipe{}, errors.New("shedbuild: recipe is empty")
		}
		return Recipe{}, fmt.Errorf("shedbuild: %w", err)
	}

	// A recipe file holds exactly ONE YAML document. Decoding once and stopping silently truncated
	// the producer graph at the first "---": an author resolving a merge conflict, or pasting a
	// replacement graph below the old one, kept only the prefix — and if that prefix happened to be
	// self-consistent, shedengine.validate saw no dangling target and the run proceeded on a
	// truncated pipeline. KnownFields(true) offers no protection past document one (crucible round
	// opus-medium-r6, R6-26).
	var trailing yaml.Node
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return Recipe{}, fmt.Errorf("shedbuild: %w", err)
		}
		return Recipe{}, fmt.Errorf("shedbuild: recipe carries more than one YAML document (a stray \"---\" at line %d); a recipe is exactly one document, and everything after the first was being dropped silently", trailing.Line)
	}

	if err := checkRecipeShape(recipe); err != nil {
		return Recipe{}, err
	}

	return recipe, nil
}

// checkRecipeShape runs Parse's own shape checks, in the fixed order Parse's godoc pins, returning
// the first failure.
func checkRecipeShape(recipe Recipe) error {
	if recipe.Version != supportedVersion {
		return fmt.Errorf("shedbuild: unsupported recipe version %d (supported: %d)", recipe.Version, supportedVersion)
	}
	if recipe.Entry == "" {
		return errors.New("shedbuild: entry must not be empty")
	}
	if len(recipe.Terminals) == 0 {
		return errors.New("shedbuild: terminals must not be empty")
	}
	if len(recipe.Producers) == 0 {
		return errors.New("shedbuild: producers must not be empty")
	}

	seen := make(map[string]int, len(recipe.Producers))
	for i, row := range recipe.Producers {
		if err := checkRowShape(i, row, seen); err != nil {
			return err
		}
		seen[row.Name] = i
	}

	return nil
}

// checkRowShape runs Parse's own per-row shape checks for the row at index i, consulting seen for
// the duplicate-name check.
func checkRowShape(i int, row Row, seen map[string]int) error {
	if row.Name == "" {
		return fmt.Errorf("shedbuild: producer %d: name must not be empty", i)
	}
	if row.Engine == "" {
		return fmt.Errorf("shedbuild: producer %d %q: engine must not be empty", i, row.Name)
	}
	if row.MaxBounces < 0 {
		return fmt.Errorf("shedbuild: producer %d %q: max_bounces must not be negative, got %d", i, row.Name, row.MaxBounces)
	}
	if earlier, ok := seen[row.Name]; ok {
		return fmt.Errorf("shedbuild: producer %d %q: duplicate name, already defined by producer %d", i, row.Name, earlier)
	}
	return nil
}

// Load reads the recipe file at path and delegates to Parse.
// path must be absolute; Load rejects a relative path before any read, exactly as
// internal/shedrecipe/paths.go rejects rather than resolves a relative Config value.
// Load is the only function in this package whose own code touches the filesystem.
func Load(path string) (Recipe, error) {
	if !filepath.IsAbs(path) {
		return Recipe{}, fmt.Errorf("shedbuild: recipe path %q must be absolute", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, fmt.Errorf("shedbuild: read recipe %q: %w", path, err)
	}

	return Parse(data)
}
