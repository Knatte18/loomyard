// set.go implements value-preserving single/multi-key YAML mutation for the non-interactive `lym config <module> --set key=value` path.
// Unlike Reconcile (which merges an entire existing file into a template), SetValues applies a small, explicit list of key=value pairs while still routing every write through the template-shaped working tree so partial/stale existing files never hide a valid key behind a missing node.
// It also grafts any existing top-level key absent from the template onto the working tree whole, at root-key granularity, so a hand-edited or template-outgrown key is carried through into the merged output instead of silently vanishing.

package yamlengine

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
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

// Shape is the shape a module's decode gives the value at a declared open-map path, which lets a set key reach below the open map's entries.
// A shape with Fields is a mapping of fixed keys, one with Entries a mapping of names the user owns, and one with neither a single value.
type Shape struct {
	// Fields maps each key a mapping of fixed keys holds to its shape.
	Fields map[string]*Shape
	// Entries is the shape of every entry of a mapping of names.
	Entries *Shape
	// Text marks a single value decoded as a string, so a set writes it as a string whatever its text.
	Text bool
}

// nodeType is the yaml.Node type, which decodes any value and so is a single value whatever it holds.
var nodeType = reflect.TypeFor[yaml.Node]()

// ShapeOf returns the shape yaml.v3 decodes value's type into:
// a struct is a mapping of the keys its exported fields' yaml tags name, a field tagged `-` left out and an untagged one keyed by its lower-cased name;
// a map is a mapping of names, a string a text value, and anything else, a yaml.Node included, a single value.
// A pointer takes its element's shape.
func ShapeOf(value any) *Shape {
	return shapeOfType(reflect.TypeOf(value))
}

// shapeOfType returns the shape yaml.v3 decodes a value of t into.
func shapeOfType(t reflect.Type) *Shape {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == nodeType:
		return &Shape{}
	case t.Kind() == reflect.Struct:
		fields := make(map[string]*Shape)
		for i := range t.NumField() {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = strings.ToLower(field.Name)
			}
			fields[name] = shapeOfType(field.Type)
		}
		return &Shape{Fields: fields}
	case t.Kind() == reflect.Map:
		return &Shape{Entries: shapeOfType(t.Elem())}
	case t.Kind() == reflect.String:
		return &Shape{Text: true}
	}
	return &Shape{}
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
// A pair at P itself, when P holds a mapping, replaces P whole with the YAML flow mapping the value spells, {} included;
// a value that is not one flow mapping is an error naming P and the per-key form, and under a shape a key the shape does not reach, or a mapping or list where the shape takes a string, is an error naming it.
// Known lists each open map as P and as P.<name>.
//
// An open map P that shapes maps to a shape takes a key P.<rest> for any rest the shape reaches, a name for each mapping of names on the way and a key the mapping holds for each mapping of fixed keys,
// and the pair sets that one value in the tree at P, creating every missing mapping on the way.
// A rest the shape does not reach, such as a key a mapping of fixed keys lacks or one below a single value, is an error naming the key, and nothing is merged.
// A text value is written as a string, a value whose first non-blank character is { or [ as a flow mapping or list, and any other value as a scalar under the tag its own text resolves to, so a number decodes as a number.
// A value on the way that is neither a mapping nor null is an error naming its path.
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
func SetValues(template, existing []byte, pairs []KV, perNameMaps []string, shapes map[string]*Shape, openMaps ...string) (SetResult, error) {
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
	if err := shapedKeyError(pairs, openMaps, shapes); err != nil {
		return SetResult{}, err
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
		if slices.Contains(openMaps, pair.Key) {
			if err := setWholeOpenMap(documentRoot(&templateNode), pair, shapes[pair.Key]); err != nil {
				return SetResult{}, err
			}
			continue
		}
		if shape, shaped, _ := shapedOpenMapKey(pair.Key, openMaps, shapes); shaped {
			if err := setShapedValue(documentRoot(&templateNode), pair, shape); err != nil {
				return SetResult{}, err
			}
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

// setWholeOpenMap replaces the open map pair.Key names, in the mapping tree under root, with the YAML flow mapping pair.Value spells, keeping the old value's comments;
// a line comment on an old value replaced by a non-empty mapping moves onto the key, where it marshals.
// Every mapping on the way to it is created when missing.
// A value that is not one flow mapping is an error naming the key and the per-key form;
// under shape, a key inside the mapping the shape does not reach is an error naming it, and nothing is set.
func setWholeOpenMap(root *yaml.Node, pair KV, shape *Shape) error {
	value, err := openMapEntryNode(pair)
	if err != nil {
		return err
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("value for %s is not a YAML flow mapping, got %q: write the map whole as {key: value, ...}, or set one key as %s.<key>", pair.Key, pair.Value, pair.Key)
	}
	if shape != nil {
		if err := mappingShapeError(pair.Key, value, shape); err != nil {
			return err
		}
	}
	parent, name, err := parentMappingOf(root, pair.Key)
	if err != nil {
		return err
	}
	setEntry(parent, name, value)
	// A line comment after an empty {} sits on the value, and a value's line comment is dropped when it marshals as a block mapping, so it moves onto the key.
	if key := parent.Content[mappingIndex(parent, name)]; len(value.Content) > 0 && value.LineComment != "" && key.LineComment == "" {
		key.LineComment, value.LineComment = value.LineComment, ""
	}
	return nil
}

// mappingShapeError returns an error naming the first key below path, in node, that shape does not reach or whose node is a mapping or list where shape takes a string, and nil otherwise.
// A single value that is not a string takes whatever node holds, so nothing below it is checked.
func mappingShapeError(path string, node *yaml.Node, shape *Shape) error {
	if shape.Text && node.Kind != yaml.ScalarNode {
		return fmt.Errorf("config key %s takes one string, not a %s: write it as a plain or quoted string", path, nodeShape(node))
	}
	if node.Kind != yaml.MappingNode || (shape.Fields == nil && shape.Entries == nil) {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		name := node.Content[i].Value
		next := shape.Entries
		if shape.Fields != nil {
			var ok bool
			if next, ok = shape.Fields[name]; !ok {
				return fmt.Errorf("unknown config key %s.%s: %s holds no key %s; it holds %s", path, name, path, name, strings.Join(slices.Sorted(maps.Keys(shape.Fields)), ", "))
			}
		}
		if err := mappingShapeError(path+"."+name, node.Content[i+1], next); err != nil {
			return err
		}
	}
	return nil
}

// shapedOpenMapKey returns the shape of the value key names under a declared open map that shapes maps to a shape, and whether key lies under such a map.
// A key with an empty segment, a segment a mapping of fixed keys lacks or a segment below a single value is an error naming the key and where it leaves the shape.
func shapedOpenMapKey(key string, openMaps []string, shapes map[string]*Shape) (*Shape, bool, error) {
	open, rest, ok := splitOpenMapKey(key, openMaps)
	if !ok {
		return nil, false, nil
	}
	shape, shaped := shapes[open]
	if !shaped {
		return nil, false, nil
	}
	walked := open
	for segment := range strings.SplitSeq(rest, ".") {
		switch {
		case segment == "":
			return nil, true, fmt.Errorf("config key %s holds an empty segment after %s", key, walked)
		case shape.Fields != nil:
			next, ok := shape.Fields[segment]
			if !ok {
				return nil, true, fmt.Errorf("unknown config key %s: %s holds no key %s; it holds %s", key, walked, segment, strings.Join(slices.Sorted(maps.Keys(shape.Fields)), ", "))
			}
			shape = next
		case shape.Entries != nil:
			shape = shape.Entries
		default:
			return nil, true, fmt.Errorf("unknown config key %s: %s holds one value, so no key lies under it", key, walked)
		}
		walked += "." + segment
	}
	return shape, true, nil
}

// shapedKeyError returns the error shapedOpenMapKey reports for the first pair key it refuses, or nil when it refuses none.
func shapedKeyError(pairs []KV, openMaps []string, shapes map[string]*Shape) error {
	for _, pair := range pairs {
		if _, _, err := shapedOpenMapKey(pair.Key, openMaps, shapes); err != nil {
			return err
		}
	}
	return nil
}

// setShapedValue writes pair's value, of shape, at pair's key in the mapping tree under root, creating every missing mapping on the way.
// A text value is a string; a mapping, or a value whose first non-blank character is { or [, takes the node an open-map entry takes;
// any other value is a scalar under the tag its own text resolves to.
// A value on the way that is neither a mapping nor null is an error naming its path.
func setShapedValue(root *yaml.Node, pair KV, shape *Shape) error {
	var value *yaml.Node
	trimmed := strings.TrimSpace(pair.Value)
	switch {
	case shape.Text:
		value = stringNode(pair.Value)
	case shape.Fields != nil, shape.Entries != nil, strings.HasPrefix(trimmed, "{"), strings.HasPrefix(trimmed, "["):
		entry, err := openMapEntryNode(pair)
		if err != nil {
			return err
		}
		value = entry
	default:
		// A null leaf tag makes scalarSetTag resolve the tag from the value's own text.
		value = &yaml.Node{Kind: yaml.ScalarNode, Tag: scalarSetTag("!!null", pair.Value), Value: pair.Value}
	}
	parentPath, name := splitLastSegment(pair.Key)
	parent, err := mappingAt(root, parentPath)
	if err != nil {
		return fmt.Errorf("%w: rewrite it as a map to set %s", err, pair.Key)
	}
	setEntry(parent, name, value)
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
