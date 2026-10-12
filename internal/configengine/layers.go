// layers.go implements the layer stack of a module's configuration: where each layer's file lives, which keys the local layer may hold, and the merge of the present layers into one unexpanded document.
// The loaders in config.go resolve through it; it is the one reader of layer files.

package configengine

import (
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
	// LayerLocal is the untracked local override file under the anchor's .lyx.
	LayerLocal Layer = "local"
)

// Category is why a config key may not be set in the local layer.
// CategoryNone means the key meets no category and the local layer may hold it.
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
)

// Spec describes one module to the layered loaders.
// OpenMaps are the dotted paths of the module's open maps;
// HubWide marks a module whose file lives at the hub only;
// WholeEntries makes each top-level key one unit, open to the local layer.
// Categories classifies the template by dotted path: an entry covers its own path and every path under it.
type Spec struct {
	Name         string
	Template     []byte
	OpenMaps     []string
	HubWide      bool
	WholeEntries bool
	Categories   map[string]Category
}

// LocalFile returns the path of a module's local override file under anchor:
// the ephemeral .lyx counterpart of the config directory, never the editor staging copy's path.
func LocalFile(anchor, module string) string {
	return filepath.Join(ScratchDir(anchor), module+".local.yaml")
}

// LayerFile returns the file of a layer and whether that layer applies under dirs.
// The default layer applies when Board is set;
// the worktree layer when Anchor is set and the module is not hub-wide or Board is empty;
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
	case LayerLocal:
		if dirs.Anchor != "" {
			return LocalFile(dirs.Anchor, spec.Name), true
		}
	}
	return "", false
}

// BaseLayer returns the lowest file layer, the one that stays full: the default layer when Board is set, else the worktree layer.
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

// ValidateLocal checks a local override document against the spec.
// Every unit path must exist in the template, or be a top-level key when the spec holds whole entries;
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
		templateValue := nodeAt(&templateNode, path)
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

// Layers returns the present file layers in stack order, base first.
// Each file is parsed, and the local one is checked by [ValidateLocal];
// a failure is marked [ErrInvalid] and names the file.
// An absent file is skipped, and an empty or comment-only local file is dropped.
// It is the one reader of layer files.
func Layers(dirs Dirs, spec Spec) ([]LayerDoc, error) {
	var docs []LayerDoc
	for _, layer := range []Layer{LayerDefault, LayerWorktree, LayerLocal} {
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
		if layer == LayerLocal {
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
	layers, err := Layers(dirs, spec)
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
