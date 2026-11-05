// diffmode.go — the drift.yaml rewrites that carry a hub across the diff rework: the rename of the model keys, and the flags for the values whose meaning changed.

package driftengine

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// DiffModeReinterpretation names the change of what a hub's compare, capture and scope values mean once a run diffs the capture itself.
const DiffModeReinterpretation = "drift-diff-mode"

// renamedKeys maps each drift.yaml key the diff rework renamed onto its new name.
var renamedKeys = []yamlengine.KeyRename{
	{From: "baseline_model", To: "expect_model"},
	{From: "verify_model", To: "judge_model"},
	{From: "verify_interactive", To: "judge_interactive"},
}

// Values the rework retired; FlagDiffMode names them in a hub's file without reading their old meaning from the code.
const (
	retiredSkipFilePlaceholder = "{skip_file}"
	retiredPartialScope        = "partial"
)

// MigrateConfig renames the retired model keys of a drift.yaml document, keeping each value:
// baseline_model to expect_model, verify_model to judge_model and verify_interactive to judge_interactive.
// A document already holding the new key keeps it and drops the old one, and a document without a retired key is returned unchanged.
// It returns one line per rename.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	migrated, lines, err := yamlengine.RenameKeys([]byte(ConfigTemplate()), existing, renamedKeys)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate drift config: %w", err))
	}
	return migrated, lines, nil
}

// FlagDiffMode flags the values of a hub drift.yaml whose meaning changed, one line each, and rewrites nothing.
// It flags a compare declared with no diff set, since the compare now runs only under diff: command;
// a capture or compare command holding {skip_file}, which is no longer filled;
// and scope: partial, which is gone, so the base runs as full.
// It returns the document unchanged, no rewritten line and the flags, each naming its way forward.
// worktreeRoot and anchorDir are unused, since no flag depends on a path.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func FlagDiffMode(existing []byte, worktreeRoot, anchorDir string) (rewritten []byte, rewrote, flagged []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("flag drift diff mode: unmarshal yaml: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return existing, nil, nil, nil
	}
	root := document.Content[0]

	if command := scalarAt(root, stepCompare, "command"); command != nil && command.Value != "" && scalarAt(root, "diff") == nil {
		flagged = append(flagged, "compare is set while diff is not: compare now runs only under diff: command, and the default diff: lines needs a capture of files of lines the key_patterns read; way forward: lym config drift --set diff=command to keep the compare, or lym config drift --set diff=lines once the capture meets that contract")
	}
	for _, step := range []string{stepCapture, stepCompare} {
		if command := scalarAt(root, step, "command"); command != nil && strings.Contains(command.Value, retiredSkipFilePlaceholder) {
			flagged = append(flagged, fmt.Sprintf("%s.command holds %s, which is no longer filled; way forward: drop %s from the command with lym config drift --set '%s={command: ..., timeout_min: N}'", step, retiredSkipFilePlaceholder, retiredSkipFilePlaceholder, step))
		}
	}
	if scope := scalarAt(root, "scope"); scope != nil && scope.Value == retiredPartialScope {
		flagged = append(flagged, "scope: partial is gone, so the base runs as full; way forward: lym config drift --set scope=full or lym config drift --set scope=off, and lym config reconcile prunes skip")
	}
	return existing, nil, flagged, nil
}
