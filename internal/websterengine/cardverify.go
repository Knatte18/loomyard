// cardverify.go re-runs a recorded batch's cards' own "**Verify:**" commands on the worktree as it stands.
// It is the in-process evidence check behind every policy-only audit warning.

package websterengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// DefaultCardVerifyTimeout bounds each card's verify command when the caller passes a zero timeout.
const DefaultCardVerifyTimeout = 10 * time.Minute

// rerunCardVerifies runs each card's Verify through verifyrun.Run in worktree, in card order, each under its own timeout and with output discarded.
// It returns one line per failing card, `card NN-<slug> verify <command> <how>`, where <how> is `exited <code>`, `failed to start: <err>`, or `timed out after <timeout>`.
// A card with no Verify passes, and a zero timeout means DefaultCardVerifyTimeout.
// It returns no error: a verify that cannot start is a failed verify.
func rerunCardVerifies(cards []planparser.Card, worktree string, timeout time.Duration) []string {
	if timeout <= 0 {
		timeout = DefaultCardVerifyTimeout
	}
	var failures []string
	for _, card := range cards {
		command := strings.TrimSpace(card.Verify)
		if !card.HasVerify || command == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		code, err := verifyrun.Run(ctx, command, worktree, io.Discard)
		cancel()

		var how string
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			how = fmt.Sprintf("timed out after %s", timeout)
		case err != nil:
			how = fmt.Sprintf("failed to start: %v", err)
		case code != 0:
			how = fmt.Sprintf("exited %d", code)
		default:
			continue
		}
		failures = append(failures, fmt.Sprintf("card %02d-%s verify %s %s", card.Number, card.Slug, command, how))
	}
	return failures
}
