// reinterpret.go carries a hub's shuttle.yaml across the change in what edit_loop_edits counts.
// The key once counted every edit of a file and now counts only the edits that return it to earlier content.

package shuttleengine

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// EditLoopRepeatsReinterpretation names the change in what edit_loop_edits counts.
// The key once counted every edit of a file and now counts only the edits that return it to earlier content.
const EditLoopRepeatsReinterpretation = "shuttle-edit-loop-repeats"

// editLoopEditsKey is the shuttle.yaml key of Config.EditLoopEdits.
const editLoopEditsKey = "edit_loop_edits"

// DropEditLoopEdits is the reinterpretation of a hub's shuttle.yaml for EditLoopRepeatsReinterpretation.
// A count of every edit has no equivalent count of repeats.
// A hub that set 0 turned the watch off while every edit counted and the notices were noise.
// A 0, or a null or empty value, which loads as 0, is therefore dropped and the template's count applies.
// That drop is returned as one rewrote line naming the way to set another count.
// Any other value is left in the file and returned as one flagged line naming it and the way forward.
// Only the hub can say what count of repeats such a value means.
// A file without the key comes back unchanged with no line.
// The rewrite takes no worktree root or anchored directory, since the key names no path.
// A document that does not parse, or whose top level is not a mapping, returns an error marked configengine.ErrInvalid.
func DropEditLoopEdits(existing []byte, _, _ string) (rewritten []byte, rewrote, flagged []string, err error) {
	var values struct {
		EditLoopEdits yaml.Node `yaml:"edit_loop_edits"`
	}
	if err := yaml.Unmarshal(existing, &values); err != nil {
		return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("drop edit_loop_edits: unmarshal yaml: %w", err))
	}
	if values.EditLoopEdits.IsZero() {
		return existing, nil, nil, nil
	}
	var count int
	if err := values.EditLoopEdits.Decode(&count); err != nil || count != 0 {
		line := fmt.Sprintf("%s %s kept, since it counts only the edits that return a file to content it held earlier now, "+
			"so the value means a different count; way forward: set the count of repeats with "+
			"lym config shuttle --layer default --set %s=<count> from the prime worktree, or drop the key to take the template's count",
			editLoopEditsKey, describeYAMLValue(values.EditLoopEdits), editLoopEditsKey)
		return existing, nil, []string{line}, nil
	}
	rewritten, _, err = yamlengine.DropKeys(existing, []string{editLoopEditsKey})
	if err != nil {
		return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("drop edit_loop_edits: %w", err))
	}
	line := fmt.Sprintf("%s dropped, since it turned the watch off while every edit counted and the notices were noise, "+
		"so the template's count of repeats applies; way forward: set another count with lym config shuttle --layer default --set %s=<count> "+
		"from the prime worktree", editLoopEditsKey, editLoopEditsKey)
	return rewritten, []string{line}, nil, nil
}

// describeYAMLValue names a node's value for a message: its text when it has one, otherwise its kind.
func describeYAMLValue(node yaml.Node) string {
	switch {
	case node.Value != "":
		return node.Value
	case node.Kind == yaml.MappingNode:
		return "(a mapping)"
	case node.Kind == yaml.SequenceNode:
		return "(a sequence)"
	default:
		return "(an empty value)"
	}
}
