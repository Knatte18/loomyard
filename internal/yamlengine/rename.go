// rename.go renames top-level keys of a YAML document in place, keeping each value and every other key and comment.

package yamlengine

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// KeyRename names one top-level key's retired name and the name that replaces it.
type KeyRename struct {
	From, To string
}

// RenameKeys renames each From key of the document's top-level mapping to its To, in renames' order, and returns the rewritten document with one line per rename.
// A renamed value keeps its node and takes the line comment template carries on To, so a reconcile shows the new key's comment.
// A document already holding To keeps To's value and drops From.
// The lines read "<from>: renamed to <to>" and "<from>: removed, <to> already set".
// The document comes back unchanged, with no lines, when it is empty or holds no From key.
// A document or template that does not parse, or whose root is not a mapping, returns an error.
func RenameKeys(template, existing []byte, renames []KeyRename) ([]byte, []string, error) {
	document, root, err := parseMappingRoot(existing, "config")
	if err != nil {
		return nil, nil, err
	}
	if root == nil {
		return existing, nil, nil
	}
	_, templateRoot, err := parseMappingRoot(template, "template")
	if err != nil {
		return nil, nil, err
	}
	if templateRoot == nil {
		templateRoot = &yaml.Node{Kind: yaml.MappingNode}
	}

	var lines []string
	for _, rename := range renames {
		at := mappingIndex(root, rename.From)
		if at < 0 {
			continue
		}
		if mappingIndex(root, rename.To) >= 0 {
			root.Content = append(root.Content[:at], root.Content[at+2:]...)
			lines = append(lines, fmt.Sprintf("%s: removed, %s already set", rename.From, rename.To))
			continue
		}
		root.Content[at].Value = rename.To
		if templateAt := mappingIndex(templateRoot, rename.To); templateAt >= 0 {
			root.Content[at+1].LineComment = templateRoot.Content[templateAt+1].LineComment
		}
		lines = append(lines, fmt.Sprintf("%s: renamed to %s", rename.From, rename.To))
	}
	if len(lines) == 0 {
		return existing, nil, nil
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	return out.Bytes(), lines, nil
}
