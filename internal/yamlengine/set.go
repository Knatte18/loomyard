// set.go implements value-preserving single/multi-key YAML mutation for the non-interactive `lym config <module> --set key=value` path.
// Unlike Reconcile (which merges an entire existing file into a template), SetValues applies a small, explicit list of key=value pairs while still routing every write through the template-shaped working tree so partial/stale existing files never hide a valid key behind a missing node.
// It also grafts any existing top-level key absent from the template onto the working tree whole, at root-key granularity, so a hand-edited or template-outgrown key is carried through into the merged output instead of silently vanishing.

package yamlengine

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// preservedKeyComment marks a root-level key grafted from existing because
// the template no longer declares it. Direct assignment keeps --set calls
// idempotent.
const preservedKeyComment = "# preserved (not in current template)"

// KV is a single key=value pair.
// Key is a dotted leaf key-path, or the dotted path of a list-valued key;
// Value is the scalar value, or for a list key a YAML flow list such as [] or [a, b].
type KV struct {
	Key   string
	Value string
}

// SetResult is the outcome of a SetValues call.
// Merged holds the new file bytes and is only valid when Unknown is empty.
// Unknown lists requested keys absent from the template's leaf-key set.
// Known is the template's full sorted leaf-key set.
// Preserved lists pre-existing top-level keys carried through (nil/empty when none).
type SetResult struct {
	Merged    []byte
	Unknown   []string
	Known     []string
	Preserved []string
}

// SetValues applies pairs to a template-shaped YAML document, preserving comments and key order.
// If any pairs[i].Key is absent from the template's leaf-key and list-key sets, SetResult.Unknown is
// returned non-empty and Merged is nil.
// Existing lists are carried whole, never merged element by element, and a list key's value
// replaces its list whole; a value that is not a YAML list is an error.
// Otherwise every pair is applied to the working tree (later pairs for a repeated key win) and the
// mutated tree is marshalled into SetResult.Merged.
// A scalar leaf outside a scalar-or-list open-map path keeps its tag, except that a value spelling a null (`~`, `null`, `Null`, `NULL`), or a leaf holding a null, gives the scalar the tag its own text resolves to.
//
// Each openMaps entry is a dotted key path whose value is an open map:
// any key P.<name> is known,
// existing's value at P is carried whole,
// and the pair sets <name> to the scalar value, appending the key when absent.
// A pair under an open map that holds a list is an error naming the path;
// the list is never converted.
// A pair at P itself, when P holds a scalar or a list, replaces P whole with the shape the value spells:
// a value starting with [ is a YAML flow list of scalars or mappings of scalars, and any other value a string;
// a bracketed value that is not such a list is an error.
// Known lists each open map as P.<name>.
//
// Each perNameMaps entry is a dotted key path whose template value is a mapping of names:
// P.<name> is known for any dot-free name, every name existing holds under P and the template lacks is carried onto the mapping, and the pair sets the name, appending it when absent.
// Once the pairs apply, every name under P holding the template's value for it is left out of Merged, a name a pair set included.
// Under a mapping of scalars the value is a string whatever its first character;
// under a mapping of lists it is a YAML flow list, and a value that is no list is an error;
// under a mapping of mappings it is a YAML flow mapping, and a value that is no flow mapping is an error.
// A per-name map of mappings also knows P.<name>.<rest> for any name, where rest is a scalar or list path some template entry holds:
// the pair sets that one key in the name's mapping, creating it, under the template entry's tag or as a flow list.
// A pair under P whose existing value at P is neither a mapping nor null is an error naming P and its shape.
func SetValues(template, existing []byte, pairs []KV, perNameMaps []string, openMaps ...string) (SetResult, error) {
	// Parse the template into the tree we will mutate and ultimately marshal.
	var templateNode yaml.Node
	if err := yaml.Unmarshal(template, &templateNode); err != nil {
		return SetResult{}, err
	}

	// The pristine template decides the per-name entry shapes and, once the pairs apply, which names hold the template's value;
	// templateNode itself takes existing's values and names below.
	var pristine yaml.Node
	if err := yaml.Unmarshal(template, &pristine); err != nil {
		return SetResult{}, err
	}
	scalarOrListPaths := scalarOrListOpenMapPaths(&templateNode, openMaps)
	entryKinds := perNameEntryKinds(&pristine, perNameMaps)
	entryLeaves := perNameEntryLeaves(&pristine, perNameMaps, entryKinds)

	var existingNode yaml.Node
	if len(existing) > 0 {
		if err := yaml.Unmarshal(existing, &existingNode); err != nil {
			return SetResult{}, err
		}
		carryOpenMaps(&templateNode, &existingNode, openMaps)
		carryPerNameEntries(&templateNode, &existingNode, perNameMaps)
	}

	templateLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&templateNode, templateLeaves)

	templateSequences := make(map[string]*yaml.Node)
	collectSequencePaths(&templateNode, "", templateSequences)

	known := knownKeys(templateLeaves, templateSequences, openMaps, perNameMaps, entryLeaves)

	// preserved collects the root-key-granularity graft step's key names
	// (below), reported to the caller via SetResult.Preserved. It stays nil
	// when existing is empty, since there is nothing on disk to preserve.
	var preserved []string

	// Layer existing's values onto the template working tree via the same
	// applyExistingOverrides helper Reconcile uses, so a --set call preserves
	// whatever the user already customized rather than resetting untouched
	// keys back to defaults.
	if len(existing) > 0 {
		existingLeaves := make(map[string]*yaml.Node)
		collectLeafPaths(&existingNode, existingLeaves)

		applyExistingOverrides(templateLeaves, existingLeaves)
		existingSequences := make(map[string]*yaml.Node)
		collectSequencePaths(&existingNode, "", existingSequences)
		applyExistingSequences(templateSequences, existingSequences)

		// Graft any of existing's top-level keys with no counterpart in the
		// template onto templateNode's root mapping, whole. This is a
		// root-key-granularity operation independent of the leaf-path
		// override above: a key the template has outgrown, or one the user
		// hand-added, must never vanish just because SetValues always
		// marshals from templateNode rather than existingNode.
		preserved = preserveOrphanRootKeys(&templateNode, &existingNode)
	}

	// Validate every requested key against the template's leaf set before
	// mutating anything, so a single unknown key rejects the whole call
	// rather than silently applying a partial write.
	if unknown := unknownKeys(pairs, templateLeaves, templateSequences, openMaps, perNameMaps, entryLeaves); len(unknown) > 0 {
		return SetResult{Unknown: unknown, Known: known}, nil
	}

	// Every key but a per-name entry key is now guaranteed to have a real node in templateNode, since
	// the working tree always contains every template leaf. Apply pairs in
	// order so a repeated key's later value wins.
	for _, pair := range pairs {
		if slices.Contains(scalarOrListPaths, pair.Key) {
			value, err := scalarOrListNode(pair)
			if err != nil {
				return SetResult{}, err
			}
			current := findValueNode(&templateNode, pair.Key)
			value.HeadComment, value.LineComment, value.FootComment = current.HeadComment, current.LineComment, current.FootComment
			*current = *value
			continue
		}
		if open, name, ok := splitOpenMapKey(pair.Key, openMaps); ok {
			if err := setOpenMapEntry(&templateNode, open, name, pair); err != nil {
				return SetResult{}, err
			}
			continue
		}
		if perName, name, ok := splitPerNameKey(pair.Key, perNameMaps); ok {
			if err := perNameShapeError(&existingNode, perName); err != nil {
				return SetResult{}, err
			}
			if err := setPerNameEntry(&templateNode, perName, name, pair, entryKinds[perName]); err != nil {
				return SetResult{}, err
			}
			continue
		}
		_, isLeaf := templateLeaves[pair.Key]
		_, isSequence := templateSequences[pair.Key]
		if perName, entryLeaf, ok := splitPerNameEntryKey(pair.Key, perNameMaps, entryLeaves); ok && !isLeaf && !isSequence {
			if err := perNameShapeError(&existingNode, perName); err != nil {
				return SetResult{}, err
			}
			if err := setPerNameEntryLeaf(documentRoot(&templateNode), pair, entryLeaf); err != nil {
				return SetResult{}, err
			}
			continue
		}
		if sequence, ok := templateSequences[pair.Key]; ok {
			if err := replaceSequence(sequence, pair); err != nil {
				return SetResult{}, err
			}
			continue
		}
		leaf := templateLeaves[pair.Key]
		if tag := scalarSetTag(leaf.Tag, pair.Value); tag != leaf.Tag {
			leaf.Tag, leaf.Style = tag, 0
		}
		leaf.Value = pair.Value
	}

	dropTemplateNames(&templateNode, &pristine, perNameMaps)
	liftEmptyValueComments(&templateNode)

	merged, err := yaml.Marshal(&templateNode)
	if err != nil {
		return SetResult{}, err
	}

	return SetResult{Merged: merged, Known: known, Preserved: preserved}, nil
}

// scalarSetTag returns the tag a scalar leaf holding value is written under when a set lands on a leaf tagged leafTag.
// A value that spells a null (`~`, `null`, `Null`, `NULL`) or a leaf tagged `!!null` takes the tag its own text resolves to, so a set to or over a null leaf reads back as written.
// Every other set keeps leafTag, and an empty value is not a spelled null.
func scalarSetTag(leafTag, value string) string {
	switch value {
	case "~", "null", "Null", "NULL":
		return "!!null"
	}
	if leafTag != "!!null" || value == "" {
		return leafTag
	}
	var parsed yaml.Node
	if err := yaml.Unmarshal([]byte(value), &parsed); err != nil || len(parsed.Content) != 1 || parsed.Content[0].Kind != yaml.ScalarNode {
		return "!!str"
	}
	return parsed.Content[0].ShortTag()
}

// splitOpenMapKey splits key into a declared open-map path and the entry name under it.
func splitOpenMapKey(key string, openMaps []string) (open, name string, ok bool) {
	for _, p := range openMaps {
		if rest, found := strings.CutPrefix(key, p+"."); found && rest != "" {
			return p, rest, true
		}
	}
	return "", "", false
}

// openMapEntryNode returns the node pair.Value spells for an entry of an open map:
// a value whose first non-blank character is { is a YAML flow mapping, one whose first non-blank character is [ is a YAML flow list, and any other value is a string scalar.
// The parsed node keeps its own scalar tags.
// A braced or bracketed value that does not parse as one node of its kind, or carries text after its closing bracket, is an error naming the key and the quoting rule.
func openMapEntryNode(pair KV) (*yaml.Node, error) {
	trimmed := strings.TrimSpace(pair.Value)
	var kind yaml.Kind
	var closing string
	switch {
	case strings.HasPrefix(trimmed, "{"):
		kind, closing = yaml.MappingNode, "}"
	case strings.HasPrefix(trimmed, "["):
		kind, closing = yaml.SequenceNode, "]"
	default:
		return stringNode(pair.Value), nil
	}
	refusal := fmt.Errorf("value for %s is not one YAML flow mapping or list, got %q: inside it, double-quote a string that holds a brace, comma or bracket", pair.Key, pair.Value)
	var parsed yaml.Node
	if err := yaml.Unmarshal([]byte(trimmed), &parsed); err != nil || len(parsed.Content) != 1 || parsed.Content[0].Kind != kind || !strings.HasSuffix(trimmed, closing) {
		return nil, refusal
	}
	entry := parsed.Content[0]
	if kind == yaml.MappingNode {
		entry.Style = 0
	}
	return entry, nil
}

// setOpenMapEntry sets pair's name under the open map at path open to the entry pair.Value spells, appending the key when absent.
// A null value becomes an empty mapping first;
// a list or other shape is an error, never converted.
// The mapping is given block style,
// so an empty {} template value is written as a block.
func setOpenMapEntry(root *yaml.Node, open, name string, pair KV) error {
	entry, err := openMapEntryNode(pair)
	if err != nil {
		return err
	}
	mapping := findValueNode(root, open)
	if mapping == nil {
		return fmt.Errorf("open map %s is not in the template", open)
	}
	if mapping.Kind == yaml.ScalarNode && mapping.Tag == "!!null" {
		*mapping = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("%s holds a %s, not a map: rewrite %s as a map of name to description", open, nodeShape(mapping), open)
	}
	mapping.Style = 0
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			mapping.Content[i+1] = entry
			return nil
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, entry)
	return nil
}

// splitPerNameKey splits key into a declared per-name map path and the one dot-free name under it.
func splitPerNameKey(key string, perNameMaps []string) (perName, name string, ok bool) {
	for _, p := range perNameMaps {
		if rest, found := strings.CutPrefix(key, p+"."); found && rest != "" && !strings.Contains(rest, ".") {
			return p, rest, true
		}
	}
	return "", "", false
}

// perNameEntryKinds returns the kind of the template's entries at each declared per-name map path that holds one.
// It reads the first entry, since a per-name map holds one kind of entry;
// a path the result lacks has scalar entries.
func perNameEntryKinds(template *yaml.Node, perNameMaps []string) map[string]yaml.Kind {
	kinds := make(map[string]yaml.Kind, len(perNameMaps))
	for _, path := range perNameMaps {
		value := findValueNode(template, path)
		if value != nil && value.Kind == yaml.MappingNode && len(value.Content) > 1 {
			kinds[path] = value.Content[1].Kind
		}
	}
	return kinds
}

// perNameEntryLeaves returns, for each declared per-name map path whose template entries are mappings, the scalar and list paths those entries hold, relative to an entry, each mapped to the node of the first entry holding it.
// A key P.<name>.<rest> is written after the node at rest, whatever the name.
func perNameEntryLeaves(template *yaml.Node, perNameMaps []string, entryKinds map[string]yaml.Kind) map[string]map[string]*yaml.Node {
	leaves := make(map[string]map[string]*yaml.Node)
	for _, path := range perNameMaps {
		if entryKinds[path] != yaml.MappingNode {
			continue
		}
		entries := findValueNode(template, path)
		leaves[path] = make(map[string]*yaml.Node)
		for i := 1; i < len(entries.Content); i += 2 {
			entryLeaves := make(map[string]*yaml.Node)
			collectLeafPaths(entries.Content[i], entryLeaves)
			collectSequencePaths(entries.Content[i], "", entryLeaves)
			for rest, node := range entryLeaves {
				if _, seen := leaves[path][rest]; !seen && !strings.Contains(rest, "[") {
					leaves[path][rest] = node
				}
			}
		}
	}
	return leaves
}

// splitPerNameEntryKey splits a key P.<name>.<rest> into the declared per-name map P and the template node the key is written after, for a P whose entries are mappings and a rest some template entry holds as a scalar or a list.
// ok is false for any other key.
func splitPerNameEntryKey(key string, perNameMaps []string, entryLeaves map[string]map[string]*yaml.Node) (perName string, leaf *yaml.Node, ok bool) {
	for _, path := range perNameMaps {
		rest, found := strings.CutPrefix(key, path+".")
		if !found {
			continue
		}
		if _, rest, found = strings.Cut(rest, "."); !found {
			continue
		}
		if leaf, ok := entryLeaves[path][rest]; ok {
			return path, leaf, true
		}
	}
	return "", nil, false
}

// setPerNameEntryLeaf writes pair, a key P.<name>.<rest>, into the mapping at P.<name> under root, creating every missing mapping on the way, after leaf, the template entry's node at rest:
// a YAML flow list when leaf is a list, else a scalar under the tag a set over leaf takes.
// A non-mapping value on the way is an error naming its path.
func setPerNameEntryLeaf(root *yaml.Node, pair KV, leaf *yaml.Node) error {
	parentPath, name := splitLastSegment(pair.Key)
	parent, err := mappingAt(root, parentPath)
	if err != nil {
		return err
	}
	if leaf.Kind == yaml.SequenceNode {
		sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if err := replaceSequence(sequence, pair); err != nil {
			return err
		}
		setEntry(parent, name, sequence)
		return nil
	}
	setEntry(parent, name, &yaml.Node{Kind: yaml.ScalarNode, Tag: scalarSetTag(leaf.Tag, pair.Value), Value: pair.Value})
	return nil
}

// perNameShapeError returns an error naming path and its shape when root holds there a value that is neither a mapping nor null, and nil otherwise.
func perNameShapeError(root *yaml.Node, path string) error {
	value := findValueNode(root, path)
	if value == nil || value.Kind == yaml.MappingNode || nodeShape(value) == "null" {
		return nil
	}
	return fmt.Errorf("%s holds a %s, not a map: rewrite %s as a map of name to value", path, nodeShape(value), path)
}

// perNameEntryNode returns the node pair.Value spells for an entry of a per-name map whose entries are of kind entryKind:
// a YAML flow list when they are lists, a YAML flow mapping when they are mappings, else a string scalar whatever its first character.
// A value that is not a flow mapping, for mapping entries, is an error naming the key.
func perNameEntryNode(pair KV, entryKind yaml.Kind) (*yaml.Node, error) {
	switch entryKind {
	case yaml.SequenceNode:
		sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if err := replaceSequence(sequence, pair); err != nil {
			return nil, err
		}
		return sequence, nil
	case yaml.MappingNode:
		entry, err := openMapEntryNode(pair)
		if err != nil {
			return nil, err
		}
		if entry.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("value for %s is not a YAML flow mapping, got %q: write the entry as {key: value, ...}, or set one of its keys as %s.<key>", pair.Key, pair.Value, pair.Key)
		}
		return entry, nil
	default:
		return stringNode(pair.Value), nil
	}
}

// setPerNameEntry sets name in the per-name map at path perName to the entry pair spells, appending the key when absent.
// A null value at the path becomes an empty mapping first.
// The mapping is given block style, so an empty {} template value is written as a block.
func setPerNameEntry(root *yaml.Node, perName, name string, pair KV, entryKind yaml.Kind) error {
	mapping := findValueNode(root, perName)
	if mapping == nil {
		return fmt.Errorf("per-name map %s is not in the template", perName)
	}
	if mapping.Kind == yaml.ScalarNode && mapping.Tag == "!!null" {
		*mapping = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("%s holds a %s, not a map: rewrite %s as a map of name to value", perName, nodeShape(mapping), perName)
	}
	value, err := perNameEntryNode(pair, entryKind)
	if err != nil {
		return err
	}
	mapping.Style = 0
	setEntry(mapping, name, value)
	return nil
}

// carryPerNameEntries appends to the template's mapping at each declared per-name map path every name existing holds there and the template lacks.
// A path where either tree holds no mapping is skipped.
func carryPerNameEntries(templateNode, existingNode *yaml.Node, perNameMaps []string) {
	for _, p := range perNameMaps {
		templateValue := findValueNode(templateNode, p)
		existingValue := findValueNode(existingNode, p)
		if templateValue == nil || existingValue == nil || templateValue.Kind != yaml.MappingNode || existingValue.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(existingValue.Content); i += 2 {
			if mappingIndex(templateValue, existingValue.Content[i].Value) < 0 {
				templateValue.Style = 0
				templateValue.Content = append(templateValue.Content, existingValue.Content[i], existingValue.Content[i+1])
			}
		}
	}
}

// dropTemplateNames removes from tree's mapping at each declared per-name map path every name holding the value template holds for it,
// so the written file keeps only the names that differ and the load resolves the rest from the template.
// A path where either tree holds no mapping is skipped.
func dropTemplateNames(tree, template *yaml.Node, perNameMaps []string) {
	for _, p := range perNameMaps {
		mapping := findValueNode(tree, p)
		templateValue := findValueNode(template, p)
		if mapping == nil || templateValue == nil || mapping.Kind != yaml.MappingNode || templateValue.Kind != yaml.MappingNode {
			continue
		}
		var content []*yaml.Node
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			at := mappingIndex(templateValue, mapping.Content[i].Value)
			if at >= 0 && sameValue(mapping.Content[i+1], templateValue.Content[at+1]) {
				continue
			}
			content = append(content, mapping.Content[i], mapping.Content[i+1])
		}
		mapping.Content = content
	}
}

// stringNode returns a !!str scalar node holding value.
func stringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// scalarOrListOpenMapPaths returns the declared open-map paths whose value in the template is a scalar or a list rather than a mapping.
// It reads the template as declared, so it must run before carryOpenMaps replaces those values with the file's.
// A key at such a path takes whichever of the two shapes its set value spells.
func scalarOrListOpenMapPaths(template *yaml.Node, openMaps []string) []string {
	var paths []string
	for _, path := range openMaps {
		if value := findValueNode(template, path); value != nil && value.Kind != yaml.MappingNode {
			paths = append(paths, path)
		}
	}
	return paths
}

// scalarOrListNode returns the node pair.Value spells for a key that takes a scalar or a list:
// a value whose first non-blank character is [ is a YAML flow list whose entries are scalars or mappings of scalars, and any other value is a string scalar.
// A bracketed value that does not parse as such a list, or carries text after the closing bracket, is an error naming the key and the quoted-entry form, never a string.
func scalarOrListNode(pair KV) (*yaml.Node, error) {
	if !strings.HasPrefix(strings.TrimSpace(pair.Value), "[") {
		return stringNode(pair.Value), nil
	}
	refusal := fmt.Errorf("value for %s is neither one value nor a YAML flow list of scalars or mappings of scalars, got %q: quote each entry that holds [ or ], e.g. [\"opus[medium]\", \"sonnet[high]\"]", pair.Key, pair.Value)
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if err := replaceSequence(sequence, pair); err != nil || !strings.HasSuffix(strings.TrimSpace(pair.Value), "]") {
		return nil, refusal
	}
	for _, entry := range sequence.Content {
		if !isScalarOrScalarMapping(entry) {
			return nil, refusal
		}
	}
	return sequence, nil
}

// isScalarOrScalarMapping reports whether node is a scalar or a mapping whose values are all scalars.
func isScalarOrScalarMapping(node *yaml.Node) bool {
	switch node.Kind {
	case yaml.ScalarNode:
		return true
	case yaml.MappingNode:
		for i := 1; i < len(node.Content); i += 2 {
			if node.Content[i].Kind != yaml.ScalarNode {
				return false
			}
		}
		return true
	}
	return false
}

// replaceSequence replaces sequence's elements with pair.Value parsed as a YAML list, so a list key
// can be set whole, including to the empty list.
func replaceSequence(sequence *yaml.Node, pair KV) error {
	var parsed yaml.Node
	if err := yaml.Unmarshal([]byte(pair.Value), &parsed); err != nil {
		return fmt.Errorf("value for list key %s: %w", pair.Key, err)
	}
	if len(parsed.Content) != 1 || parsed.Content[0].Kind != yaml.SequenceNode {
		return fmt.Errorf("value for list key %s must be a YAML list such as [] or [a, b], got %q", pair.Key, pair.Value)
	}
	sequence.Content = parsed.Content[0].Content
	sequence.Style = yaml.FlowStyle
	return nil
}

// preserveOrphanRootKeys grafts every top-level key in existingNode's root
// mapping that has no counterpart in templateNode's root mapping onto
// templateNode, in sorted key order, marking each with preservedKeyComment.
// It returns the sorted list of grafted key names (nil when none).
func preserveOrphanRootKeys(templateNode, existingNode *yaml.Node) []string {
	templateRoot := rootMappingNode(templateNode)
	existingRoot := rootMappingNode(existingNode)
	if templateRoot == nil || existingRoot == nil {
		return nil
	}

	// Build the template's top-level key-name set to test existing's
	// top-level keys against.
	templateKeys := make(map[string]bool)
	for i := 0; i+1 < len(templateRoot.Content); i += 2 {
		templateKeys[templateRoot.Content[i].Value] = true
	}

	// orphan pairs a top-level key name with its key/value node pair, kept
	// together so the whole pair can be appended to templateRoot.Content and
	// sorted by name before appending.
	type orphan struct {
		name    string
		keyNode *yaml.Node
		valNode *yaml.Node
	}
	var orphans []orphan
	for i := 0; i+1 < len(existingRoot.Content); i += 2 {
		keyNode := existingRoot.Content[i]
		valNode := existingRoot.Content[i+1]
		if templateKeys[keyNode.Value] {
			continue
		}
		orphans = append(orphans, orphan{name: keyNode.Value, keyNode: keyNode, valNode: valNode})
	}
	if len(orphans) == 0 {
		return nil
	}

	sort.Slice(orphans, func(i, j int) bool { return orphans[i].name < orphans[j].name })

	preserved := make([]string, 0, len(orphans))
	for _, o := range orphans {
		// Direct assignment, never concatenation: this must overwrite
		// whatever HeadComment the key carried in existing (including a
		// preservedKeyComment from a prior --set run) so repeat calls stay
		// idempotent instead of duplicating or growing the comment.
		o.keyNode.HeadComment = preservedKeyComment
		templateRoot.Content = append(templateRoot.Content, o.keyNode, o.valNode)
		preserved = append(preserved, o.name)
	}
	return preserved
}

// rootMappingNode unwraps a parsed yaml.Node down to its root MappingNode.
func rootMappingNode(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}

// SetEntries sets each pair's key, a top-level key, to the JSON object its value spells, replacing the key's value whole or appending the key, and returns the new document.
// The other keys keep their order and comments, and the set entry keeps its key's comments.
// A value that is not a JSON object, or an empty key, is an error naming the pair, and nothing is returned.
func SetEntries(existing []byte, pairs []KV) ([]byte, error) {
	doc, root, err := parseMappingRoot(existing, "existing")
	if err != nil {
		return nil, err
	}
	if root == nil {
		root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}
	for _, pair := range pairs {
		if pair.Key == "" {
			return nil, fmt.Errorf("an entry name must not be empty, got %q", pair.Key+"="+pair.Value)
		}
		var object map[string]any
		if err := json.Unmarshal([]byte(pair.Value), &object); err != nil || object == nil {
			return nil, fmt.Errorf("value for %s must be a JSON object such as {\"module\": {\"key\": 1}}, got %q", pair.Key, pair.Value)
		}
		var parsed yaml.Node
		if err := yaml.Unmarshal([]byte(pair.Value), &parsed); err != nil {
			return nil, fmt.Errorf("value for %s: %w", pair.Key, err)
		}
		value := parsed.Content[0]
		plainScalars(value)
		value.Style = 0
		setEntry(root, pair.Key, value)
	}
	return yaml.Marshal(doc)
}

// plainScalars clears the quoting style of every scalar under node, so a JSON-spelled value is written in plain YAML and quoted only where its text needs it.
func plainScalars(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode {
		node.Style = 0
	}
	for _, child := range node.Content {
		plainScalars(child)
	}
}
