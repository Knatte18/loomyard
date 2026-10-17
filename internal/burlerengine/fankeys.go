// fankeys.go holds the per-round fan grammar a module's review fan keys share: the FanList value, its shape error and the validation that resolves every named fan through burler.yaml.

package burlerengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// FanList is a fan key's value: one fan name for every round, or a list of fan names, one per round, whose last entry serves every later round.
// An empty entry runs its round solo, or with the single fixer.
// A YAML scalar loads as a list of one.
type FanList []string

// FanShapeError reports a fan key whose value is neither a scalar nor a sequence of scalars.
// Line is the value's line in the file, which a loader maps onto the key it belongs to.
type FanShapeError struct {
	Line int
}

func (e *FanShapeError) Error() string {
	return fmt.Sprintf(`value at line %d is neither a fan name nor a list of fan names; set the key to a fan name, or to a list of fan names, with "" for a solo or single round`, e.Line)
}

// UnmarshalYAML accepts a scalar or a block sequence of scalars and rejects any other shape.
func (l *FanList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*l = FanList{node.Value}
		return nil
	case yaml.SequenceNode:
		fans := make(FanList, 0, len(node.Content))
		for _, entry := range node.Content {
			if entry.Kind != yaml.ScalarNode {
				return &FanShapeError{Line: entry.Line}
			}
			fans = append(fans, entry.Value)
		}
		*l = fans
		return nil
	default:
		return &FanShapeError{Line: node.Line}
	}
}

// FanKey is one config key holding a FanList.
type FanKey struct {
	Key  string
	Fans FanList
}

// ValidateFanKeys refuses an empty list on each of keys and resolves each non-empty entry through burler.yaml under dirs, which falls back per name to the embedded template.
// module names the config module the keys belong to in every refusal.
// A fan that does not resolve is refused naming the key, the 1-based entry, the unknown name and the fans that exist,
// and an unreadable burler.yaml is refused naming the first key with a non-empty entry.
// With every entry empty, burler.yaml is never read.
func ValidateFanKeys(dirs configengine.Dirs, module string, keys []FanKey) error {
	var burlerCfg *Config
	for _, fan := range keys {
		if len(fan.Fans) == 0 {
			return fmt.Errorf(`%s config key %q: the list is empty; write "" to run the round solo or with the single fixer`, module, fan.Key)
		}
		for i, name := range fan.Fans {
			if name == "" {
				continue
			}
			if burlerCfg == nil {
				loaded, err := LoadConfig(dirs)
				if err != nil {
					return fmt.Errorf("%s config key %q: %w; fix burler.yaml, or set the key to empty to run the segment solo", module, fan.Key, err)
				}
				burlerCfg = &loaded
			}
			if _, err := ResolveFan(*burlerCfg, name); err != nil {
				return fmt.Errorf(`%s config key %q entry %d: %w; define the fan in burler.yaml, or set the entry to ""`, module, fan.Key, i+1, err)
			}
		}
	}
	return nil
}
