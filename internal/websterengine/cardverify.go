// cardverify.go re-runs a recorded batch's cards' own "**Verify:**" commands on the worktree as it stands.
// It is the in-process evidence check behind every policy-only audit warning.

package websterengine

import (
	"io"

	"github.com/Knatte18/loomyard/internal/cardverify"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// rerunCardVerifies runs each card's Verify through runner, in card order, with output discarded.
// It returns one line per failing card, `card NN-<slug> verify <command> <how>`, as cardverify.Runner.RunCards defines it.
// A card with no Verify passes.
// It returns no error: a verify that cannot start is a failed verify.
func rerunCardVerifies(cards []planparser.Card, runner cardverify.Runner) []string {
	return runner.RunCards(cards, io.Discard, false)
}
