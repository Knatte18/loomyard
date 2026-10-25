// movekeys.go applies configreg's declared key moves to one layer of config files, before the layer's modules are reconciled.

package configsync

import (
	"errors"
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

// formerSourceDefaults maps a key move's source module to its source key to the template value that key held, as YAML text, before the key moved to another module.
// A base layer that still holds such a value was never tuned, so the move drops it instead of carrying it over the target's own default.
// Only values that differ from the target's current default are listed: an equal one changes nothing when it moves.
var formerSourceDefaults = map[string]map[string]string{
	"darn": {
		"darn_reviewer_model":     "sonnet[high]",
		"darn_fixer_model":        `""`,
		"darn_reviewer_fan":       `""`,
		"darn_fixer_fan":          `""`,
		"review_fan":              `""`,
		"review_fix_fan":          `""`,
		"review_hard_max_bounces": "2",
	},
}

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
	// fromTemplate marks a created standalone base file started from the module template, whose values carry no operator intent.
	fromTemplate bool
	// keepHeader marks a registered module's file of a hub's base layer: a move that leaves it with no unit saves it as the header-only base.
	// A retired module's file is deleted instead.
	keepHeader bool
	changed    bool
	// reported marks a file that carries report lines, so its Result is listed even when the move leaves it unwritten.
	reported bool
	// gained marks a target that received a value, so the move writes it before the sources.
	gained bool
	// moved, keptTarget and overwritten are the report lines the file's Result carries.
	moved, keptTarget, overwritten []string
}

// moveKeys applies configreg.KeyMoves to the layer's files under dir and returns one Result per file it changes, would change or carries report lines for.
// A file that only carries report lines, such as a target a prime-seeded source's dropped value was reported on, is never written and its Result is never Applied.
// dir is the board dir or the anchor, and base marks a base layer of its stack.
// seedDir is set only at a board pass that seeds from the prime: an absent source is then the prime's copy, never written.
// A value moves unexpanded, with its line comment replaced by the target template's.
// At a base layer a target value other than its template default is kept and the source key dropped, and a default is overwritten;
// at a sparse layer any target value is kept.
// At a base layer a source value equal to its key's former template default is dropped instead of moved.
// A move marked KeepTarget keeps any target value at either layer, the template default included, unless the target was started from the template in this pass,
// and its kept-target line names the dropped and kept keys without their mappings.
// A source file the move empties is deleted.
//
// The default layer is a hub's sparse base, so a move into or out of it writes it sparse:
// an absent target starts as the sparse seed of the prime's copy of the target when seedDir is set and that copy parses, else empty, and never from the template;
// a target value equal to the template default is overwritten only while the board's base-sparse marker is absent, and is a pin once it exists;
// a registered module's file the move leaves with no unit, target or source, is saved as the header-only base.
// Only a standalone worktree base, which stays full, starts an absent target from the template.
// A file that cannot be read or parsed fails the move with a *FileError before anything is written.
func moveKeys(dir, seedDir string, layer configengine.Layer, base, apply bool) ([]Result, error) {
	sparseBase := layer == configengine.LayerDefault
	overwriteDefaults := base
	if sparseBase {
		if _, err := os.Stat(configengine.BaseSparseMarker(dir)); err == nil {
			overwriteDefaults = false
		}
	}
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
		_, registered := configreg.Lookup(module)
		file := &moveFile{module: module, path: pathOf(module), keepHeader: sparseBase && registered}
		files[module] = file
		data, err := os.ReadFile(file.path)
		switch {
		case err == nil:
			file.exists = true
		case os.IsNotExist(err) && seedDir != "" && sparseBase:
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
		file := &moveFile{module: module, path: pathOf(module), keepHeader: sparseBase}
		files[module] = file
		data, err := os.ReadFile(file.path)
		switch {
		case err == nil:
			file.exists = true
		case os.IsNotExist(err):
			data = nil
			registered, ok := configreg.Lookup(module)
			if !ok {
				return nil, &FileError{Module: module, Path: file.path, Err: fmt.Errorf("key move target module %q is not registered", module)}
			}
			switch {
			case sparseBase:
				if prime := primeConfig(seedDir, module); prime != nil {
					_, markerErr := os.Stat(configengine.SparseMarker(seedDir))
					if data, err = sparseSeed(registered, prime, os.IsNotExist(markerErr)); err != nil {
						if errors.Is(err, configengine.ErrInvalid) {
							return nil, primeSeedError(registered, seedDir, err)
						}
						return nil, &FileError{Module: module, Path: file.path, Err: fmt.Errorf("seed %s from the prime: %w", module, err)}
					}
				}
			case base:
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
		if err := target.takeKey(source, at, move, overwriteDefaults); err != nil {
			return nil, err
		}
	}

	var reportable []*moveFile
	for _, file := range files {
		if file.changed {
			order = append(order, file)
		}
		if file.changed || file.reported {
			reportable = append(reportable, file)
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i].module < order[j].module })
	sort.Slice(reportable, func(i, j int) bool { return reportable[i].module < reportable[j].module })

	var results []Result
	for _, file := range reportable {
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

// copyKeys applies configreg.KeyCopies to the base files under boardDir and returns one Result per target file it changes or would change.
// A key is copied only when the source file sets it and the target file does not; the source keeps its key.
// The value is placed unexpanded with its line comment replaced by the target template's.
// An absent target file starts as the header-only base, and a target the copy leaves with units keeps its header.
// A file that cannot be read or parsed fails the copy with a *FileError before anything is written.
func copyKeys(boardDir string, apply bool) ([]Result, error) {
	loaded := map[string]*moveFile{}
	load := func(module string) (*moveFile, error) {
		if file, ok := loaded[module]; ok {
			return file, nil
		}
		file := &moveFile{module: module, path: configengine.ConfigFile(boardDir, module), keepHeader: true}
		loaded[module] = file
		data, err := os.ReadFile(file.path)
		switch {
		case err == nil:
			file.exists = true
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

	copied := map[string][]string{}
	var targets []*moveFile
	for _, keyCopy := range configreg.KeyCopies() {
		source, err := load(keyCopy.FromModule)
		if err != nil {
			return nil, err
		}
		at := mappingIndex(source.root, keyCopy.FromKey)
		if at < 0 {
			continue
		}
		target, err := load(keyCopy.ToModule)
		if err != nil {
			return nil, err
		}
		if mappingIndex(target.root, keyCopy.ToKey) >= 0 {
			continue
		}
		registered, ok := configreg.Lookup(keyCopy.ToModule)
		if !ok {
			return nil, &FileError{Module: keyCopy.ToModule, Path: target.path, Err: fmt.Errorf("key copy target module %q is not registered", keyCopy.ToModule)}
		}
		if len(copied[target.module]) == 0 {
			targets = append(targets, target)
		}
		placeValue(target.root, keyCopy.ToKey, source.root.Content[at+1], templateLineComment([]byte(registered.Template()), keyCopy.ToKey))
		copied[target.module] = append(copied[target.module], fmt.Sprintf("%s.%s -> %s.%s", keyCopy.FromModule, keyCopy.FromKey, keyCopy.ToModule, keyCopy.ToKey))
	}

	sort.Slice(targets, func(i, j int) bool { return targets[i].module < targets[j].module })
	var results []Result
	for _, target := range targets {
		sort.Strings(copied[target.module])
		result := Result{Module: target.module, Layer: configengine.LayerDefault, Copied: copied[target.module]}
		if apply {
			if err := target.writeCopied(); err != nil {
				return nil, err
			}
			result.Applied = true
		}
		results = append(results, result)
	}
	return results, nil
}

// writeCopied saves a copy target as a sparse base: its document, preceded by the base header when the file held no unit before.
func (file *moveFile) writeCopied() error {
	data, err := yaml.Marshal(file.doc)
	if err != nil {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("encode config for %s: %w", file.module, err)}
	}
	if !strings.HasPrefix(string(data), "#") {
		data = append([]byte(configengine.SparseBaseHeader), data...)
	}
	if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("mkdir config dir for %s: %w", file.module, err)}
	}
	if err := fsx.AtomicWriteBytes(file.path, data); err != nil {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("write config for %s: %w", file.module, err)}
	}
	return nil
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
	if len(file.doc.Content) == 0 {
		// A comment-only file, such as the header-only base, holds no unit.
		file.root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		file.doc.Kind = yaml.DocumentNode
		file.doc.Content = []*yaml.Node{file.root}
		return nil
	}
	if file.doc.Content[0].Kind != yaml.MappingNode {
		return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("parse: the top level is not a mapping")}
	}
	file.root = file.doc.Content[0]
	return nil
}

// formerSourceDefault reports whether value equals the template value the move's source key held before it moved.
func formerSourceDefault(move configreg.KeyMove, value *yaml.Node) (bool, error) {
	text, listed := formerSourceDefaults[move.FromModule][move.FromKey]
	if !listed {
		return false, nil
	}
	var former yaml.Node
	if err := yaml.Unmarshal([]byte(text), &former); err != nil {
		return false, fmt.Errorf("former default of %s.%s is not YAML: %w", move.FromModule, move.FromKey, err)
	}
	return nodesEqual(value, former.Content[0]), nil
}

// takeKey moves the source mapping's entry at index at into this target file under the move's target key, and records the report lines.
// The source entry is removed from its mapping in every case.
// With overwriteDefaults a source value equal to its key's former template default is dropped, never moved.
// A target started from the template, or one lacking the key, takes the source's mapping whole; a move never merges per name.
// With overwriteDefaults a target value equal to the template default is replaced by the moved value; without it that value is a pin and is kept.
func (target *moveFile) takeKey(source *moveFile, at int, move configreg.KeyMove, overwriteDefaults bool) error {
	value := source.root.Content[at+1]
	sourceName := move.FromModule + "." + move.FromKey
	targetName := move.ToModule + "." + move.ToKey

	reportOn := source
	if source.virtual {
		reportOn = target
	}
	reportOn.reported = true
	moved := fmt.Sprintf("%s -> %s", sourceName, targetName)
	source.root.Content = append(source.root.Content[:at], source.root.Content[at+2:]...)
	source.changed = true

	if overwriteDefaults {
		former, err := formerSourceDefault(move, value)
		if err != nil {
			return &FileError{Module: move.FromModule, Path: source.path, Err: err}
		}
		if former {
			reportOn.keptTarget = append(reportOn.keptTarget, fmt.Sprintf("%s: %s dropped, a former template default", sourceName, nodeText(value)))
			return nil
		}
	}

	held := mappingIndex(target.root, move.ToKey)
	if held >= 0 && !target.fromTemplate {
		current := target.root.Content[held+1]
		if move.KeepTarget {
			reportOn.keptTarget = append(reportOn.keptTarget, fmt.Sprintf("%s dropped, %s kept", sourceName, targetName))
			return nil
		}
		if !overwriteDefaults {
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

// write saves the file, or deletes it when the move left it with no keys;
// a hub base file with no keys is saved as the header-only base instead.
func (file *moveFile) write() error {
	var data []byte
	switch {
	case len(file.root.Content) > 0:
		var err error
		if data, err = yaml.Marshal(file.doc); err != nil {
			return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("encode config for %s: %w", file.module, err)}
		}
	case file.keepHeader:
		data = []byte(configengine.SparseBaseHeader)
	default:
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return &FileError{Module: file.module, Path: file.path, Err: fmt.Errorf("remove emptied config for %s: %w", file.module, err)}
		}
		return nil
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
