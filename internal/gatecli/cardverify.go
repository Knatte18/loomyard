// cardverify.go implements `lym gate card-verify <card id>`, which runs one plan card's `**Verify:**` line.
// The line runs in this process, or in the detached runner an agent's capped call starts or joins.

package gatecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/cardverify"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/proc"
)

// cardVerifyCmd builds `lym gate card-verify <card id>` over seams, the bounds and launcher of a capped call's front.
func cardVerifyCmd(seams detachSeams) *cobra.Command {
	return &cobra.Command{
		Use:         "card-verify <card-id>",
		Short:       "run one plan card's **Verify:** line",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `card-verify reads the plan in this worktree, finds the card named by its number or its N-slug id, and runs the card's **Verify:** line through the same runner webster re-runs at record-batch.
A line that is one "lym gate test --wait" call runs as written, taking its own slots.
Any other line takes one gate slot of the hub's pool, waiting for it without a cap, and runs under the runner's timeout with the slot's environment.
Outside every hub the line runs unslotted.
An agent strand's call runs the line in a detached runner it starts or joins and answers done false with exit 75, no failure, once its keep-warm cap passes:
run the same command again in a turn that does nothing else, so the prompt cache stays warm; it prints the line's result once the line ended.
With no strand the line runs in this process.
The line's output passes through, and the exit code is the line's own; a line that timed out or could not start exits 1.
A card with no **Verify:** line passes with a message saying so.
Run it from the task worktree whose _lym/plan holds the card.

Examples:

  lym gate card-verify 4
  lym gate card-verify 4-gate-run-log`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runCardVerify(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0], seams))
			return nil
		},
	}
}

// cardTarget is the card a card-verify call names and where it runs.
type cardTarget struct {
	// cwd is the absolute directory the call resolves against.
	cwd string
	// location is the worktree whose plan holds the card.
	location *lymcwd.Location
	// inHub is whether location is in a hub.
	inHub bool
	// card is the card the call names.
	card planparser.Card
}

// findCard reads the plan of the worktree ctx's directory belongs to and returns the card that id names.
// A call that cannot name one returns the refusal's text with its way forward, and a zero target.
func findCard(ctx context.Context, id string) (target cardTarget, refusal string) {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return cardTarget{}, fmt.Sprintf("gate card-verify: cannot read the working directory: %v; way forward: re-run the same command", err)
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return cardTarget{}, fmt.Sprintf("gate card-verify: %v; way forward: re-run the same command", err)
	}
	if !inHub {
		location, err = lymcwd.ResolveWorktree(cwd)
		if errors.Is(err, lymcwd.ErrNotAGitRepo) {
			return cardTarget{}, fmt.Sprintf("gate card-verify: %s is in no git worktree, so it holds no plan; way forward: run it from the task worktree whose _lym/plan holds the card", cwd)
		}
		if err != nil {
			return cardTarget{}, fmt.Sprintf("gate card-verify: resolve the worktree of %s: %v; way forward: re-run the same command", cwd, err)
		}
	}

	planDir := planparser.PlanDir(location.AnchorPath())
	if _, err := os.Stat(planparser.PlanOverview(location.AnchorPath())); errors.Is(err, fs.ErrNotExist) {
		return cardTarget{}, fmt.Sprintf("gate card-verify: this worktree holds no plan at %s; way forward: run it from the task worktree whose _lym/plan holds the card", planDir)
	}
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		return cardTarget{}, fmt.Sprintf("gate card-verify: cannot read the plan at %s: %v; way forward: fix the plan, then re-run the same command", planDir, err)
	}
	index := slices.IndexFunc(plan.Cards, func(card planparser.Card) bool { return cardMatches(card, id) })
	if index < 0 {
		known := make([]string, len(plan.Cards))
		for i, card := range plan.Cards {
			known[i] = card.ID()
		}
		return cardTarget{}, fmt.Sprintf("gate card-verify: %q names no card of the plan; way forward: pass one of %s, then re-run the same command", id, strings.Join(known, ", "))
	}
	return cardTarget{cwd: cwd, location: location, inHub: inHub, card: plan.Cards[index]}, ""
}

// line returns the card's `**Verify:**` line, empty when the card has none.
func (t cardTarget) line() string {
	if !t.card.HasVerify {
		return ""
	}
	return strings.TrimSpace(t.card.Verify)
}

// runner returns the card-verify runner over the card's worktree, holding the hub's pool and wait directory inside a hub.
func (t cardTarget) runner() cardverify.Runner {
	runner := cardverify.Runner{Worktree: t.location.WorktreePath()}
	if t.inHub {
		runner.Slots = hubgeom.GateSlots(t.location)
		runner.WaitDir = gateslot.WaitDir(t.location.AnchorPath())
	}
	return runner
}

// runNoLine answers that the card has no line to run.
func (t cardTarget) runNoLine(out io.Writer) int {
	return output.Ok(out, map[string]any{"message": fmt.Sprintf("card %s has no **Verify:** line; nothing to run", t.card.ID())})
}

// runCardVerify runs the card named id and returns the verb's exit code:
// the line's own, 1 for a refusal or a line that did not exit, and 0 for a pass.
// A capped call, which gives the verb no wait, starts or joins a detached runner of the line.
// It then answers as a front does.
func runCardVerify(ctx context.Context, out, stderr io.Writer, id string, seams detachSeams) int {
	target, refusal := findCard(ctx, id)
	if refusal != "" {
		return output.Err(out, refusal)
	}
	line := target.line()
	if line == "" {
		return target.runNoLine(out)
	}
	if call, capped := frontDecision(target.location.WorktreePath(), false, seams); capped {
		call.verb, call.words, call.cwd = verbCardVerify, []string{target.card.ID()}, target.cwd
		return runFront(ctx, out, stderr, call, seams)
	}
	return runCardLine(out, target.card, target.runner().RunLine(ctx, line, out))
}

// runCardLine closes a card-verify run whose line ended as how says, empty for a pass.
// It prints `gate card-verify: card <id> verify <line> <how>` for a failure.
// It returns the line's own exit code, 1 for a failure without one, and 0 for a pass.
func runCardLine(out io.Writer, card planparser.Card, how string) int {
	if how == "" {
		return 0
	}
	fmt.Fprintf(out, "gate card-verify: card %s verify %s %s\n", card.ID(), strings.TrimSpace(card.Verify), how)
	if code, ok := strings.CutPrefix(how, "exited "); ok {
		if parsed, err := strconv.Atoi(code); err == nil && parsed != 0 {
			return parsed
		}
	}
	return 1
}

// runCard runs the card-verify request in the run directory d and returns the verb's exit code.
// Its answers go to stdout and stderr as the in-process verb gives them.
// It reads the plan again in the request's directory.
// A plan edited since the front's read is refused, with the refusal in stderr.
// The line runs under ctx, which the run's lease and a terminating signal cancel;
// its phase goes to the run's phase file and its process tree to lineFile.
func (d detachRun) runCard(ctx context.Context, request detachRequest, stdout, stderr io.Writer) int {
	target, refusal := findCard(ctx, request.Args[0])
	if refusal != "" {
		printLine(stderr, refusal)
		return 1
	}
	line := target.line()
	if line == "" {
		return target.runNoLine(stdout)
	}

	// The job holds the whole tree of the line, so the runner's death ends it where no parent-death signal exists.
	release, err := proc.HoldNamedJob("lym-gate-" + filepath.Base(d.dir))
	if err != nil {
		logger.Warn("gate run-detached: could not hold the run's job, so the line is not tied to the runner", "dir", d.dir, "cause", err)
	} else {
		defer func() {
			if err := release(); err != nil {
				logger.Warn("gate run-detached: could not release the run's job", "dir", d.dir, "cause", err)
			}
		}()
	}

	runner := target.runner()
	runner.Phase = func(slotWait bool) {
		state := phaseRunning
		if slotWait {
			state = phaseSlots
		}
		d.writePhase(runPhase{State: state, Line: line})
	}
	runner.Tied = d.writeLine
	return runCardLine(stdout, target.card, runner.RunLine(ctx, line, stdout))
}

// writeLine records the process tree of the run's line.
// It logs a failure, since the record only serves a later front's cleanup.
func (d detachRun) writeLine(tree proc.TreeRecord) {
	data, err := json.Marshal(tree)
	if err == nil {
		err = fsx.AtomicWriteBytes(d.path(lineFile), data)
	}
	if err != nil {
		logger.Warn("gate run-detached: could not write the line's process tree", "dir", d.dir, "cause", err)
	}
}
