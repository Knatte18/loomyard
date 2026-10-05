// circling.go implements the `circling` loom subtree: the two verbs through which the operator settles a review segment's escalation.
// `accept` ends the review loop on the current round and lets the run proceed; `continue` runs another round.
// Each records a decision file in the Bouncer's run directory and nothing else: neither resumes the run, so the operator runs `lyx loom start` afterwards, like `lyx loom approve`.
//
// The group carries its own PersistentPreRunE, and the loom parent's pre-run skips it, for the reason review.go gives.
// It resolves the target worktree through the same resolver the review group uses.
//
// The verbs check no caller identity, like `lyx loom review` and `lyx loom approve`:
// any shell addressing the run can record a decision, an agent's tmux session in the task worktree included.
// That is bounded by the refusal preconditions alone — the round carries an escalation the Bouncer wrote (a CIRCLING judgement or a spent review budget), one decision per round, and only while the run awaits at that Bouncer row — and by no agent stencil naming the verbs.
// The decision file in the committed run directory records the decision and the escalation's cause for that round.

package loomcli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

const (
	circlingGroup        = "circling"
	circlingWayNoRun     = `way forward: "lyx loom start" in the task worktree begins the run; an escalation exists only once a review segment's Bouncer has escalated a round`
	circlingWayNotAtHalt = `way forward: "lyx loom status <slug>" shows where the run is; the verbs apply only while the run is awaiting at a review segment's Bouncer row after an escalation`
	circlingWayNot       = `way forward: "lyx loom status <slug>" shows the run's state; the verbs apply only to a round the Bouncer escalated (a CIRCLING judgement or a spent review budget), and a plain CONTINUE round below the budget needs no decision`
	circlingWayMalformed = `way forward: fix or delete the named escalation file, then run "lyx loom start", which re-escalates the round, and re-run the verb`
	circlingWayDecided   =`way forward: the decision for this round is recorded and stays; run "lyx loom start" in the task worktree to resume the run`
)

// circlingDeps is every side effect circlingVerb performs, injected so tests supply fakes.
type circlingDeps struct {
	// readStatus reads the target worktree's default run status; found is false when the run has no status file.
	readStatus reviewStatusReader
	// bouncerSubdir reports the run subdirectory of the named recipe row when that row is a Bouncer.
	bouncerSubdir func(row string) (subdir string, isBouncer bool, err error)
	// record writes decision for the latest round of the Bouncer run directory named by subdir and returns that round and its escalation cause.
	record func(subdir string, decision shedadapters.CirclingDecision) (round int, cause shedadapters.EscalationCause, err error)
}

// circlingVerb records decision for the run's escalated round, or refuses with a way forward.
func circlingVerb(out io.Writer, slug string, deps circlingDeps, decision shedadapters.CirclingDecision) int {
	verb := string(decision)
	refuse := func(format string, args ...any) int {
		return output.Err(out, fmt.Sprintf("loom: %s %s: ", circlingGroup, verb)+fmt.Sprintf(format, args...))
	}

	st, found, err := deps.readStatus()
	if err != nil {
		return refuse("read the run status: %s", err)
	}
	if !found {
		return refuse("the run has no status file; %s", circlingWayNoRun)
	}
	if st.State != shedengine.StateAwaiting {
		return refuse("the run is %s at %s, not awaiting; %s", st.State, st.CurrentProducer, circlingWayNotAtHalt)
	}
	subdir, isBouncer, err := deps.bouncerSubdir(st.CurrentProducer)
	if err != nil {
		return refuse("%s", err)
	}
	if !isBouncer {
		return refuse("the run is awaiting at %s, which is not a review segment's Bouncer row; %s", st.CurrentProducer, circlingWayNotAtHalt)
	}

	round, cause, err := deps.record(subdir, decision)
	switch {
	case errors.Is(err, shedadapters.ErrNotEscalated):
		return refuse("the latest judged round at %s is not escalated; %s", st.CurrentProducer, circlingWayNot)
	case errors.Is(err, shedadapters.ErrCirclingDecided):
		return refuse("a decision is already recorded for the escalated round at %s; %s", st.CurrentProducer, circlingWayDecided)
	case errors.Is(err, shedadapters.ErrEscalationMalformed):
		return refuse("%s; %s", err, circlingWayMalformed)
	case err != nil:
		return refuse("%s", err)
	}
	return output.Ok(out, map[string]any{
		"slug":     slug,
		"decision": verb,
		"round":    round,
		"cause":    string(cause),
		"resume":   "lyx loom start",
	})
}

// circlingState carries what the circling group's pre-run resolved for its verbs.
type circlingState struct {
	deps circlingDeps
	slug string
}

// preRun resolves the invoking cwd, then the target worktree, then the verbs' side effects.
func (s *circlingState) preRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == circlingGroup {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	slug := ""
	if len(args) >= 1 {
		slug = args[0]
	}
	target, err := resolveReviewTarget(circlingGroup, cmd.Name(), location, slug, defaultReviewTargetDeps())
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	s.slug = target.WorktreeName
	statusPath := shedrun.StatusFile(target, shedrun.SelfRunID)
	statusLock := shedrun.StatusLock(target, shedrun.SelfRunID)
	reviewsDir := loomengine.LoomReviewsDir(target)
	s.deps = circlingDeps{
		readStatus:    circlingStatusReader(statusPath, statusLock),
		bouncerSubdir: loomrecipe.BouncerRunSubdir,
		record: func(subdir string, decision shedadapters.CirclingDecision) (int, shedadapters.EscalationCause, error) {
			return shedadapters.RecordCirclingDecision(filepath.Join(reviewsDir, subdir), decision)
		},
	}
	return nil
}

// circlingStatusReader reads the status file at statusPath under the lock at statusLock.
// Absence is decided by the committed status file alone:
// the lock sits in the untracked scratch tree, which a fresh checkout of a task branch lacks even when its run is awaiting,
// so a missing lock directory is created rather than read as a missing run.
func circlingStatusReader(statusPath, statusLock string) reviewStatusReader {
	return func() (shedengine.Status, bool, error) {
		if _, err := os.Stat(statusPath); errors.Is(err, fs.ErrNotExist) {
			return shedengine.Status{}, false, nil
		}
		if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
			return shedengine.Status{}, false, fmt.Errorf("create the status lock directory: %w", err)
		}
		return state.ReadJSONStrict[shedengine.Status](statusPath, statusLock)
	}
}

// circlingCmd builds the `circling` subtree.
func (c *loomCLI) circlingCmd() *cobra.Command {
	st := &circlingState{}

	group := &cobra.Command{
		Use:   circlingGroup,
		Short: "settle a review segment's escalation: accept or continue",
		Long: `circling holds the verbs through which the operator settles a review segment
that escalated a round, either because its judge found the rounds circling or
because the review budget was spent without convergence.

Each verb takes an optional slug naming the task worktree. A task worktree
addresses itself; from the prime the slug is required. A verb records the
decision for the escalated round, with the escalation's cause, and nothing
else: run "lyx loom start" in the task worktree afterwards to resume the run.
The verbs check no caller identity; a second decision for the same round is
refused.

Example:
  lyx loom circling accept <slug>
  lyx loom circling continue <slug>`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: st.preRun,
	}

	verb := func(decision shedadapters.CirclingDecision, short, long string) *cobra.Command {
		return &cobra.Command{
			Use:   string(decision) + " [slug]",
			Short: short,
			Long:  long,
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if clihelp.ShouldAbort(cmd.Context()) {
					return nil
				}
				clihelp.SetExit(cmd.Context(), circlingVerb(cmd.OutOrStdout(), st.slug, st.deps, decision))
				return nil
			},
		}
	}

	accept := verb(shedadapters.CirclingAccept, "accept the escalated review loop as it stands and let the run proceed", `accept records the operator's acceptance of the round the Bouncer escalated,
whether it judged the rounds circling or spent the review budget. On resume
the Bouncer settles the segment as approved and the run proceeds to the next
row. It resumes nothing itself: run "lyx loom start" in the task worktree.

Example:
  lyx loom circling accept <slug>`)

	cont := verb(shedadapters.CirclingContinue, "run another review round after an escalation", `continue records the operator's decision to keep reviewing after the Bouncer
escalated a round, whether it judged the rounds circling or spent the review
budget. On resume the Bouncer sends the segment back for another round; after
a budget escalation that grants exactly one more round. It resumes nothing
itself: run "lyx loom start" in the task worktree.

Example:
  lyx loom circling continue <slug>`)

	group.AddCommand(accept, cont)
	return group
}
