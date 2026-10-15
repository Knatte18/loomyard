// movekeys.go applies configreg's declared key moves to one layer of config files, before the layer's modules are reconciled.

package configsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// moveFile is one module's file of the layer a move works on, parsed and edited in memory until the move writes it.
type moveFile struct {
	module string
	path   string
	doc    *yaml.Node
	root   *yaml.Node
	// exists reports that the file is on disk.
	exists bool
	// virtual marks a source read from the prime's copy: the move never writes it.
	virtual bool
	// fromTemplate marks a created base file started from the module template, whose values carry no operator intent.
	fromTemplate bool
	changed      bool
	// gained marks a target that received a value, so the move writes it before the sources.
	gained bool
	// moved, keptTarget and overwritten are the report lines the file's Result carries.
	moved, keptTarget, overwritten []string
}

// moveKeys applies configreg.KeyMoves to the layer's files under dir and returns one Result per file it changes or would change.
// dir is the board dir or the anchor, and base marks the full base layer of its stack.
// seedDir is set only at a board pass that seeds from the prime: an absent source is then the prime's copy, never written,
// and an absent target starts from the prime's copy of the target when that parses.
// A value moves unexpanded, with its line comment replaced by the target template's.
// At a base layer a target value other than its template default is kept and the source key dropped, and a default is overwritten;
// at a sparse layer any target value is kept.
// A source file the move empties is deleted.
// A file that cannot be read or parsed fails the move with a *FileError before anything is written.
func moveKeys(dir, seedDir string, layer configengine.Layer, base, apply bool) ([]Result, error) {
	pathOf := func(module string) string {
		if layer == configengine.LayerLocal {
			return configengine.LocalFile(dir, module)
		}
		return configengine.ConfigFile(dir, module)
	}
	files := map[string]*moveFile{}
	var order []*moveFile

	loadSource := func(module string) (*moveFile, error) {
		if file, loaded := files[module]; loaded {
			return file, nil
		}
		file := &moveFile{module: module, path: pathOf(module)}
		files[module] = file
		data, err := os.ReadFile(file.path)
		switch {
		case err == nil:
			file.exists = true
		case os.IsNotExist(err) && seedDir != "" && layer == configengine.LayerDefault:
			if data = primeConfig(seedDir, module); data != nil {
				file.virtual = true
			}
		case os.IsNotExist(err):
			data = nil
		default:
			return nil, &FileError{Module: module, Path: file.path, Err: fmt.Errorf("read config for %s: %w", module, err)}
		}
		if err := file.parse(data); err != nil {
			return nil, err
		}
		return file, nil
	}

	loadTarget := func(module string) (*moveFile, error) {
		if file, loaded := files[module]; loaded {
			return file, nil
		}
		file := &moveFile{module: module, path: pathOf(module)}
		files[module] = file
		data, err := os.ReadFile(file.path)
		switch {
		case err == nil:
			file.exists = true
		case os.IsNotExist(err):
			data = nil
			if seedDir != "" && layer == configengine.LayerDefault {
				data = primeConfig(seedDir, module)
			}
			if data == nil && base {
				registered, ok := configreg.Lookup(module)
				if !ok {
					return nil, &FileError{Module: module, Path: file.path, Err: fmt.Errorf("key move target module %q is not registered", module)}
				}
				data = []byte(registered.Template())
				file.fromTemplate = true
			}
		default:
			return nil, &FileError{Module: module, Path: file.path, Err: fmt.Errorf("read config for %s: %w", module, err)}
		}
		if err := file.parse(data); err != nil {
			return nil, err
		}
		return file, nil
	}

	for _, move := range configreg.KeyMoves() {
		source, err := loadSource(move.FromModule)
		if err != nil {
			return nil, err
		}
		at := mappingIndex(source.root, move.FromKey)
		if at < 0 {
			continue
		}
		target, err := loadTarget(move.ToModule)
		if err != nil {
			return nil, err
		}
		if err := target.takeKey(source, at, move, base); err != nil {
			return nil, err
		}
	}

	for _, file := range files {
		if file.changed {
			order = append(order, file)
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i].module < order[j].module })

	var results []Result
	for _, file := range order {
		if file.virtual || len(file.moved)+len(file.keptTarget)+len(file.overwritten) == 0 {
			continue
		}
		sort.Strings(file.moved)
		sort.Strings(file.keptTarget)
		sort.Strings(file.overwritten)
		results = append(results, Result{Module: file.module, Layer: layer, Moved: file.moved, KeptTarget: file.keptTarget, Overwritten: file.overwritten})
	}
	if !apply {
		return results, nil
	}

	// Targets are written before sources, so an interrupted move leaves a value in both files rather than in neither.
	sort.SliceStable(order, func(i, j int) bool { return order[i].gained && !order[j].gained })
	for _, file := range order {
		if file.virtual {
			continue
		}
		if err := file.write(); err != nil {
			return nil, err
		}
		for i := range results {
			if results[i].Module == file.module {
				results[i].Applied = true
			}
		}
	}
	return results, nil
}

// parse reads data as the file's document; empty data is an empty mapping.
func (file *moveFile) parse(data []byte) error {
	if len(strings.TrimSpace(string(data))) == 0 {
		file.root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		file.doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{file.root}}
		return nil
	}
	file.doc = &yaml.Node{}
	if err := yaml.Unmarshal(data, file.doc); err != nil {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("parse: %w", err)}
	}
	if len(file.doc.Content) == 0 || file.doc.Content[0].Kind != yaml.MappingNode {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("parse: the top level is not a mapping")}
	}
	file.root = file.doc.Content[0]
	return nil
}

// takeKey moves the source mapping's entry at index at into this target file under the move's target key, and records the report lines.
// The source entry is removed from its mapping in every case.
func (target *moveFile) takeKey(source *moveFile, at int, move configreg.KeyMove, base bool) error {
	value := source.root.Content[at+1]
	sourceName := move.FromModule + "." + move.FromKey
	targetName := move.ToModule + "." + move.ToKey

	reportOn := source
	if source.virtual {
		reportOn = target
	}
	moved := fmt.Sprintf("%s -> %s", sourceName, targetName)
	source.root.Content = append(source.root.Content[:at], source.root.Content[at+2:]...)
	source.changed = true

	held := mappingIndex(target.root, move.ToKey)
	if held >= 0 && !target.fromTemplate {
		current := target.root.Content[held+1]
		if !base {
			reportOn.keptTarget = append(reportOn.keptTarget, fmt.Sprintf("%s: %s dropped, %s: %s kept", sourceName, nodeText(value), targetName, nodeText(current)))
			return nil
		}
		registered, ok := configreg.Lookup(move.ToModule)
		if !ok {
			return &FileError{Module: move.ToModule, Path: target.path, Err: fmt.Errorf("key move target module %q is not registered", move.ToModule)}
		}
		defaultNode, err := templateValueNode([]byte(registered.Template()), move.ToKey)
		if err != nil {
			return &FileError{Module: move.ToModule, Path: target.path, Err: err}
		}
		if defaultNode == nil || !nodesEqual(current, defaultNode) {
			reportOn.keptTarget = append(reportOn.keptTarget, fmt.Sprintf("%s: %s dropped, %s: %s kept", sourceName, nodeText(value), targetName, nodeText(current)))
			return nil
		}
		reportOn.overwritten = append(reportOn.overwritten, fmt.Sprintf("%s: %s replaced %s's template default %s", sourceName, nodeText(value), targetName, nodeText(defaultNode)))
	}

	registered, ok := configreg.Lookup(move.ToModule)
	if !ok {
		return &FileError{Module: move.ToModule, Path: target.path, Err: fmt.Errorf("key move target module %q is not registered", move.ToModule)}
	}
	placeValue(target.root, move.ToKey, value, templateLineComment([]byte(registered.Template()), move.ToKey))
	target.changed = true
	target.gained = true
	reportOn.moved = append(reportOn.moved, moved)
	if reportOn != target {
		target.moved = append(target.moved, moved)
	}
	return nil
}

// write saves the file, or deletes it when the move left it with no keys.
func (file *moveFile) write() error {
	if len(file.root.Content) == 0 {
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("remove emptied config for %s: %w", file.module, err)}
		}
		return nil
	}
	data, err := yaml.Marshal(file.doc)
	if err != nil {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("encode config for %s: %w", file.module, err)}
	}
	if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("mkdir config dir for %s: %w", file.module, err)}
	}
	if err := fsx.AtomicWriteBytes(file.path, data); err != nil {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("write config for %s: %w", file.module, err)}
	}
	return nil
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

// templateValueNode returns the value node the template holds at the top-level key, or nil when it has none.
func templateValueNode(template []byte, key string) (*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(template, &document); err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	root := document.Content[0]
	at := mappingIndex(root, key)
	if at < 0 {
		return nil, nil
	}
	return root.Content[at+1], nil
}

// nodesEqual reports whether two nodes hold the same kind, scalar value and children; comments and mapping key order are ignored.
func nodesEqual(a, b *yaml.Node) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.ScalarNode:
		return a.ShortTag() == b.ShortTag() && a.Value == b.Value
	case yaml.SequenceNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !nodesEqual(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
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
	}
	return false
}

// nodeText renders a node's value on one line, unexpanded.
func nodeText(node *yaml.Node) string {
	if node.Kind == yaml.ScalarNode {
		return node.Value
	}
	data, err := yaml.Marshal(copyBare(node))
	if err != nil {
		return "?"
	}
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.TrimSpace(string(data)), "\n", " ")), " ")
}

// copyBare deep-copies a node without any comment.
func copyBare(node *yaml.Node) *yaml.Node {
	clone := *node
	clone.HeadComment, clone.LineComment, clone.FootComment = "", "", ""
	clone.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		clone.Content[i] = copyBare(child)
	}
	return &clone
}

// placeValue sets key in the mapping to a bare copy of value, replacing an existing entry's value or appending the key.
// The entry takes the line comment text.
// A mapping value carries the comment on its key line, any other value on its own line.
func placeValue(mapping *yaml.Node, key string, value *yaml.Node, comment string) {
	placed := copyBare(value)
	at := mappingIndex(mapping, key)
	if at < 0 {
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, placed)
		at = len(mapping.Content) - 2
	} else {
		mapping.Content[at+1] = placed
	}
	keyNode := mapping.Content[at]
	keyNode.LineComment = ""
	if placed.Kind == yaml.MappingNode {
		keyNode.LineComment = comment
	} else {
		placed.LineComment = comment
	}
}

// templateLineComment returns the line comment, from its '#', that the template carries on its top-level key's line, or "" when the key has none.
func templateLineComment(template []byte, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(string(template), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		if at := strings.Index(line, " #"); at >= 0 {
			return strings.TrimSpace(line[at:])
		}
		return ""
	}
	return ""
}
