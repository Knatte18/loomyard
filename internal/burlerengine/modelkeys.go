// modelkeys.go holds the per-round model-spec grammar a module's reviewer and fixer model keys share: the ModelSpecList value, its shape error and the validation of every entry.

package burlerengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// ModelSpecList is a reviewer or fixer model key's value: one model-spec for every round, or a list of model-specs, one per round, whose last entry serves every later round.
// A YAML scalar loads as a list of one.
type ModelSpecList []string

// modelSpecListWayForward is the way forward every model-spec list refusal ends with.
const modelSpecListWayForward = "set the key to a model-spec, or to a non-empty list of model-specs"

// ModelSpecShapeError reports a model key whose value is neither a scalar nor a sequence of scalars.
// Line is the value's line in the file, which a loader maps onto the key it belongs to.
type ModelSpecShapeError struct {
	Line int
}

func (e *ModelSpecShapeError) Error() string {
	return fmt.Sprintf("value at line %d is neither a model-spec nor a list of model-specs; %s", e.Line, modelSpecListWayForward)
}

// UnmarshalYAML accepts a scalar or a block sequence of scalars and rejects any other shape.
func (l *ModelSpecList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*l = ModelSpecList{node.Value}
		return nil
	case yaml.SequenceNode:
		specs := make(ModelSpecList, 0, len(node.Content))
		for _, entry := range node.Content {
			if entry.Kind != yaml.ScalarNode {
				return &ModelSpecShapeError{Line: entry.Line}
			}
			specs = append(specs, entry.Value)
		}
		*l = specs
		return nil
	default:
		return &ModelSpecShapeError{Line: node.Line}
	}
}

// Unset reports whether the list is empty or a single empty string, which a segment's own key reads as "take the shared fallback".
func (l ModelSpecList) Unset() bool {
	return len(l) == 0 || (len(l) == 1 && l[0] == "")
}

// ValidateModelSpecList refuses an empty list and an entry that is not a model-spec, naming module, key and the 1-based entry index.
func ValidateModelSpecList(module, key string, specs ModelSpecList) error {
	if len(specs) == 0 {
		return fmt.Errorf("%s config key %q: the list is empty; %s", module, key, modelSpecListWayForward)
	}
	for i, spec := range specs {
		if _, err := modelspec.Parse(spec); err != nil {
			return fmt.Errorf("%s config key %q entry %d: %w; %s", module, key, i+1, err, modelSpecListWayForward)
		}
	}
	return nil
}
