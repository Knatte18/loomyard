// settinglookup.go answers a recipe row's named config value from a module's resolved layer stack.

package configreg

import (
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// SettingLookup returns a read-only lookup of one top-level key of one registered config module, resolved over the layer stack under dirs.
// A call refuses an unregistered module and a key the module's embedded template lacks, each with the way forward in its message.
// The first call for a module loads its stack strictly and keeps the decoded snapshot, so every later key of that module answers from it;
// a later change to a config file is seen only through a new lookup.
// The value is returned as yaml.v3 decodes it, and checking its shape is the reader's job.
// The closure is safe for concurrent use.
func SettingLookup(dirs configengine.Dirs) func(module, key string) (any, error) {
	var mu sync.Mutex
	snapshots := map[string]map[string]any{}
	return func(module, key string) (any, error) {
		m, ok := Lookup(module)
		if !ok {
			return nil, fmt.Errorf("configreg: setting module %q is not a registered config module; way forward: name one of %s", module, strings.Join(Names(), ", "))
		}
		var template map[string]any
		if err := yaml.Unmarshal([]byte(m.Template()), &template); err != nil {
			return nil, fmt.Errorf("configreg: decode %s template: %w", module, err)
		}
		if _, ok := template[key]; !ok {
			return nil, fmt.Errorf("configreg: setting key %q is not a key of module %q; way forward: name a key of %s.yaml's template", key, module, module)
		}

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
		return snapshot[key], nil
	}
}
