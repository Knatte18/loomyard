// runner.go runs a card's `**Verify:**` lines: each line through the platform shell, in card order.

package cardverify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// DefaultTimeout bounds a slot-held line when no timeout is set.
const DefaultTimeout = 10 * time.Minute

// gateCallPrefix holds the words every `lym gate test` call starts with.
var gateCallPrefix = []string{"lym", "gate", "test"}

// waitFlag is the gate flag that lets a `lym gate test` line block for its slots itself, outside any timeout.
const waitFlag = "--wait"

// controlOperators are the shell constructs that make a line more than one `lym gate test` call.
var controlOperators = []string{";", "&", "|", "\n", "`", "$(", "<", ">"}

// Runner runs a card's `**Verify:**` lines in a worktree.
type Runner struct {
	// Worktree is the directory every line runs in.
	Worktree string
	// Slots is the gate-slot pool a slot-held line takes one slot from;
	// nil runs every line unslotted with the parent's environment.
	Slots *gateslot.Pool
	// WaitDir is the directory a slot-held line writes its wait record into while it waits; empty writes none.
	WaitDir string
	// Timeout bounds a slot-held line from the moment its slot is held; zero means DefaultTimeout.
	Timeout time.Duration
	// Phase, when set, is called with true right before a slot-held line acquires its slot.
	// It is called with false right before any line runs, so a caller can tell the slot wait from the run.
	Phase func(slotWait bool)
	// Tied, when set, makes a line run tied to the calling process.
	// It is called with the line's process tree once the line runs, so a caller can record the tree.
	Tied func(proc.TreeRecord)
}

// RunLine runs one `**Verify:**` line through verifyrun in the worktree, writing its output to out.
// It returns how the line failed, or the empty string when it passed.
// Cancelling ctx during the slot wait fails the line; during the run it ends the line's process group.
// A failure reads `exited <code>`, `failed to start: <err>`, or `timed out after <timeout>`.
// A failed slot wait reads `could not acquire a gate slot: <err>`.
// A `lym gate test` call that holds `--wait` runs unslotted and outside the timeout.
// The gate's own pre-tier ceiling and tier timeouts bound it.
// Any other line takes one slot of the pool and waits for it without a cap.
// Its timeout counts from the moment the slot is held.
// It runs with the environment that gateslot.HolderEnv returns.
// Neither sees gateslot.PrebuiltLymEnv: a line's own go test builds lym once per test binary.
func (r Runner) RunLine(ctx context.Context, line string, out io.Writer) string {
	return r.runLine(ctx, strings.TrimSpace(line), "card verify", out)
}

// runLine runs line with site as the label of its slot holder and wait record.
func (r Runner) runLine(ctx context.Context, line, site string, out io.Writer) string {
	env := slices.DeleteFunc(gateslot.StripPrebuilt(os.Environ()), func(entry string) bool { return strings.HasPrefix(entry, agentname.StrandNameEnv+"=") })
	if IsGateCall(line) {
		r.phase(false)
		return r.execute(ctx, line, env, out, 0)
	}
	if r.Slots == nil {
		r.phase(false)
		return r.execute(ctx, line, env, out, r.timeout())
	}
	r.phase(true)
	lease, err := r.acquire(ctx, site)
	if err != nil {
		return fmt.Sprintf("could not acquire a gate slot: %v", err)
	}
	defer func() {
		if err := lease.Release(); err != nil {
			logger.Warn("cardverify: release gate slot", "site", site, "cause", err)
		}
	}()
	limits, err := r.Slots.Limits()
	if err != nil {
		return fmt.Sprintf("could not acquire a gate slot: read gate limits: %v", err)
	}
	r.phase(false)
	return r.execute(ctx, line, gateslot.HolderEnv(env, lease, limits.Tier0Parallel), out, r.timeout())
}

func (r Runner) phase(slotWait bool) {
	if r.Phase != nil {
		r.Phase(slotWait)
	}
}

// RunCards runs each card's `**Verify:**` value in card order, writing its output to out.
// It returns one line per failing card, `card NN-<slug> verify <command> <how>`, where <how> is how the line failed.
// A card with no `**Verify:**` passes, and stopAtFirst ends the run at the first failing card.
// It returns no error: a line that cannot start is a failed line.
func (r Runner) RunCards(ctx context.Context, cards []planparser.Card, out io.Writer, stopAtFirst bool) []string {
	var failures []string
	for _, card := range cards {
		command := strings.TrimSpace(card.Verify)
		if !card.HasVerify || command == "" {
			continue
		}
		label := fmt.Sprintf("%02d-%s", card.Number, card.Slug)
		how := r.runLine(ctx, command, "card verify "+label, out)
		if how == "" {
			continue
		}
		failures = append(failures, fmt.Sprintf("card %s verify %s %s", label, command, how))
		if stopAtFirst {
			break
		}
	}
	return failures
}

// IsGateCall reports whether line is exactly one `lym gate test` call that holds `--wait` before any `--` and no shell control operator.
// Such a line blocks for its slots itself, so a runner leaves it unslotted and unbounded.
func IsGateCall(line string) bool {
	for _, operator := range controlOperators {
		if strings.Contains(line, operator) {
			return false
		}
	}
	words := strings.Fields(line)
	if len(words) < len(gateCallPrefix) {
		return false
	}
	for i, word := range gateCallPrefix {
		if words[i] != word {
			return false
		}
	}
	for _, word := range words[len(gateCallPrefix):] {
		switch word {
		case "--":
			return false
		case waitFlag:
			return true
		}
	}
	return false
}

func (r Runner) timeout() time.Duration {
	if r.Timeout <= 0 {
		return DefaultTimeout
	}
	return r.Timeout
}

// execute runs line under env and returns how it failed, empty when it passed; a zero bound runs without one.
func (r Runner) execute(ctx context.Context, line string, env []string, out io.Writer, bound time.Duration) string {
	if bound > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, bound)
		defer cancel()
	}
	var code int
	var err error
	if r.Tied != nil {
		code, err = verifyrun.RunTied(ctx, line, r.Worktree, env, out, r.Tied)
	} else {
		code, err = verifyrun.Run(ctx, line, r.Worktree, env, out)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("timed out after %s", bound)
	case err != nil:
		return fmt.Sprintf("failed to start: %v", err)
	case code != 0:
		return fmt.Sprintf("exited %d", code)
	}
	return ""
}

// acquire takes one slot for site, keeping a wait record in the wait directory until the acquire returns.
// A wait record that cannot be written is logged and skipped, since it only feeds the status display.
func (r Runner) acquire(ctx context.Context, site string) (*gateslot.Lease, error) {
	if r.WaitDir != "" {
		waitPath, err := gateslot.WriteWait(r.WaitDir, gateslot.Wait{Site: site, PID: os.Getpid(), Started: time.Now()})
		if err != nil {
			logger.Warn("cardverify: write gate wait record", "site", site, "cause", err)
		} else {
			defer os.Remove(waitPath)
		}
	}
	return r.Slots.Acquire(ctx, gateslot.Holder{Worktree: r.Worktree, Site: site})
}
