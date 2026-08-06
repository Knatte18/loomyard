// resolve.go implements environment variable expansion in YAML content.
// It walks YAML node trees and replaces ${env:...} markers with values from a supplied environment map.

package yamlengine

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envMarkerRe matches ${env:NAME} or ${env:NAME:-default} tokens.
// Group 1: variable NAME
// Group 2: optional ":-default" suffix (including the :-)
// Group 3: the default value (captured only if group 2 matched)
var envMarkerRe = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)(:-((?s).*?))?\}`)

// Resolve expands ${env:...} markers in YAML content using the supplied environment map.
// An empty or whitespace-only src resolves to itself.
// The caller is responsible for populating env;
// Resolve performs no I/O.
func Resolve(src []byte, env map[string]string) ([]byte, error) {
	// Handle empty/whitespace-only input
	if len(strings.TrimSpace(string(src))) == 0 {
		return src, nil
	}

	// Parse YAML into a node tree
	var node yaml.Node
	if err := yaml.Unmarshal(src, &node); err != nil {
		return nil, fmt.Errorf("unmarshal YAML: %w", err)
	}

	// Walk and expand all scalar leaf nodes
	if err := walkAndExpand(&node, env); err != nil {
		return nil, err
	}

	// Marshal the mutated tree back to bytes
	out, err := yaml.Marshal(&node)
	if err != nil {
		return nil, fmt.Errorf("marshal YAML: %w", err)
	}

	return out, nil
}

// walkAndExpand recursively walks a node tree and expands env markers in
// every scalar leaf node, mutating the tree in place.
func walkAndExpand(n *yaml.Node, env map[string]string) error {
	if n == nil {
		return nil
	}

	switch n.Kind {
	case yaml.DocumentNode:
		// Document node: recurse into Content (usually has one child — the root)
		for _, child := range n.Content {
			if err := walkAndExpand(child, env); err != nil {
				return err
			}
		}

	case yaml.MappingNode:
		// Mapping node: Content contains alternating key-value pairs.
		// We only expand the values (odd indices).
		for i := 1; i < len(n.Content); i += 2 {
			if err := walkAndExpand(n.Content[i], env); err != nil {
				return err
			}
		}

	case yaml.SequenceNode:
		// Sequence node: recursively process each element
		for _, child := range n.Content {
			if err := walkAndExpand(child, env); err != nil {
				return err
			}
		}

	case yaml.ScalarNode:
		// Scalar leaf: expand env markers in its Value
		expanded, err := expandScalar(n.Value, env)
		if err != nil {
			return err
		}
		n.Value = expanded

	case yaml.AliasNode:
		// Alias nodes are references; skip them (they point to other nodes)
		// and let those be processed when encountered directly.
	}

	return nil
}

// expandScalar expands ${env:NAME} and ${env:NAME:-default} markers in a
// scalar string, supporting interpolation. Required form errors if NAME is
// absent. Optional form uses default when NAME is absent or empty.
func expandScalar(s string, env map[string]string) (string, error) {
	// Find all matches of the env-marker pattern
	matches := envMarkerRe.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		// No markers found; return unchanged
		return s, nil
	}

	var result strings.Builder
	lastEnd := 0

	// Process each match in order, building up the result string
	for _, m := range matches {
		// m[0:2] is the overall match
		// m[2:4] is group 1 (variable NAME)
		// m[4:6] is group 2 (optional ":-default" including the ":-")
		// m[6:8] is group 3 (the default value)

		matchStart, matchEnd := m[0], m[1]
		nameStart, nameEnd := m[2], m[3]
		hasOptional := m[4] >= 0 && m[6] >= 0
		var defaultStart, defaultEnd int
		if hasOptional {
			defaultStart, defaultEnd = m[6], m[7]
		}

		// Write the literal text before this match
		result.WriteString(s[lastEnd:matchStart])

		// Extract the variable name
		varName := s[nameStart:nameEnd]

		// Determine if this is optional form (has the ":-" group)
		if hasOptional {
			// Optional form: ${env:NAME:-default}
			defaultVal := s[defaultStart:defaultEnd]

			// Check if NAME is present and non-empty in env
			if envVal, ok := env[varName]; ok && envVal != "" {
				result.WriteString(envVal)
			} else {
				// Use the default verbatim (even if it's empty)
				result.WriteString(defaultVal)
			}
		} else {
			// Required form: ${env:NAME}
			envVal, ok := env[varName]
			if !ok {
				// Variable not in env — this is an error
				return "", fmt.Errorf("unset required env var %q", varName)
			}
			// Variable is present (even if empty); use its value
			result.WriteString(envVal)
		}

		lastEnd = matchEnd
	}

	// Write any remaining literal text after the last match
	result.WriteString(s[lastEnd:])

	return result.String(), nil
}
