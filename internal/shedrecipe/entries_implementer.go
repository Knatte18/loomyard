// entries_implementer.go implements the "Implementer" registry entry, which builds a recipe's implementer row with the engine a config setting names, and the closed table of implementer engines it chooses from.

package shedrecipe

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// implementerSettingKey is the row Config key holding the `module.key` reference of the setting that names the implementer.
const implementerSettingKey = "implementer_setting"

// implementers maps each value the implementer setting may hold to the Constructor that builds the implementer row.
// Every implementer builds the same row with the same edges, so a choice never skips a row or enters one conditionally.
var implementers = map[string]Constructor{
	"webster": websterEntry,
}

// implementerEntry is the Constructor for the "Implementer" registry row.
// It reads the setting the row's implementer_setting key names through Env.Setting and builds the row with that implementer's Constructor, which gets every other key of the row's Config.
// It refuses a row without implementer_setting, and a setting that holds anything but one implementer name, each naming the way forward.
func implementerEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	value, ref, named, err := readSetting("Implementer", row, cfg, implementerSettingKey, env)
	if err != nil {
		return nil, err
	}
	if !named {
		return nil, fmt.Errorf("shedrecipe: Implementer: row %q: config key %q is absent; way forward: set %s to the module.key setting naming the implementer, such as %q", row.Name, implementerSettingKey, implementerSettingKey, "loom.implementer")
	}
	name, _ := value.(string)
	build, ok := implementers[name]
	if !ok {
		accepted := strings.Join(slices.Sorted(maps.Keys(implementers)), ", ")
		return nil, fmt.Errorf("shedrecipe: Implementer: row %q: setting %q (%s) holds %#v, want one of %s; way forward: run \"lyx config %s --set %s=<implementer>\" with one of %s", row.Name, ref.Text, ref, value, accepted, ref.Module, ref.Key, accepted)
	}
	rest := maps.Clone(cfg)
	delete(rest, implementerSettingKey)
	return build(row, rest, env)
}
