// resolve.go expands ${env:...} markers in YAML content.
// It walks the YAML node tree and replaces each marker with a value from a supplied environment map.

package yamlengine

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envMarkerRe matches ${env:NAME} and ${env:NAME:-default} tokens.
// Group 1 is NAME, group 2 is the optional ":-default" suffix including the ":-", and group 3 is the default value.
// Group 3 is captured only when group 2 matched.
var envMarkerRe = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)(:-((?s).*?))?\}`)

// Resolve expands ${env:...} markers in YAML content using the supplied environment map.
// An empty or whitespace-only src resolves to itself.
// The caller populates env, since Resolve performs no I/O.
func Resolve(src []byte, env map[string]string) ([]byte, error) {
	if len(strings.TrimSpace(string(src))) == 0 {
		return src, nil
	}

	var node yaml.Node
	if err := yaml.Unmarshal(src, &node); err != nil {
		return nil, fmt.Errorf("unmarshal YAML: %w", err)
	}

	if err := walkAndExpand(&node, env); err != nil {
		return nil, err
	}

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
		// Content alternates keys and values, so only the odd indices, the values, are expanded.
		for i := 1; i < len(n.Content); i += 2 {
			if err := walkAndExpand(n.Content[i], env); err != nil {
				return err
			}
		}

	case yaml.SequenceNode:
		for _, child := range n.Content {
			if err := walkAndExpand(child, env); err != nil {
				return err
			}
		}

	case yaml.ScalarNode:
		expanded, err := expandScalar(n.Value, env)
		if err != nil {
			return err
		}
		n.Value = expanded

	case yaml.AliasNode:
		// Alias nodes are references, so they are skipped;
		// the nodes they point to are processed where they occur directly.
	}

	return nil
}

// expandScalar expands the ${env:NAME} and ${env:NAME:-default} markers in a scalar string.
// The required form errors when NAME is absent from env.
// The optional form uses the default when NAME is absent or empty.
func expandScalar(s string, env map[string]string) (string, error) {
	matches := envMarkerRe.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s, nil
	}

	var result strings.Builder
	lastEnd := 0

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

		result.WriteString(s[lastEnd:matchStart])

		varName := s[nameStart:nameEnd]

		if hasOptional {
			defaultVal := s[defaultStart:defaultEnd]

			if envVal, ok := env[varName]; ok && envVal != "" {
				result.WriteString(envVal)
			} else {
				// The default is used verbatim, even when empty.
				result.WriteString(defaultVal)
			}
		} else {
			envVal, ok := env[varName]
			if !ok {
				return "", fmt.Errorf("unset required env var %q", varName)
			}
			result.WriteString(envVal)
		}

		lastEnd = matchEnd
	}

	result.WriteString(s[lastEnd:])

	return result.String(), nil
}
