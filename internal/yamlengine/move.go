// move.go implements the comment-keeping move of renamed keys in a YAML document.

package yamlengine

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// MoveOutcome says what a key move did with an old key.
type MoveOutcome string

const (
	// MoveDone put the old key's value at its new path.
	MoveDone MoveOutcome = "moved"
	// MoveDroppedNull removed an old key holding null, since its absent new path reads the same default.
	MoveDroppedNull MoveOutcome = "dropped-null"
	// MoveDroppedNewSet removed an old key whose new path the document already held, keeping the new path's value.
	MoveDroppedNewSet MoveOutcome = "dropped-new-set"
)

// Move is one change a key move made: the old and new dotted paths, relative to the prefix moved under, and the outcome.
type Move struct {
	Old     string
	New     string
	Outcome MoveOutcome
}

// MoveKeys moves keys of the YAML document existing to their new places, so a renamed key keeps its value.
// moves maps each old dotted key to its new dotted path, both under the mapping that prefix leads to.
// prefix holds one key per level, so a key holding a dot is addressed as it is; an empty prefix is the root.
// A moved key keeps its value and comments, and missing mappings on the new path are created in block style.
// An alias that a move leaves ahead of its anchor, or without it, is written as a copy of the anchored value.
// An old key holding null is dropped, and so is one whose new path is already held.
// An old key that is also the area its new path lies under is left alone while it holds a mapping.
// Such keys move before the others, so another key can move into the area they free.
// The changes are returned sorted by old key.
// A document holding no old key, or no mapping at prefix, is returned byte for byte with no change.
// A document that does not parse, or a non-mapping value on a new path, is an error.
func MoveKeys(existing []byte, prefix []string, moves map[string]string) ([]byte, []Move, error) {
	doc, root, err := parseMappingRoot(existing, "existing")
	if err != nil {
		return nil, nil, err
	}
	if root == nil {
		return existing, nil, nil
	}
	under := root
	for _, segment := range prefix {
		at := mappingIndex(under, segment)
		if at < 0 || under.Content[at+1].Kind != yaml.MappingNode {
			return existing, nil, nil
		}
		under = under.Content[at+1]
	}

	namesOwnArea := func(old string) bool { return strings.HasPrefix(moves[old], old+".") }
	order := slices.Sorted(maps.Keys(moves))
	slices.SortStableFunc(order, func(a, b string) int {
		switch {
		case namesOwnArea(a) == namesOwnArea(b):
			return 0
		case namesOwnArea(a):
			return -1
		}
		return 1
	})

	var done []Move
	for _, old := range order {
		parentPath, name := splitLastSegment(old)
		parent := under
		if parentPath != "" {
			parent = findValueNode(under, parentPath)
		}
		if parent == nil || parent.Kind != yaml.MappingNode {
			continue
		}
		at := mappingIndex(parent, name)
		if at < 0 {
			continue
		}
		key, value := parent.Content[at], parent.Content[at+1]
		if value.Kind == yaml.MappingNode && namesOwnArea(old) {
			continue
		}

		change := Move{Old: old, New: moves[old], Outcome: MoveDone}
		switch {
		case nodeShape(value) == "null":
			change.Outcome = MoveDroppedNull
		case findValueNode(under, change.New) != nil:
			change.Outcome = MoveDroppedNewSet
		}
		removeKey(under, strings.Split(old, "."))
		done = append(done, change)
		if change.Outcome != MoveDone {
			continue
		}

		target, newName, err := parentMappingOf(under, change.New)
		if err != nil {
			return nil, nil, fmt.Errorf("move %s to %s: %w", old, change.New, err)
		}
		// A flow mapping holding block entries and their comments marshals as invalid YAML, so it takes block style.
		target.Style &^= yaml.FlowStyle
		renamed := *key
		renamed.Value = newName
		target.Content = append(target.Content, &renamed, value)
	}

	if len(done) == 0 {
		return existing, nil, nil
	}
	slices.SortFunc(done, func(a, b Move) int { return cmp.Compare(a.Old, b.Old) })
	if len(root.Content) == 0 {
		return []byte{}, done, nil
	}
	inlineStrandedAliases(doc, map[string]*yaml.Node{})
	moved, err := yaml.Marshal(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal YAML: %w", err)
	}
	return moved, done, nil
}

// inlineStrandedAliases replaces, in document order under node, every alias whose anchored node is not yet written.
// anchors maps each anchor name written so far to the node it names.
// A replaced alias keeps its own comments and holds a copy of the anchored value.
func inlineStrandedAliases(node *yaml.Node, anchors map[string]*yaml.Node) {
	if node.Kind == yaml.AliasNode {
		if anchors[node.Value] != node.Alias {
			copied := copyWithoutAnchors(node.Alias)
			copied.HeadComment, copied.LineComment, copied.FootComment = node.HeadComment, node.LineComment, node.FootComment
			*node = *copied
		}
		return
	}
	if node.Anchor != "" {
		anchors[node.Anchor] = node
	}
	for _, child := range node.Content {
		inlineStrandedAliases(child, anchors)
	}
}

// copyWithoutAnchors returns a deep copy of node that declares no anchor and holds no alias.
func copyWithoutAnchors(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode {
		return copyWithoutAnchors(node.Alias)
	}
	copied := *node
	copied.Anchor = ""
	copied.Content = nil
	for _, child := range node.Content {
		copied.Content = append(copied.Content, copyWithoutAnchors(child))
	}
	return &copied
}
