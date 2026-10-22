// layers.go implements the layer stack of a module's configuration: where each layer's file lives, which keys the local layer may hold, and the merge of the present layers into one unexpanded document.
// The loaders in config.go resolve through it; it is the one reader of layer files.

package configengine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// Dirs is the told geometry of a layered load.
// Anchor is the reading worktree's anchor and Board the hub's board dir, empty standalone.
// Neither is derived: the caller hands both in.
type Dirs struct {
	Anchor string
	Board  string
}

// Layer names one layer of the stack, for provenance and for choosing a layer to write.
type Layer string

const (
	// LayerTemplate is the embedded template, the lowest layer; it has no file.
	LayerTemplate Layer = "template"
	// LayerDefault is the file under the board dir: the repo default of a per-worktree module, the hub file of a hub-wide one.
	LayerDefault Layer = "default"
	// LayerWorktree is the worktree's own tracked file under its anchor.
	LayerWorktree Layer = "worktree"
	// LayerMachine is the untracked machine override file under the board dir's .lyx, between the worktree and local layers.
	// It applies only to a hub-wide module with a machine key, and only when both Anchor and Board are told.
	LayerMachine Layer = "machine"
	// LayerLocal is the untracked local override file under the anchor's .lyx.
	LayerLocal Layer = "local"
)

// Category is why a config key may not be set in the local layer.
// CategoryNone means the key meets no category and the local layer may hold it.
// CategoryMachine is the one category that opens a key to another layer, the machine layer.
type Category string

const (
	// CategoryNone is a key that meets no category.
	CategoryNone Category = ""
	// CategoryExecutable is a key that runs an executable.
	CategoryExecutable Category = "executable"
	// CategorySourcedFile is a key that sources a file into a shared process.
	CategorySourcedFile Category = "sourced-file"
	// CategoryCrossPair is a key that reaches across pairs, including a shared process's boot option or what lands on the parent branch.
	CategoryCrossPair Category = "cross-pair"
	// CategoryGuard is a key that switches off or hollows a guard, a review or an approval gate.
	CategoryGuard Category = "guard"
	// CategoryMachine is a key whose value is a property of the machine, bounding a resource every worktree of the hub on that machine shares.
	// The machine layer holds it and the local layer refuses it.
	CategoryMachine Category = "machine"
)

// Spec describes one module to the layered loaders.
// OpenMaps are the dotted paths of the module's open maps;
// PerNameMaps are the dotted paths of the module's per-name maps, mappings of names the operator owns, such as a lens's prose or a batcher profile, which a layer adds to or overrides by name;
// HubWide marks a module whose file lives at the hub only;
// WholeEntries makes each top-level key one unit, open to the local layer.
// JSONEntries makes a set write each pair as one whole top-level entry from a JSON object, with no template key check.
// Categories classifies the template by dotted path: an entry covers its own path and every path under it.
// RetiredKeys are the dotted keys the module no longer reads, each mapped to the category it held as a template key;
// they stay out of Categories and OpenMaps, the local and machine layers admit them by that category, and a load drops them from the stacked document.
// SeedOnly marks the module whose file is the operator's registry, seeded whole and never rewritten, so its base stays full in a hub too.
// Overlay is the module's optional template overlay, picked by a selector key in the base layer; its zero value means none.
type Spec struct {
	Name         string
	Template     []byte
	OpenMaps     []string
	PerNameMaps  []string
	HubWide      bool
	WholeEntries bool
	JSONEntries  bool
	Categories   map[string]Category
	RetiredKeys  map[string]Category
	SeedOnly     bool
	Overlay      Overlay
}

// LocalFile returns the path of a module's local override file under anchor:
// the ephemeral .lyx counterpart of the config directory, never the editor staging copy's path.
func LocalFile(anchor, module string) string {
	return filepath.Join(ScratchDir(anchor), module+".local.yaml")
}

// MachineFile returns the path of a module's machine override file under board:
// the never-tracked .lyx counterpart of the hub file, never the editor staging copy's path.
func MachineFile(board, module string) string {
	return filepath.Join(ScratchDir(board), module+".machine.yaml")
}

// LayerFile returns the file of a layer and whether that layer applies under dirs.
// The default layer applies when Board is set;
// the worktree layer when Anchor is set and the module is not hub-wide or Board is empty;
// the machine layer when the module is hub-wide with a [MachineKeys] entry and both Anchor and Board are set,
// so a load that tells Board alone, as a hub-file check does, stacks none;
// the local layer when Anchor is set.
// The template layer never has a file.
func LayerFile(dirs Dirs, spec Spec, layer Layer) (string, bool) {
	switch layer {
	case LayerDefault:
		if dirs.Board != "" {
			return ConfigFile(dirs.Board, spec.Name), true
		}
	case LayerWorktree:
		if dirs.Anchor != "" && (!spec.HubWide || dirs.Board == "") {
			return ConfigFile(dirs.Anchor, spec.Name), true
		}
	case LayerMachine:
		if spec.HubWide && dirs.Anchor != "" && dirs.Board != "" && len(MachineKeys(spec)) > 0 {
			return MachineFile(dirs.Board, spec.Name), true
		}
	case LayerLocal:
		if dirs.Anchor != "" {
			return LocalFile(dirs.Anchor, spec.Name), true
		}
	}
	return "", false
}

// BaseLayer returns the lowest file layer: the default layer when Board is set, else the worktree layer.
// In a hub the base is sparse over the template, a key-less one holding [SparseBaseHeader];
// only a seed-only module's base and a standalone worktree base stay full.
func BaseLayer(dirs Dirs, spec Spec) Layer {
	if dirs.Board != "" {
		return LayerDefault
	}
	return LayerWorktree
}

// LocalKeys returns the sorted paths the spec classifies as CategoryNone, the keys the local layer may hold.
func LocalKeys(spec Spec) []string {
	return categoryPaths(spec, func(category Category) bool { return category == CategoryNone })
}

// LocalDenied returns the sorted paths the spec classifies in any category but CategoryNone, the keys the local layer refuses.
func LocalDenied(spec Spec) []string {
	return categoryPaths(spec, func(category Category) bool { return category != CategoryNone })
}

// MachineKeys returns the sorted paths the spec classifies as CategoryMachine, the keys the machine layer may hold.
func MachineKeys(spec Spec) []string {
	return categoryPaths(spec, func(category Category) bool { return category == CategoryMachine })
}

// categoryPaths returns the sorted Categories paths whose category satisfies keep.
func categoryPaths(spec Spec, keep func(Category) bool) []string {
	paths := []string{}
	for path, category := range spec.Categories {
		if keep(category) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// coveringCategory returns the category of the longest Categories entry covering path, and whether any entry covers it.
func coveringCategory(spec Spec, path string) (Category, bool) {
	best := ""
	var category Category
	found := false
	for entry, entryCategory := range spec.Categories {
		if path != entry && !strings.HasPrefix(path, entry+".") {
			continue
		}
		if !found || len(entry) > len(best) {
			best, category, found = entry, entryCategory, true
		}
	}
	return category, found
}

// retiredCovering returns the longest retired key equal to path or a dotted prefix of it, its category, and whether any covers path.
func retiredCovering(spec Spec, path string) (string, Category, bool) {
	best := ""
	var category Category
	found := false
	for key, keyCategory := range spec.RetiredKeys {
		if path != key && !strings.HasPrefix(path, key+".") {
			continue
		}
		if !found || len(key) > len(best) {
			best, category, found = key, keyCategory, true
		}
	}
	return best, category, found
}

// dropRetired removes from doc every retired key of the spec, at its dotted path, keeping every other key and comment.
// A document that holds none is returned byte for byte.
func dropRetired(doc []byte, spec Spec) ([]byte, error) {
	if len(spec.RetiredKeys) == 0 || len(strings.TrimSpace(string(doc))) == 0 {
		return doc, nil
	}
	var parsed yaml.Node
	if err := yaml.Unmarshal(doc, &parsed); err != nil {
		return nil, MarkInvalid(err)
	}
	if parsed.Kind != yaml.DocumentNode || len(parsed.Content) == 0 {
		return doc, nil
	}
	dropped := false
	for key := range spec.RetiredKeys {
		if removeKey(parsed.Content[0], strings.Split(key, ".")) {
			dropped = true
		}
	}
	if !dropped {
		return doc, nil
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&parsed); err != nil {
		return nil, fmt.Errorf("marshal %s config without retired keys: %w", spec.Name, err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("marshal %s config without retired keys: %w", spec.Name, err)
	}
	return out.Bytes(), nil
}

// removeKey deletes the mapping entry at the key path under node and reports whether one was removed.
func removeKey(node *yaml.Node, path []string) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != path[0] {
			continue
		}
		if len(path) == 1 {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return true
		}
		return removeKey(node.Content[i+1], path[1:])
	}
	return false
}

// ValidateLocal checks a local override document against the spec.
// A unit a retired key covers is judged by that key's category alone: CategoryNone admits it with no shape check, any other category refuses it.
// Every other unit path must exist in the template, or be a top-level key when the spec holds whole entries;
// a path under a per-name map exists when the template holds it or, whatever the name, some template entry of that map holds the rest of it, and is judged against that entry;
// its shape, mapping or not, must match the template's where the template holds the path;
// and the longest category entry covering it must be CategoryNone, a hub-wide module also refusing a path no entry covers.
// A failure is marked [ErrInvalid] and names the key; an empty or comment-only document passes.
func ValidateLocal(spec Spec, doc []byte) error {
	paths, err := yamlengine.LeafPaths(doc, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return MarkInvalid(err)
	}
	if len(paths) == 0 {
		return nil
	}
	var docNode, templateNode yaml.Node
	if err := yaml.Unmarshal(doc, &docNode); err != nil {
		return MarkInvalid(err)
	}
	if err := yaml.Unmarshal(spec.Template, &templateNode); err != nil {
		return fmt.Errorf("%s config template: %w", spec.Name, err)
	}

	for _, path := range paths {
		if _, retiredCategory, retired := retiredCovering(spec, path); retired {
			if retiredCategory != CategoryNone {
				return MarkInvalid(fmt.Errorf("key %q is %s and cannot be set in a local override", path, retiredCategory))
			}
			continue
		}
		templateValue := templateValueAt(spec, &templateNode, path)
		if templateValue == nil && !spec.WholeEntries {
			return MarkInvalid(fmt.Errorf("key %q is not in the %s template", path, spec.Name))
		}
		if templateValue != nil {
			docIsMapping := nodeAt(&docNode, path).Kind == yaml.MappingNode
			if docIsMapping != (templateValue.Kind == yaml.MappingNode) {
				return MarkInvalid(fmt.Errorf("key %q: the %s template holds a different shape", path, spec.Name))
			}
		}
		category, covered := coveringCategory(spec, path)
		if covered && category != CategoryNone {
			return MarkInvalid(fmt.Errorf("key %q is %s and cannot be set in a local override", path, category))
		}
		if !covered && spec.HubWide {
			return MarkInvalid(fmt.Errorf("key %q is not classified for a local override", path))
		}
	}
	return nil
}

// machineRemoval is the way forward of a refusal whose key belongs in no machine override.
const machineRemoval = "; way forward: remove it from the machine override"

// ValidateMachine checks a machine override document against the spec.
// A unit a retired key covers is admitted only when that key's category is CategoryMachine, and its value shape is never checked.
// Every other unit path must exist in the template and match its shape, and the longest category entry covering it must be CategoryMachine;
// a path under a per-name map exists when the template holds it or, whatever the name, some template entry of that map holds the rest of it, and is judged against that entry.
// A failure is marked [ErrInvalid], names the key and a way forward; an empty or comment-only document passes.
func ValidateMachine(spec Spec, doc []byte) error {
	paths, err := yamlengine.LeafPaths(doc, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return MarkInvalid(fmt.Errorf("%w%s", err, machineRemoval))
	}
	if len(paths) == 0 {
		return nil
	}
	var docNode, templateNode yaml.Node
	if err := yaml.Unmarshal(doc, &docNode); err != nil {
		return MarkInvalid(fmt.Errorf("%w%s", err, machineRemoval))
	}
	if err := yaml.Unmarshal(spec.Template, &templateNode); err != nil {
		return fmt.Errorf("%s config template: %w", spec.Name, err)
	}

	for _, path := range paths {
		if _, retiredCategory, retired := retiredCovering(spec, path); retired {
			if retiredCategory != CategoryMachine {
				return MarkInvalid(fmt.Errorf("key %q is retired and cannot be set in a machine override%s", path, machineRemoval))
			}
			continue
		}
		templateValue := templateValueAt(spec, &templateNode, path)
		if templateValue == nil {
			return MarkInvalid(fmt.Errorf("key %q is not in the %s template%s", path, spec.Name, machineRemoval))
		}
		docIsMapping := nodeAt(&docNode, path).Kind == yaml.MappingNode
		if docIsMapping != (templateValue.Kind == yaml.MappingNode) {
			return MarkInvalid(fmt.Errorf("key %q: the %s template holds a different shape%s", path, spec.Name, machineRemoval))
		}
		category, covered := coveringCategory(spec, path)
		switch {
		case !covered:
			return MarkInvalid(fmt.Errorf("key %q is not classified for a machine override%s", path, machineRemoval))
		case category == CategoryNone:
			return MarkInvalid(fmt.Errorf("key %q is not a machine key; way forward: remove it from the machine override and set it in the hub file through \"lyx config %s\" from the prime, or in the local override through \"lyx config %s --layer local\"", path, spec.Name, spec.Name))
		case category != CategoryMachine:
			return MarkInvalid(fmt.Errorf("key %q is %s and cannot be set in a machine override; way forward: remove it from the machine override and set it in the hub file through \"lyx config %s\" from the prime", path, category, spec.Name))
		}
	}
	return nil
}

// templateValueAt returns the template node a unit path is judged against: the node at path,
// or, for a path the template lacks under a per-name map P, P.<name> or P.<name>.<rest>, the first template entry at P, or the node at rest in the first entry holding it.
// It returns nil when neither holds the path.
func templateValueAt(spec Spec, template *yaml.Node, path string) *yaml.Node {
	if node := nodeAt(template, path); node != nil {
		return node
	}
	for _, perName := range spec.PerNameMaps {
		under, found := strings.CutPrefix(path, perName+".")
		if !found {
			continue
		}
		entries := nodeAt(template, perName)
		if entries == nil || entries.Kind != yaml.MappingNode {
			continue
		}
		_, rest, _ := strings.Cut(under, ".")
		for i := 1; i < len(entries.Content); i += 2 {
			if rest == "" {
				return entries.Content[i]
			}
			if node := nodeAt(entries.Content[i], rest); node != nil {
				return node
			}
		}
	}
	return nil
}

// nodeAt returns the value node of the mapping key at the dotted path in a parsed document, or nil when a step is absent or not a mapping.
func nodeAt(doc *yaml.Node, path string) *yaml.Node {
	node := doc
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	for _, key := range strings.Split(path, ".") {
		if node.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				next = node.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		node = next
	}
	return node
}

// LayerDoc is one present layer file: its layer, its path and its raw bytes.
type LayerDoc struct {
	Layer Layer
	Path  string
	Bytes []byte
}

// Layers returns the present file layers in stack order, base first:
// default, worktree, machine, local.
// Each file is parsed, the machine one is checked by [ValidateMachine] and the local one by [ValidateLocal];
// a failure is marked [ErrInvalid] and names the file.
// An absent file is skipped, and an empty or comment-only machine or local file is dropped.
// No reconcile reads or writes the machine file.
// It is the one reader of layer files.
func Layers(dirs Dirs, spec Spec) ([]LayerDoc, error) {
	return readLayers(dirs, spec, true)
}

// readLayers is Layers' body.
// With anchorLayers false the worktree and local layers are skipped, as for an anchor with no _lyx/ beside a told board dir;
// the machine layer is judged on dirs alone.
func readLayers(dirs Dirs, spec Spec, anchorLayers bool) ([]LayerDoc, error) {
	var docs []LayerDoc
	var defaultBytes []byte
	for _, layer := range []Layer{LayerDefault, LayerWorktree, LayerMachine, LayerLocal} {
		if !anchorLayers && (layer == LayerWorktree || layer == LayerLocal) {
			continue
		}
		path, applies := LayerFile(dirs, spec, layer)
		if !applies {
			continue
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read config file %s: %w", path, err)
		}
		units, err := yamlengine.LeafPaths(data, spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			return nil, MarkInvalid(fmt.Errorf("config file %s: %w", path, err))
		}
		switch layer {
		case LayerDefault:
			defaultBytes = data
		case LayerWorktree:
			// Only the base picks a layer: a hub worktree file holding another value than the base's would change the template the base is judged against.
			if dirs.Board != "" && spec.Overlay.Selector != "" {
				other, err := worktreeSelectorOther(spec, data, defaultBytes)
				if err != nil {
					return nil, err
				}
				if other {
					return nil, selectorRefusal(dirs, spec, path, true)
				}
			}
		case LayerMachine:
			if len(units) == 0 {
				continue
			}
			if err := ValidateMachine(spec, data); err != nil {
				return nil, MarkInvalid(fmt.Errorf("machine config file %s: %w", path, err))
			}
		case LayerLocal:
			if len(units) == 0 {
				continue
			}
			if err := ValidateLocal(spec, data); err != nil {
				return nil, MarkInvalid(fmt.Errorf("local config file %s: %w", path, err))
			}
		}
		docs = append(docs, LayerDoc{Layer: layer, Path: path, Bytes: data})
	}
	return docs, nil
}

// BelowLayer returns the unexpanded merge of the spec's template and every present file layer below layer, read through [Layers].
// It is what a layer's keys are compared against to tell which of them add nothing, so it merges the layers the way the loaders do and no other package reads a layer file itself.
func BelowLayer(dirs Dirs, spec Spec, layer Layer) ([]byte, error) {
	layers, err := Layers(dirs, spec)
	if err != nil {
		return nil, err
	}
	merged, err := EffectiveTemplate(spec, baseDocument(dirs, spec, layers), dirs.Board != "")
	if err != nil {
		return nil, err
	}
	for _, doc := range layers {
		if layerRank(doc.Layer) >= layerRank(layer) {
			break
		}
		merged, err = yamlengine.Merge(merged, doc.Bytes, spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			return nil, MarkInvalid(fmt.Errorf("config file %s: %w", doc.Path, err))
		}
	}
	return merged, nil
}

// layerRank orders the layers from the template up.
func layerRank(layer Layer) int {
	switch layer {
	case LayerTemplate:
		return 0
	case LayerDefault:
		return 1
	case LayerWorktree:
		return 2
	case LayerMachine:
		return 3
	default:
		return 4
	}
}

// Stack is one module's stacked layers.
// Doc is the present file layers merged unexpanded in stack order, with no template key filled.
// Layers are the layers Doc was merged from.
// Origins maps each top-level key Doc holds to the highest layer holding it.
type Stack struct {
	Doc     []byte
	Layers  []LayerDoc
	Origins map[string]LayerDoc
}

// StackLayers returns the module's stack under dirs, with the errors of [Layers].
// No present layer yields an empty Doc.
// It is the one place layers are merged.
func StackLayers(dirs Dirs, spec Spec) (Stack, error) {
	return stackLayers(dirs, spec, true)
}

// stackLayers is StackLayers' body, reading its layers through readLayers with the same anchorLayers.
func stackLayers(dirs Dirs, spec Spec, anchorLayers bool) (Stack, error) {
	layers, err := readLayers(dirs, spec, anchorLayers)
	if err != nil {
		return Stack{}, err
	}
	stack := Stack{Doc: []byte{}, Layers: layers, Origins: map[string]LayerDoc{}}
	for _, layer := range layers {
		stack.Doc, err = yamlengine.Merge(stack.Doc, layer.Bytes, spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			return Stack{}, MarkInvalid(fmt.Errorf("config file %s: %w", layer.Path, err))
		}
		var parsed yaml.Node
		if err := yaml.Unmarshal(layer.Bytes, &parsed); err != nil {
			return Stack{}, MarkInvalid(fmt.Errorf("config file %s: %w", layer.Path, err))
		}
		for _, key := range topLevelKeys(&parsed) {
			stack.Origins[key] = layer
		}
	}
	return stack, nil
}

// Provenance returns the module's resolved configuration with each unit's line comment naming the layer it came from:
// the highest layer whose document holds the unit, or the template layer when none does.
// The template layer is labelled template:<value> when the overlay document the base picks holds the unit.
// It resolves through [Load], so its strictness and refusals are Load's, and an absent base layer errors rather than degrading.
func Provenance(dirs Dirs, spec Spec) ([]byte, error) {
	resolved, err := Load(dirs, spec)
	if err != nil {
		return nil, err
	}
	anchorLayers := true
	if dirs.Board != "" && dirs.Anchor != "" {
		if _, err := FindBaseDir(dirs.Anchor); err != nil {
			anchorLayers = false
		}
	}
	layers, err := readLayers(dirs, spec, anchorLayers)
	if err != nil {
		return nil, err
	}
	held := make(map[Layer]map[string]bool, len(layers))
	for _, doc := range layers {
		paths, err := yamlengine.LeafPaths(doc.Bytes, spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			return nil, MarkInvalid(fmt.Errorf("config file %s: %w", doc.Path, err))
		}
		held[doc.Layer] = make(map[string]bool, len(paths))
		for _, path := range paths {
			held[doc.Layer][path] = true
		}
	}
	units, err := yamlengine.LeafPaths(resolved, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, err
	}
	value, _, err := selectorValue(spec, baseDocument(dirs, spec, layers))
	if err != nil {
		return nil, err
	}
	overlayUnits := map[string]bool{}
	if document, picked := spec.Overlay.Documents[value]; picked && value != "" {
		paths, err := yamlengine.LeafPaths(document, spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			return nil, MarkInvalid(fmt.Errorf("%s config: %s %q layer: %w", spec.Name, spec.Overlay.Selector, value, err))
		}
		for _, path := range paths {
			overlayUnits[path] = true
		}
	}
	notes := make(map[string]string, len(units))
	for _, unit := range units {
		origin := string(LayerTemplate)
		if overlayUnits[unit] {
			origin = string(LayerTemplate) + ":" + value
		}
		for _, doc := range layers {
			if held[doc.Layer][unit] {
				origin = string(doc.Layer)
			}
		}
		notes[unit] = origin
	}
	return yamlengine.AnnotateLeaves(resolved, notes, spec.OpenMaps, spec.WholeEntries)
}

// topLevelKeys returns the keys of a parsed document's root mapping.
func topLevelKeys(doc *yaml.Node) []string {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	root := doc.Content[0]
	keys := make([]string, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		keys = append(keys, root.Content[i].Value)
	}
	return keys
}

// KeyAtLine returns the top-level key of a resolved document whose entry spans line, or "" when none does.
// An entry spans from its key's line up to the next key's line, so a loader names the key a value decoding error at that line belongs to.
func KeyAtLine(contents []byte, line int) string {
	var root yaml.Node
	if err := yaml.Unmarshal(contents, &root); err != nil || len(root.Content) == 0 {
		return ""
	}
	keys := root.Content[0].Content
	found := ""
	for i := 0; i < len(keys); i += 2 {
		if keys[i].Line <= line {
			found = keys[i].Value
		}
	}
	return found
}
