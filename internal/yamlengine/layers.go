// layers.go implements the node-level operations over layered YAML: merge, strip, sparse set, prune, unit paths and unit annotation.
// Every operation works on raw unexpanded bytes and shares one notion of a unit, described in the package documentation.

package yamlengine

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// unitRule decides which values the layer operations carry and compare whole.
// A mapping key is descended into unless its path is a declared open map or, with wholeEntries, it is a top-level key.
// A scalar or a sequence is always one unit.
type unitRule struct {
	openMaps     []string
	wholeEntries bool
}

// whole reports whether the key at path, at the top level when topLevel, is one unit whatever its value.
func (rule unitRule) whole(path string, topLevel bool) bool {
	return slices.Contains(rule.openMaps, path) || (rule.wholeEntries && topLevel)
}

// descends reports whether the layer operations step into value, the value of the key at path.
func (rule unitRule) descends(value *yaml.Node, path string, topLevel bool) bool {
	return value.Kind == yaml.MappingNode && !rule.whole(path, topLevel)
}

// Merge lays upper over lower key by key at every mapping depth and returns the merged document.
// A unit upper holds replaces lower's, a unit only lower holds stays, and a mapping both hold merges recursively;
// no layer can delete a key from the one below.
// A unit is a mapping key descended into, except a key at a declared openMaps path and, when wholeEntries is true, every top-level key, each carried whole; a scalar or a sequence is always one unit.
// An empty or comment-only side yields the other side's bytes, and both empty yields empty bytes.
// Comments and key order come from lower, with upper's new keys appended in upper's order.
func Merge(lower, upper []byte, openMaps []string, wholeEntries bool) ([]byte, error) {
	lowerDoc, lowerRoot, err := parseMappingRoot(lower, "lower")
	if err != nil {
		return nil, err
	}
	_, upperRoot, err := parseMappingRoot(upper, "upper")
	if err != nil {
		return nil, err
	}
	switch {
	case upperRoot == nil && lowerRoot == nil:
		return []byte{}, nil
	case upperRoot == nil:
		return lower, nil
	case lowerRoot == nil:
		return upper, nil
	}

	unitRule{openMaps: openMaps, wholeEntries: wholeEntries}.mergeMapping(lowerRoot, upperRoot, "", true)

	merged, err := yaml.Marshal(lowerDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal merged YAML: %w", err)
	}
	return merged, nil
}

// mergeMapping lays the upper mapping over the lower one in place.
func (rule unitRule) mergeMapping(lower, upper *yaml.Node, path string, topLevel bool) {
	if len(lower.Content) == 0 {
		lower.Style = 0
	}
	for i := 0; i+1 < len(upper.Content); i += 2 {
		key, value := upper.Content[i], upper.Content[i+1]
		keyPath := joinKeyPath(path, key.Value)
		at := mappingIndex(lower, key.Value)
		if at < 0 {
			lower.Content = append(lower.Content, deepCopyNode(key), deepCopyNode(value))
			continue
		}
		lowerValue := lower.Content[at+1]
		if rule.descends(value, keyPath, topLevel) && lowerValue.Kind == yaml.MappingNode {
			rule.mergeMapping(lowerValue, value, keyPath, false)
			continue
		}
		setEntry(lower, key.Value, deepCopyNode(value))
	}
}

// Strip removes from upper every unit whose node equals lower's node at the same path, and returns the stripped bytes and the sorted dotted paths it removed.
// Units follow the same rule as in Merge: a key at a declared openMaps path and, when wholeEntries is true, every top-level key is compared whole.
// Equality is over the unexpanded nodes: same kind, same scalar tag and value, same children, comments ignored;
// so a ${env:...} marker equals only the same marker text, in either direction.
// A mapping left empty by the strip is removed with its key, and an upper left with nothing returns empty bytes.
// When nothing is removed, upper is returned byte for byte.
func Strip(lower, upper []byte, openMaps []string, wholeEntries bool) ([]byte, []string, error) {
	_, lowerRoot, err := parseMappingRoot(lower, "lower")
	if err != nil {
		return nil, nil, err
	}
	upperDoc, upperRoot, err := parseMappingRoot(upper, "upper")
	if err != nil {
		return nil, nil, err
	}
	removed := []string{}
	if lowerRoot == nil || upperRoot == nil {
		return upper, removed, nil
	}

	unitRule{openMaps: openMaps, wholeEntries: wholeEntries}.stripMapping(lowerRoot, upperRoot, "", true, &removed)
	if len(removed) == 0 {
		return upper, removed, nil
	}
	sort.Strings(removed)
	if len(upperRoot.Content) == 0 {
		return []byte{}, removed, nil
	}

	stripped, err := yaml.Marshal(upperDoc)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal stripped YAML: %w", err)
	}
	return stripped, removed, nil
}

// stripMapping removes from the upper mapping, in place, every unit equal to lower's, recording each removed unit's path.
func (rule unitRule) stripMapping(lower, upper *yaml.Node, path string, topLevel bool, removed *[]string) {
	kept := make([]*yaml.Node, 0, len(upper.Content))
	for i := 0; i+1 < len(upper.Content); i += 2 {
		key, value := upper.Content[i], upper.Content[i+1]
		keyPath := joinKeyPath(path, key.Value)
		if at := mappingIndex(lower, key.Value); at >= 0 {
			lowerValue := lower.Content[at+1]
			if rule.descends(value, keyPath, topLevel) && lowerValue.Kind == yaml.MappingNode {
				rule.stripMapping(lowerValue, value, keyPath, false, removed)
				if len(value.Content) == 0 {
					continue
				}
			} else if nodesEqual(lowerValue, value) {
				*removed = append(*removed, keyPath)
				continue
			}
		}
		kept = append(kept, key, value)
	}
	upper.Content = kept
}

// SetSparse applies pairs to existing, an empty mapping when existing is empty or comment-only, and writes only the set keys, creating intermediate mappings.
// It never scaffolds from the template, which only decides which keys are known.
// A key is known when the template holds it as a leaf or a list, or lies under a declared openMaps path P as P.<name>;
// a list key takes a YAML flow list such as [] or [a, b], and an open-map entry is a scalar in the mapping at P.
// A key at P itself, when P holds a scalar or a list, takes either: a value starting with [ is written as a flow list of scalars, any other value as a string, and a bracketed value that is not such a list is an error.
// Unknown lists the requested keys that are not known, Known the sorted known set, and Merged is nil when Unknown is non-empty.
// A scalar leaf outside a scalar-or-list open-map path takes its template leaf's tag, except that a value spelling a null (`~`, `null`, `Null`, `NULL`), or a template leaf holding a null, gives it the tag its own text resolves to.
// Preserved stays empty.
func SetSparse(template, existing []byte, pairs []KV, openMaps ...string) (SetResult, error) {
	var templateNode yaml.Node
	if err := yaml.Unmarshal(template, &templateNode); err != nil {
		return SetResult{}, err
	}
	scalarOrListPaths := scalarOrListOpenMapPaths(&templateNode, openMaps)
	existingDoc, existingRoot, err := parseMappingRoot(existing, "existing")
	if err != nil {
		return SetResult{}, err
	}
	if existingRoot != nil {
		carryOpenMaps(&templateNode, existingDoc, openMaps)
	}

	templateLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&templateNode, templateLeaves)
	templateSequences := make(map[string]*yaml.Node)
	collectSequencePaths(&templateNode, "", templateSequences)

	known := knownKeys(templateLeaves, templateSequences, openMaps)
	if unknown := unknownKeys(pairs, templateLeaves, templateSequences, openMaps); len(unknown) > 0 {
		return SetResult{Unknown: unknown, Known: known}, nil
	}

	if existingRoot == nil {
		existingRoot = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		existingDoc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{existingRoot}}
	}
	for _, pair := range pairs {
		if open, name, ok := splitOpenMapKey(pair.Key, openMaps); ok {
			mapping, err := mappingAt(existingRoot, open)
			if err != nil {
				return SetResult{}, fmt.Errorf("%w: rewrite %s as a map of name to description", err, open)
			}
			setEntry(mapping, name, stringNode(pair.Value))
			continue
		}
		parentPath, name := splitLastSegment(pair.Key)
		parent := existingRoot
		if parentPath != "" {
			if parent, err = mappingAt(existingRoot, parentPath); err != nil {
				return SetResult{}, err
			}
		}
		if slices.Contains(scalarOrListPaths, pair.Key) {
			value, err := scalarOrListNode(pair)
			if err != nil {
				return SetResult{}, err
			}
			setEntry(parent, name, value)
			continue
		}
		if _, ok := templateSequences[pair.Key]; ok {
			sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			if err := replaceSequence(sequence, pair); err != nil {
				return SetResult{}, err
			}
			setEntry(parent, name, sequence)
			continue
		}
		setEntry(parent, name, &yaml.Node{Kind: yaml.ScalarNode, Tag: scalarSetTag(templateLeaves[pair.Key].Tag, pair.Value), Value: pair.Value})
	}

	merged, err := yaml.Marshal(existingDoc)
	if err != nil {
		return SetResult{}, err
	}
	return SetResult{Merged: merged, Known: known}, nil
}

// knownKeys returns the sorted key set a template makes settable: its leaf paths, its list-valued key paths and one P.<name> per open map P.
func knownKeys(templateLeaves, templateSequences map[string]*yaml.Node, openMaps []string) []string {
	known := make([]string, 0, len(templateLeaves)+len(templateSequences)+len(openMaps))
	for path := range templateLeaves {
		known = append(known, path)
	}
	for path := range templateSequences {
		known = append(known, path)
	}
	for _, p := range openMaps {
		known = append(known, p+".<name>")
	}
	sort.Strings(known)
	return known
}

// unknownKeys returns the sorted, deduplicated pair keys the template neither holds nor opens.
func unknownKeys(pairs []KV, templateLeaves, templateSequences map[string]*yaml.Node, openMaps []string) []string {
	var unknown []string
	for _, pair := range pairs {
		_, isLeaf := templateLeaves[pair.Key]
		_, isSequence := templateSequences[pair.Key]
		_, _, isOpen := splitOpenMapKey(pair.Key, openMaps)
		if !isLeaf && !isSequence && !isOpen {
			unknown = append(unknown, pair.Key)
		}
	}
	sort.Strings(unknown)
	return slices.Compact(unknown)
}

// splitLastSegment splits a dotted path into its parent path and last key.
func splitLastSegment(path string) (parent, name string) {
	if at := strings.LastIndex(path, "."); at >= 0 {
		return path[:at], path[at+1:]
	}
	return "", path
}

// mappingAt returns the mapping at the dotted path under root, creating missing mappings on the way.
// A null value becomes an empty mapping; any other non-mapping value is an error naming its path.
func mappingAt(root *yaml.Node, path string) (*yaml.Node, error) {
	node := root
	walked := ""
	for _, segment := range strings.Split(path, ".") {
		walked = joinKeyPath(walked, segment)
		at := mappingIndex(node, segment)
		if at < 0 {
			node.Content = append(node.Content, stringNode(segment), &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			at = len(node.Content) - 2
		}
		value := node.Content[at+1]
		if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
			*value = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s holds a %s, not a map", walked, nodeShape(value))
		}
		if len(value.Content) == 0 {
			value.Style = 0
		}
		node = value
	}
	return node, nil
}

// setEntry sets name to value in the mapping, replacing an existing entry's value and keeping its comments, or appending the key.
func setEntry(mapping *yaml.Node, name string, value *yaml.Node) {
	at := mappingIndex(mapping, name)
	if at < 0 {
		mapping.Content = append(mapping.Content, stringNode(name), value)
		return
	}
	old := mapping.Content[at+1]
	value.HeadComment, value.LineComment, value.FootComment = old.HeadComment, old.LineComment, old.FootComment
	mapping.Content[at+1] = value
}

// Prune removes from a sparse existing every unit the template does not know, and returns the pruned bytes and the sorted removed paths.
// A key at a declared openMaps path is known whole, so everything under it stays.
// It never adds a key, so it keeps a sparse layer sparse where a full reconcile would scaffold from the template.
// A mapping left empty is removed with its key, and an existing left with nothing returns empty bytes.
// When nothing is removed, existing is returned byte for byte.
func Prune(template, existing []byte, openMaps ...string) ([]byte, []string, error) {
	_, templateRoot, err := parseMappingRoot(template, "template")
	if err != nil {
		return nil, nil, err
	}
	existingDoc, existingRoot, err := parseMappingRoot(existing, "existing")
	if err != nil {
		return nil, nil, err
	}
	removed := []string{}
	if existingRoot == nil {
		return existing, removed, nil
	}

	unitRule{openMaps: openMaps}.pruneMapping(templateRoot, existingRoot, "", &removed)
	if len(removed) == 0 {
		return existing, removed, nil
	}
	sort.Strings(removed)
	if len(existingRoot.Content) == 0 {
		return []byte{}, removed, nil
	}

	pruned, err := yaml.Marshal(existingDoc)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal pruned YAML: %w", err)
	}
	return pruned, removed, nil
}

// pruneMapping removes from the existing mapping, in place, every unit the template mapping does not know, recording each removed unit's path.
// template is nil where the template holds no mapping.
func (rule unitRule) pruneMapping(template, existing *yaml.Node, path string, removed *[]string) {
	kept := make([]*yaml.Node, 0, len(existing.Content))
	for i := 0; i+1 < len(existing.Content); i += 2 {
		key, value := existing.Content[i], existing.Content[i+1]
		keyPath := joinKeyPath(path, key.Value)
		if !rule.whole(keyPath, false) {
			var templateValue *yaml.Node
			if template != nil {
				if at := mappingIndex(template, key.Value); at >= 0 {
					templateValue = template.Content[at+1]
				}
			}
			if templateValue == nil {
				*removed = append(*removed, rule.unitPaths(value, keyPath, false)...)
				continue
			}
			if value.Kind == yaml.MappingNode && templateValue.Kind == yaml.MappingNode {
				rule.pruneMapping(templateValue, value, keyPath, removed)
				if len(value.Content) == 0 {
					continue
				}
			}
		}
		kept = append(kept, key, value)
	}
	existing.Content = kept
}

// LeafPaths returns the sorted dotted paths of every unit in doc.
// A key at a declared openMaps path and, when wholeEntries is true, every top-level key is one unit; an empty mapping counts as one unit too.
func LeafPaths(doc []byte, openMaps []string, wholeEntries bool) ([]string, error) {
	_, root, err := parseMappingRoot(doc, "doc")
	if err != nil {
		return nil, err
	}
	paths := []string{}
	if root != nil {
		paths = unitRule{openMaps: openMaps, wholeEntries: wholeEntries}.mappingUnitPaths(root, "", true)
	}
	sort.Strings(paths)
	return paths, nil
}

// unitPaths returns the paths of the units in value, the value of the key at path.
func (rule unitRule) unitPaths(value *yaml.Node, path string, topLevel bool) []string {
	if rule.descends(value, path, topLevel) && len(value.Content) > 0 {
		return rule.mappingUnitPaths(value, path, false)
	}
	return []string{path}
}

// mappingUnitPaths returns the paths of the units under the mapping at path.
func (rule unitRule) mappingUnitPaths(mapping *yaml.Node, path string, topLevel bool) []string {
	var paths []string
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyPath := joinKeyPath(path, mapping.Content[i].Value)
		paths = append(paths, rule.unitPaths(mapping.Content[i+1], keyPath, topLevel)...)
	}
	return paths
}

// AnnotateLeaves writes each notes[path] as the line comment of the unit at path and returns the re-marshalled document.
// Units are those LeafPaths returns; a path doc lacks, or that is not a unit, is ignored.
// A note without a leading # gets "# " in front.
// An empty or comment-only doc is returned as it is.
func AnnotateLeaves(doc []byte, notes map[string]string, openMaps []string, wholeEntries bool) ([]byte, error) {
	parsed, root, err := parseMappingRoot(doc, "doc")
	if err != nil {
		return nil, err
	}
	if root == nil {
		return doc, nil
	}

	unitRule{openMaps: openMaps, wholeEntries: wholeEntries}.annotateMapping(root, "", true, notes)

	annotated, err := yaml.Marshal(parsed)
	if err != nil {
		return nil, fmt.Errorf("marshal annotated YAML: %w", err)
	}
	return annotated, nil
}

// annotateMapping sets the line comment of every unit of the mapping that notes names.
// A mapping unit carries its comment on the key, a scalar or sequence unit on its value.
func (rule unitRule) annotateMapping(mapping *yaml.Node, path string, topLevel bool, notes map[string]string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i], mapping.Content[i+1]
		keyPath := joinKeyPath(path, key.Value)
		if rule.descends(value, keyPath, topLevel) && len(value.Content) > 0 {
			rule.annotateMapping(value, keyPath, false, notes)
			continue
		}
		note, ok := notes[keyPath]
		if !ok {
			continue
		}
		if !strings.HasPrefix(note, "#") {
			note = "# " + note
		}
		if value.Kind == yaml.MappingNode {
			key.LineComment = note
		} else {
			value.LineComment = note
		}
	}
}

// parseMappingRoot parses data and returns the document and its root mapping.
// The root is nil for empty or comment-only data; a root that is not a mapping is an error naming what.
func parseMappingRoot(data []byte, what string) (doc, root *yaml.Node, err error) {
	doc = &yaml.Node{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return doc, nil, nil
	}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s YAML: %w", what, err)
	}
	root = documentRoot(doc)
	if root != nil && root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s YAML: top level is a %s, not a mapping", what, nodeShape(root))
	}
	return doc, root, nil
}

// mappingIndex returns the index of key's key node in the mapping's content, or -1.
func mappingIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// nodesEqual reports whether two unexpanded nodes have the same kind, scalar tag and value and children, comments and mapping key order ignored.
func nodesEqual(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.ScalarNode:
		return a.ShortTag() == b.ShortTag() && a.Value == b.Value
	case yaml.SequenceNode:
		return slices.EqualFunc(a.Content, b.Content, nodesEqual)
	case yaml.MappingNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := 0; i+1 < len(a.Content); i += 2 {
			at := mappingIndex(b, a.Content[i].Value)
			if at < 0 || !nodesEqual(a.Content[i+1], b.Content[at+1]) {
				return false
			}
		}
		return true
	case yaml.AliasNode:
		return nodesEqual(a.Alias, b.Alias)
	}
	return false
}
