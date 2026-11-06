// diffmode.go — the drift.yaml rewrites that carry a hub across the diff rework: the rename of the model keys, the removal of the arguments that passed the retired skip file, and the flags for the values whose meaning changed.

package driftengine

import (
	"fmt"
	"regexp"
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

// retiredPartialScope is the scope value the rework retired, which FlagDiffMode names in a hub's file.
const retiredPartialScope = "partial"

// skipFileArgument matches one argument that hands the retired placeholder to a variable, `-e NAME={skip_file}`, `--env NAME={skip_file}` or `--env=NAME={skip_file}` with the assignment quoted or not, and the whitespace before it.
// A quote around the assignment is part of the match only when both quotes are there, so the quote that closes an enclosing `sh -c '...'` string is never taken and an argument that ends such a string stays.
// Its first group holds the whitespace before the argument and its second what ends the argument.
var skipFileArgument = regexp.MustCompile(`(^|\s+)(?:-e|--env)(?:\s+|=)(?:"` + skipFileAssignment + `"|'` + skipFileAssignment + `'|` + skipFileAssignment + `)(\s|$)`)

// skipFileAssignment is the pattern of one variable assignment handing over the retired placeholder.
var skipFileAssignment = `\w+=` + regexp.QuoteMeta(skipFilePlaceholder)

// MigrateConfig carries a drift.yaml document across what the rework retired.
// It renames the retired model keys, keeping each value: baseline_model to expect_model, verify_model to judge_model and verify_interactive to judge_interactive.
// A document already holding the new key keeps it and drops the old one.
// It removes each whole argument of a capture or compare command that hands {skip_file} to a variable, which nothing fills any more; any other use of the placeholder stays for the operator, since lym cannot tell what the command means by it, and so does an argument that ends a quoted shell string, as in `sh -c 'docker run -e A={skip_file}'`, since the quote that closes the string follows it.
// A document with none of these is returned unchanged.
// It returns one line per rename and per command it rewrote.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	migrated, lines, err := yamlengine.RenameKeys([]byte(ConfigTemplate()), existing, renamedKeys)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate drift config: %w", err))
	}
	migrated, dropped, err := dropSkipFileArguments(migrated)
	if err != nil {
		return nil, nil, err
	}
	return migrated, append(lines, dropped...), nil
}

// dropSkipFileArguments removes the skip file arguments from the capture and compare commands of a drift.yaml document, and returns it, rewritten only when a command changed, with one line per command changed.
func dropSkipFileArguments(existing []byte) ([]byte, []string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate drift config: unmarshal yaml: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil
	}
	var lines []string
	for _, step := range []string{stepCapture, stepCompare} {
		command := scalarAt(document.Content[0], step, "command")
		if command == nil {
			continue
		}
		kept := withoutSkipFileArguments(command.Value)
		if kept == command.Value {
			continue
		}
		command.Value = kept
		lines = append(lines, fmt.Sprintf("%s.command: removed the argument handing over %s, which nothing fills", step, skipFilePlaceholder))
	}
	if len(lines) == 0 {
		return existing, nil, nil
	}
	encoded, err := encodeDocument(&document, "migrate drift config")
	if err != nil {
		return nil, nil, err
	}
	return encoded, lines, nil
}

// withoutSkipFileArguments returns command with every skip file argument removed.
// One pass consumes the whitespace between two adjacent arguments, so it repeats until nothing matches.
func withoutSkipFileArguments(command string) string {
	for {
		kept := skipFileArgument.ReplaceAllStringFunc(command, argumentRemainder)
		if kept == command {
			return command
		}
		command = kept
	}
}

// argumentRemainder returns what stays of one matched skip file argument.
// An argument on its own line of a backslash-continued command keeps the line break that led to it and drops what followed, so the continuation before it stays whole;
// any other argument keeps what ended it, which is the single space between its neighbours.
func argumentRemainder(match string) string {
	parts := skipFileArgument.FindStringSubmatch(match)
	lead, end := parts[1], parts[2]
	if !strings.Contains(lead, "\n") {
		return end
	}
	if end == "\n" {
		return lead + end
	}
	return lead
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
		if command := scalarAt(root, step, "command"); command != nil && strings.Contains(command.Value, skipFilePlaceholder) {
			flagged = append(flagged, fmt.Sprintf("%s.command holds %s, which is no longer filled; way forward: drop %s from the command with lym config drift --set '%s={command: ..., timeout_min: N}'", step, skipFilePlaceholder, skipFilePlaceholder, step))
		}
	}
	if scope := scalarAt(root, "scope"); scope != nil && scope.Value == retiredPartialScope {
		flagged = append(flagged, "scope: partial is gone, so the base runs as full; way forward: lym config drift --set scope=full or lym config drift --set scope=off, and lym config reconcile prunes skip")
	}
	return existing, nil, flagged, nil
}
