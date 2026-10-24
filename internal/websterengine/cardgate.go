// cardgate.go derives each card's gate steps and the batch's gate, which begin-batch and recover-batch render into the implementer prompt's card_gates and batch_gate markers.
// Go derives the command and the fork runs it;
// Go never runs it, so a fork that skips it is caught by the plan-level verify gate.

package websterengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// cardGateCommand returns the gate for card as its ordered steps: the hub's `card` gate call over the card's own scope, then the comment line-break lint and the PATTERN headroom lint.
// It refuses, with the config's way forward, a hub whose `card` list the config does not hold.
func cardGateCommand(card planparser.Card, gate gateslot.Config) ([]string, error) {
	call, err := gate.GateCall("card", card.ID())
	if err != nil {
		return nil, err
	}
	return []string{call, "lyx loom lint-comments", "lyx loom lint-pattern"}, nil
}

// renderCardGates renders each card in declared order as its pointer bullet with one indented sub-bullet per gate step.
// Each step is its own backticked command, never chained, so a fork runs each as its own Bash call.
// Pointers are spelled as renderCardPointers spells them.
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
// It refuses, with the config's way forward, a hub whose `batch` list the config does not hold.
func renderBatchGate(cards []planparser.Card, gate gateslot.Config) (string, error) {
	ids := make([]string, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.ID())
	}
	call, err := gate.GateCall("batch", ids...)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("- `%s`", call), nil
}
