// cardgate.go derives each card's gate steps and the batch's gate.
// begin-batch and recover-batch render them into the implementer prompt's card_gates and batch_gate markers.
// The fork runs these steps, never Go itself, so a fork that skips them is caught by the plan-level verify gate.

package websterengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// cardGateCommand returns the gate for card as its ordered steps:
// the hub's `card` gate call over the card's own scope.
// The call also runs the lint steps that the `card` key lists.
// It refuses, with the config's way forward, a hub whose `card` list the config does not hold.
func cardGateCommand(card planparser.Card, gate gateslot.Config) ([]string, error) {
	call, err := gate.AgentGateCall("card", card.ID())
	if err != nil {
		return nil, err
	}
	return []string{call}, nil
}

// renderCardGates renders each card in declared order as its pointer bullet with one indented sub-bullet per gate step.
// Each step is its own backticked command, never chained, so a fork runs each as its own Bash call.
func renderCardGates(cards []planparser.Card, planDirDisplay string, gate gateslot.Config) (string, error) {
	var lines []string
	for _, c := range cards {
		lines = append(lines, fmt.Sprintf("- `%s`:", cardPointerPath(c, planDirDisplay)))
		steps, err := cardGateCommand(c, gate)
		if err != nil {
			return "", err
		}
		for _, step := range steps {
			lines = append(lines, fmt.Sprintf("  - `%s`", step))
		}
	}
	return strings.Join(lines, "\n"), nil
}

// renderBatchGate renders the batch's gate as one line: the hub's `batch` gate call over every card of the batch.
// When last is set the call also runs the module's main packages,
// so the plan's last fork meets a break there before the plan-level verify does.
// It refuses, with the config's way forward, a hub whose `batch` list the config does not hold.
func renderBatchGate(cards []planparser.Card, gate gateslot.Config, last bool) (string, error) {
	ids := make([]string, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.ID())
	}
	render := gate.AgentGateCall
	if last {
		render = gate.AgentGateCallWithMainPackages
	}
	call, err := render("batch", ids...)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("- `%s`", call), nil
}

// isLastBatch reports whether batch is the last of the execution-ordered batches.
func isLastBatch(batches []batcher.Batch, batch batcher.Batch) bool {
	if len(batches) == 0 {
		return false
	}
	lastNumber, _ := batchIdentity(batches[len(batches)-1])
	number, _ := batchIdentity(batch)
	return number == lastNumber
}
