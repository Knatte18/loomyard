// circling.go implements the `circling` loom subtree: the two verbs through which the operator settles a review segment's escalation.
// `accept` ends the review loop on the current round and lets the run proceed; `continue` runs another round.
// Each records a decision file in the Bouncer's run directory and nothing else: neither resumes the run, so the operator runs `lyx loom resume` afterwards.
//
// The group carries its own PersistentPreRunE, and the loom parent's pre-run skips it, for the reason review.go gives.
// It resolves the target worktree through the same resolver the review group uses.
//
// The verbs check no caller identity, like `lyx loom review` and `lyx loom approve`:
// any shell addressing the run can record a decision, an agent's tmux session in the task worktree included.
// That is bounded by which stencils name the verbs, the Bouncer escalation brief and the arbiter stencil only;
// and by the refusal preconditions — the verbs refuse unless the run awaits at a Bouncer row with a pending escalation, the round carries an escalation the Bouncer wrote (a circling, hard-cap or budget escalation), one decision per round, and only while the run awaits at that Bouncer row.
// `continue` also refuses a round at or past its perch's hard cap, whoever asks, with `accept` or raising the cap as its way forward; `accept` is never refused by the cap.
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
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
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
	circlingWayNot       = `way forward: "lyx loom status <slug>" shows the run's state; the verbs apply only to a round the Bouncer escalated (a circling, hard-cap or budget escalation), and a plain CONTINUE round below the budget needs no decision`
	circlingWayHardCap   = `way forward: fix the file or key the message names, then re-run "lyx loom circling continue %s" in place of any "lyx loom resume" it names`
	circlingWayMalformed = `way forward: fix or delete the named escalation file, then run "lyx loom start", which re-escalates the round, and re-run the verb`
	circlingWayDecided   = `way forward: the decision for this round is recorded and stays; run "lyx loom resume" in the task worktree to resume the run`
	circlingWayRetry     = `way forward: re-run the verb; if the failure persists, fix the file or directory the message names`
	circlingWayRecipe    = `way forward: the embedded loom recipe failed to parse, so this lyx build is broken; rebuild or reinstall lyx, then re-run the verb`
)

// circlingDeps is every side effect circlingVerb performs, injected so tests supply fakes.
type circlingDeps struct {
	// readStatus reads the target worktree's default run status; found is false when the run has no status file.
	readStatus reviewStatusReader
	// bouncerSubdir reports the run subdirectory of the named recipe row when that row is a Bouncer.
	bouncerSubdir func(row string) (subdir string, isBouncer bool, err error)
	// record writes decision for the latest round of the Bouncer run directory named by subdir and returns that round and its escalation cause.
	record func(subdir string, decision shedadapters.CirclingDecision) (round int, cause shedadapters.EscalationCause, err error)
	// latestRound returns the latest round of the Bouncer run directory named by subdir.
	latestRound func(subdir string) (int, error)
	// hardCap returns the row's resolved hard cap, 0 meaning none, and the way forward that raises it.
	hardCap func(row string) (hardCap int, raise string, err error)
}

// circlingVerb records decision for the run's escalated round, or refuses with a way forward.
func circlingVerb(out io.Writer, slug string, deps circlingDeps, decision shedadapters.CirclingDecision) int {
	verb := string(decision)
	refuse := func(format string, args ...any) int {
		return output.Err(out, fmt.Sprintf("loom: %s %s: ", circlingGroup, verb)+fmt.Sprintf(format, args...))
	}

	st, found, err := deps.readStatus()
	if err != nil {
		return refuse("read the run status: %s; %s", err, circlingWayRetry)
	}
	if !found {
		return refuse("the run has no status file; %s", circlingWayNoRun)
	}
	if st.State != shedengine.StateAwaiting {
		return refuse("the run is %s at %s, not awaiting; %s", st.State, st.CurrentProducer, circlingWayNotAtHalt)
	}
	subdir, isBouncer, err := deps.bouncerSubdir(st.CurrentProducer)
	if err != nil {
		return refuse("%s; %s", err, circlingWayRecipe)
	}
	if !isBouncer {
		return refuse("the run is awaiting at %s, which is not a review segment's Bouncer row; %s", st.CurrentProducer, circlingWayNotAtHalt)
	}

	if decision == shedadapters.CirclingContinue {
		latest, err := deps.latestRound(subdir)
		if err != nil {
			return refuse("%s; %s", err, circlingWayRetry)
		}
		hardCap, raise, err := deps.hardCap(st.CurrentProducer)
		if err != nil {
			return refuse("%s; %s", err, fmt.Sprintf(circlingWayHardCap, slug))
		}
		if hardCap > 0 && latest >= hardCap {
			return refuse("round %d at %s is at or past its perch's hard cap %d, so no review round runs past it; way forward: run `lyx loom circling accept %s`, or raise the hard cap: %s, then run `lyx loom circling continue %s`", latest, st.CurrentProducer, hardCap, slug, raise, slug)
		}
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
		return refuse("%s; %s", err, circlingWayRetry)
	}
	return output.Ok(out, map[string]any{
		"slug":     slug,
		"decision": verb,
		"round":    round,
		"cause":    string(cause),
		"resume":   "lyx loom resume",
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
		latestRound: func(subdir string) (int, error) {
			return shedadapters.LatestRound(filepath.Join(reviewsDir, subdir))
		},
		hardCap: func(row string) (int, string, error) {
			return perchHardCap(target, row)
		},
	}
	return nil
}

// perchHardCap returns the resolved hard cap of the Bouncer row's perch and the way forward that raises it, by the layer its value comes from.
// A load failure of the file that holds the cap is returned as is.
func perchHardCap(location *lyxcwd.Location, row string) (int, string, error) {
	dirs := fabricengine.ConfigDirs(location)
	if row == loomshed.NameDarnBouncer {
		cfg, err := darnengine.LoadConfig(dirs, "darn")
		if err != nil {
			return 0, "", err
		}
		return cfg.ReviewHardMaxBounces, `run "lyx config darn --set review_hard_max_bounces=<n>" from the prime`, nil
	}
	perch, ok := loomPerches[row]
	if !ok {
		return 0, "", fmt.Errorf("loom: %s is a Bouncer row with no hard-cap source, so this lyx build is broken", row)
	}
	cfg, err := loomengine.LoadConfig(dirs, "loom")
	if err != nil {
		return 0, "", err
	}
	caps := cfg.Caps(perch)
	raise, err := loomengine.CapRoute(dirs, caps.HardKey)
	if err != nil {
		return 0, "", err
	}
	return caps.Hard, raise, nil
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
		return state.ReadJSONNoMkdir[shedengine.Status](statusPath, statusLock)
	}
}

// circlingCmd builds the `circling` subtree.
func (c *loomCLI) circlingCmd() *cobra.Command {
	st := &circlingState{}

	group := &cobra.Command{
		Use:   circlingGroup,
		Short: "settle a review segment's escalation: accept or continue",
		Long: `circling holds the verbs through which the operator settles a review segment
that escalated a round, because its judge found the rounds circling (a circling
escalation), because a design-wrong finding is still open at the perch's hard
cap (a hard-cap escalation), or because the review budget was spent and the
soft-cap rule could not settle the round (a budget escalation).

Each verb takes an optional slug naming the task worktree. A task worktree
addresses itself; from the prime the slug is required. A verb records the
decision for the escalated round, with the escalation's cause, and nothing
else: run "lyx loom resume" in the task worktree afterwards to resume the run.
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
			Use:         string(decision) + " [<slug>]",
			Short:       short,
			Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
			Long:        long,
			Args:        cobra.MaximumNArgs(1),
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
whether a circling, hard-cap or budget escalation. On resume the Bouncer
settles the segment as approved and the run proceeds to the next row. The
perch's hard cap never refuses it. It resumes nothing itself: run
"lyx loom resume" in the task worktree.

Example:
  lyx loom circling accept <slug>`)

	cont := verb(shedadapters.CirclingContinue, "run another review round after an escalation", `continue records the operator's decision to keep reviewing after the Bouncer
escalated a round, whether a circling, hard-cap or budget escalation. On
resume the Bouncer sends the segment back for another round. It runs one more
round only below the perch's hard cap and is refused at or past it, where
"accept" or raising the hard cap is the way forward. It resumes nothing
itself: run "lyx loom resume" in the task worktree.

Example:
  lyx loom circling continue <slug>`)

	group.AddCommand(accept, cont)
	return group
}
