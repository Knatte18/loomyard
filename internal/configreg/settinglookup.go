// settinglookup.go answers a recipe row's named config value from a module's resolved layer stack.

package configreg

import (
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// CheckSettingKey refuses an unregistered module and a dotted key path the module's embedded template lacks.
// It refuses an old name of a moved key first, naming the new full name.
// Each refusal names the way forward in its message.
// CheckSettingKey reads no config file, so it answers without a worktree.
func CheckSettingKey(module, key string) error {
	m, ok := Lookup(module)
	if !ok {
		return fmt.Errorf("configreg: setting module %q is not a registered config module; way forward: name one of %s", module, strings.Join(Names(), ", "))
	}
	if newPath, moved := m.Spec().MovedKeys[key]; moved {
		return fmt.Errorf("configreg: setting key %s was moved; way forward: name it %s", configengine.KeyName(module, key), configengine.KeyName(module, newPath))
	}
	var template map[string]any
	if err := yaml.Unmarshal([]byte(m.Template()), &template); err != nil {
		return fmt.Errorf("configreg: decode %s template: %w", module, err)
	}
	if _, ok := valueAtPath(template, key); !ok {
		return fmt.Errorf("configreg: setting key %q is not a key of module %q; way forward: name a key of %s.yaml's template", configengine.KeyName(module, key), module, module)
	}
	return nil
}

// valueAtPath returns the value at the dotted path of document, and whether document holds one.
func valueAtPath(document map[string]any, path string) (any, bool) {
	var value any = document
	for _, step := range strings.Split(path, ".") {
		mapping, isMapping := value.(map[string]any)
		if !isMapping {
			return nil, false
		}
		var held bool
		if value, held = mapping[step]; !held {
			return nil, false
		}
	}
	return value, true
}

// SettingLookup returns a read-only lookup of one dotted key path of one registered config module.
// The lookup resolves that key over the layer stack under dirs.
// A call refuses an unregistered module and a key path the module's embedded template lacks.
// Each refusal names the way forward in its message.
// The first call for a module loads its stack strictly and keeps the decoded snapshot.
// Every later key of that module answers from it;
// a later change to a config file is seen only through a new lookup.
// The value is returned as yaml.v3 decodes it, and checking its shape is the reader's job.
// The closure is safe for concurrent use.
func SettingLookup(dirs configengine.Dirs) func(module, key string) (any, error) {
	var mu sync.Mutex
	snapshots := map[string]map[string]any{}
	return func(module, key string) (any, error) {
		if err := CheckSettingKey(module, key); err != nil {
			return nil, err
		}
		m, _ := Lookup(module)

		mu.Lock()
		defer mu.Unlock()
		snapshot, ok := snapshots[module]
		if !ok {
			document, err := configengine.Load(dirs, m.Spec())
			if err != nil {
				return nil, fmt.Errorf("configreg: load %s config: %w", module, err)
			}
			if err := yaml.Unmarshal(document, &snapshot); err != nil {
				return nil, fmt.Errorf("configreg: decode %s config: %w", module, err)
			}
			snapshots[module] = snapshot
		}
		value, _ := valueAtPath(snapshot, key)
		return value, nil
	}
}
