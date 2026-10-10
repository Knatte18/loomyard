// cardverify.go re-runs a recorded batch's cards' own "**Verify:**" commands on the worktree as it stands.
// It is the in-process evidence check behind every policy-only audit warning.

package websterengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// DefaultCardVerifyTimeout bounds each card's verify command when the caller passes a zero timeout.
const DefaultCardVerifyTimeout = 10 * time.Minute

// rerunCardVerifies runs each card's Verify through verifyrun.Run in worktree, in card order, each under its own timeout and with output discarded.
// It returns one line per failing card, `card NN-<slug> verify <command> <how>`, where <how> is `exited <code>`, `failed to start: <err>`, `could not acquire a gate slot: <err>`, or `timed out after <timeout>`.
// A card with no Verify passes, and a zero timeout means DefaultCardVerifyTimeout.
// It returns no error: a verify that cannot start is a failed verify.
//
// A non-nil slots pool gates each card's run: a wait record naming the site `card verify NN-<slug>` sits in waitDir while the acquire waits, the timeout counts only from the moment the slot is held, the command runs with the lease's environment, and the slot is released after it.
// A nil slots runs unslotted with the parent's environment, and an empty waitDir writes no wait record.
func rerunCardVerifies(cards []planparser.Card, worktree string, timeout time.Duration, slots *gateslot.Pool, waitDir string) []string {
	if timeout <= 0 {
		timeout = DefaultCardVerifyTimeout
	}
	var failures []string
	for _, card := range cards {
		command := strings.TrimSpace(card.Verify)
		if !card.HasVerify || command == "" {
			continue
		}
		site := fmt.Sprintf("card verify %02d-%s", card.Number, card.Slug)
		how := runCardVerify(command, worktree, timeout, slots, waitDir, site)
		if how == "" {
			continue
		}
		failures = append(failures, fmt.Sprintf("card %02d-%s verify %s %s", card.Number, card.Slug, command, how))
	}
	return failures
}

// runCardVerify runs one card's command and returns how it failed, empty when it passed.
func runCardVerify(command, worktree string, timeout time.Duration, slots *gateslot.Pool, waitDir, site string) string {
	var env []string
	if slots != nil {
		lease, err := acquireCardVerifySlot(slots, worktree, waitDir, site)
		if err != nil {
			return fmt.Sprintf("could not acquire a gate slot: %v", err)
		}
		defer func() {
			if err := lease.Release(); err != nil {
				logger.Warn("webster: release gate slot", "site", site, "cause", err)
			}
		}()
		env = lease.Env(os.Environ())
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	code, err := verifyrun.Run(ctx, command, worktree, env, io.Discard)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("timed out after %s", timeout)
	case err != nil:
		return fmt.Sprintf("failed to start: %v", err)
	case code != 0:
		return fmt.Sprintf("exited %d", code)
	}
	return ""
}

// acquireCardVerifySlot acquires a slot for site, keeping a wait record in waitDir until the acquire returns.
// A wait record that cannot be written is logged and skipped, since it only feeds the status display.
func acquireCardVerifySlot(slots *gateslot.Pool, worktree, waitDir, site string) (*gateslot.Lease, error) {
	if waitDir != "" {
		waitPath, err := gateslot.WriteWait(waitDir, gateslot.Wait{Site: site, PID: os.Getpid(), Started: time.Now()})
		if err != nil {
			logger.Warn("webster: write gate wait record", "site", site, "cause", err)
		} else {
			defer os.Remove(waitPath)
		}
	}
	return slots.Acquire(context.Background(), gateslot.Holder{Worktree: worktree, Site: site})
}
