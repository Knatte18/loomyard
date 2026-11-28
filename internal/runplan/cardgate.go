// cardgate.go derives each card's gate steps and the batch's gate, which a batch prompt renders.
// The batch's attempt runs these steps, never Go itself.
// An attempt that skips them is caught by the plan-level verify.

package runplan

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// CardPointerPath returns c's card file path re-rooted onto planDirDisplay.
// Its pointer bullet and its card gate bullet both spell this path.
func CardPointerPath(c planparser.Card, planDirDisplay string) string {
	return planDirDisplay + "/" + filepath.Base(c.SourcePath)
}

// CardGateCommand returns the gate for card as its ordered steps:
// the hub's `card` gate call over the card's own scope.
// The call also runs the lint steps that the `card` key lists.
// It refuses, with the config's way forward, a hub whose `card` list the config does not hold.
func CardGateCommand(card planparser.Card, gate gateslot.Config) ([]string, error) {
	call, err := gate.AgentGateCall(gateslot.CardGateKey, card.ID())
	if err != nil {
		return nil, err
	}
	return []string{call}, nil
}

// RenderCardGates renders each card in declared order as its pointer bullet with one indented sub-bullet per gate step.
// Each step is its own backticked command, never chained, so an agent runs each as its own Bash call.
func RenderCardGates(cards []planparser.Card, planDirDisplay string, gate gateslot.Config) (string, error) {
	var lines []string
	for _, c := range cards {
		lines = append(lines, fmt.Sprintf("- `%s`:", CardPointerPath(c, planDirDisplay)))
		steps, err := CardGateCommand(c, gate)
		if err != nil {
			return "", err
		}
		for _, step := range steps {
			lines = append(lines, fmt.Sprintf("  - `%s`", step))
		}
	}
	return strings.Join(lines, "\n"), nil
}

// RenderBatchGate renders the batch's gate as one line: the hub's `batch` gate call over every card of the batch.
// When last is set the call also runs the module's main packages,
// so the plan's last batch meets a break there before the plan-level verify does.
// It refuses, with the config's way forward, a hub whose `batch` list the config does not hold.
func RenderBatchGate(cards []planparser.Card, gate gateslot.Config, last bool) (string, error) {
	ids := make([]string, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.ID())
	}
	render := gate.AgentGateCall
	if last {
		render = gate.AgentGateCallWithMainPackages
	}
	call, err := render(gateslot.BatchGateKey, ids...)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("- `%s`", call), nil
}

// IsLastBatch reports whether batch is the last of the execution-ordered batches.
// batches is the run's recorded partition mapped onto the current plan, so only the run's last batch reads as last.
func IsLastBatch(batches []batcher.Batch, batch batcher.Batch) bool {
	if len(batches) == 0 {
		return false
	}
	lastNumber, _ := BatchIdentity(batches[len(batches)-1])
	number, _ := BatchIdentity(batch)
	return number == lastNumber
}
