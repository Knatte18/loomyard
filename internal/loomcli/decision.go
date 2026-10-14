// decision.go implements the `decision` loom subtree: the one verb, `add`, through which a design call made after the Discussion ended is recorded in the decision record.
// The run's parent, or an operator, uses it to answer a question the record does not settle, so Plan-Write and a later reader see the call beside the decisions the Discussion made.
// The verb appends one entry through `discussionparser.AppendDecision`, which re-runs the discussion check and restores the record on a finding, and then commits the record.
//
// The group carries its own PersistentPreRunE, and the loom parent's pre-run skips it, for the reason review.go gives.
// It resolves the target worktree through the same resolver the review group uses.
//
// The verb refuses a task session's label: a caller whose exported strand name differs from the run's recorded parent is refused, and the operator's own unnamed shell always passes.
// The one other caller is the run's own arbiter strand, which alone may pass `--by arbiter`; any other caller of that label is refused.
// The check reads only the exported strand name and the recorded parent name, so it guards a mistaken call, not a session that clears or forges the variable, and `--by` stays an unverified label.
// That is bounded by the entry being append-only, one entry per call, under an `Added after Discussion` heading with the claimed label and the date, in a committed diff.
//
// On a darn run the verb works once Darn-Write has validated and committed its record, and also while Darn-Write runs.
// The caller rule and the append-only, labelled, committed entry are unchanged.
// The one new edge is appending while the writer runs, bounded to a record the writer validated and has not touched.
// The working copy must equal the committed copy and hold every added entry its history held.
// After the commit the committed record must hold the appended entry, so an entry a writer write displaced is refused rather than reported.
//
// The verb never routes the run, never touches the support log or the plan, and an approved plan is not re-planned by it.

package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

const (
	decisionGroup = "decision"

	decisionByParent   = "parent"
	decisionByOperator = "operator"
	decisionByArbiter  = "arbiter"

	decisionWayNoRecord = `way forward: the record exists once Discussion-Write has written it; "lyx loom status <slug>" shows whether the run has reached that row, and "lyx loom start" begins or resumes the run; on a darn run the record exists once Darn-Write has validated it, and until then message the running Darn-Write session with the call or, with no writer live, record the call as a board note naming the task`
	decisionWayWriter   = `way forward: Discussion-Write owns the record while it runs; message the Discussion-Write session with the decision instead, or re-run once the row has handed off`

	decisionWayDarnWriter = `way forward: Darn-Write has not finished its record or is editing it after validating it; message the running Darn-Write session, and re-run "lyx loom decision add" once the committed record is unchanged`
	decisionWayEntryLost  = `way forward: a Darn-Write write landed during the append; message the running Darn-Write session to leave the record alone, then re-run "lyx loom decision add"`
	decisionWayNoHeading  = `way forward: restore the "## Decisions" heading in the decision record, then re-run the verb`
	decisionWayRetry      = `way forward: re-run "lyx loom decision add"; nothing is appended`
	decisionWayFindings   = `way forward: the record is restored; fix the named section and re-run the verb`
	decisionWayArbiter    = `way forward: the "arbiter" label is the run's own arbiter's; pass --by parent from the run's parent, or --by operator from the operator's own shell`
)

// decisionInput is the four flag values of one `decision add` call.
type decisionInput struct {
	by, title, decision, rationale string
}

// decisionDeps is every side effect decisionVerb performs, injected so tests supply fakes.
type decisionDeps struct {
	// strandName is the full name reed exported to the calling session, empty for a shell reed did not spawn.
	strandName string
	// parent resolves the run's recorded parent, called only for a named caller.
	parent func() (hubgeom.Parent, error)
	// arbiterStrand reports whether name is the target run's own arbiter strand, called only for a named caller.
	arbiterStrand func(name string) bool
	// readStatus reads the target worktree's default run status; found is false when the run has no status file.
	readStatus reviewStatusReader
	// recordPath is the decision record the entry lands in, reported on the success envelope.
	recordPath string
	// append adds d to the decision record and re-runs the discussion check, restoring the record on a finding or an error.
	append func(d discussionparser.AddedDecision) ([]discussionparser.Finding, error)
	// commit commits the discussion artifacts, the record included.
	commit func() error
	// now is the clock the entry's date comes from.
	now func() time.Time
	// darn reports whether the target's run is a darn run.
	darn func() (bool, error)
	// readRecord reads the decision record's working copy.
	readRecord func() ([]byte, error)
	// committed reads the decision record as committed at HEAD; found is false when HEAD holds no record.
	committed func() ([]byte, bool, error)
	// history returns every committed version of the decision record, newest first.
	history func() ([][]byte, error)
}

// checkDarnRecord returns the refusal text for a darn run's record that is not safe to append to, or empty when it is.
// The record is safe once the writer has validated and committed it and not touched it since: it exists, is committed, equals its committed copy and holds every added entry its history held.
// A failed read of any of the four is returned as an error.
func checkDarnRecord(deps decisionDeps) (string, error) {
	working, err := deps.readRecord()
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Sprintf("the run has no decision record at %s; %s", deps.recordPath, decisionWayNoRecord), nil
	}
	if err != nil {
		return "", err
	}
	committed, found, err := deps.committed()
	if err != nil {
		return "", err
	}
	if !found {
		return "the darn run's decision record is not committed yet; " + decisionWayDarnWriter, nil
	}
	if !bytes.Equal(working, committed) {
		return "the decision record differs from its committed copy; " + decisionWayDarnWriter, nil
	}
	versions, err := deps.history()
	if err != nil {
		return "", err
	}
	if findings := discussionparser.CheckAddedDecisionsKept(working, versions); len(findings) > 0 {
		return "the decision record is missing an entry its history held: " + strings.Join(renderFindings(findings), "; ") + "; " + decisionWayDarnWriter, nil
	}
	return "", nil
}

// committedHoldsEntry reports whether the record as committed at HEAD holds an `Added after Discussion` entry with the heading of added.
func committedHoldsEntry(deps decisionDeps, added discussionparser.AddedDecision) (bool, error) {
	committed, found, err := deps.committed()
	if err != nil || !found {
		return false, err
	}
	entries, err := discussionparser.ParseAddedDecisions(committed)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if addedDecisionHeading(entry) == addedDecisionHeading(added) {
			return true, nil
		}
	}
	return false, nil
}

// decisionVerb appends the decision in input to the record and commits it, or refuses with a way forward.
func decisionVerb(out io.Writer, slug string, deps decisionDeps, input decisionInput) int {
	refuse := func(format string, args ...any) int {
		return output.Err(out, fmt.Sprintf("loom: %s add: ", decisionGroup)+fmt.Sprintf(format, args...))
	}

	if input.by != decisionByParent && input.by != decisionByOperator && input.by != decisionByArbiter {
		return refuse("--by is %q; way forward: pass --by %s or --by %s", input.by, decisionByParent, decisionByOperator)
	}
	for _, flag := range []struct{ name, value string }{{"title", input.title}, {"decision", input.decision}, {"rationale", input.rationale}} {
		if strings.TrimSpace(flag.value) == "" {
			return refuse("--%s is empty; way forward: pass --%s with the text to record", flag.name, flag.name)
		}
	}

	if input.by == decisionByArbiter {
		if deps.strandName == "" || !deps.arbiterStrand(deps.strandName) {
			return refuse("the caller %q is not the run's arbiter; %s", deps.strandName, decisionWayArbiter)
		}
	} else if deps.strandName != "" {
		parent, err := deps.parent()
		if err != nil {
			return refuse("the caller %q is a named session; way forward: the run's parent could not be resolved (%s); the operator's own unnamed shell may still run the verb", deps.strandName, err)
		}
		if deps.strandName != parent.Name {
			if parent.Name == "" {
				return refuse("the caller %q is a task session; way forward: a design call after the Discussion is the operator's to record; the operator's own unnamed shell may always run it", deps.strandName)
			}
			return refuse("the caller %q is a task session; way forward: a design call after the Discussion is the parent's to record; ask %s to run the verb; the operator's own unnamed shell may always run it", deps.strandName, parent.Name)
		}
	}

	st, found, err := deps.readStatus()
	if err != nil {
		return refuse("read the run status: %s; %s", err, decisionWayRetry)
	}
	if found && st.State == shedengine.StateRunning && st.CurrentProducer == loomshed.NameDiscussionWrite {
		return refuse("%s is running and owns the decision record; %s", loomshed.NameDiscussionWrite, decisionWayWriter)
	}

	darn, err := deps.darn()
	if err != nil {
		return refuse("read the run's recipe: %s; %s", err, decisionWayRetry)
	}
	if darn {
		refusal, err := checkDarnRecord(deps)
		if err != nil {
			return refuse("read the darn run's decision record: %s; %s", err, decisionWayRetry)
		}
		if refusal != "" {
			return refuse("%s", refusal)
		}
	}

	added := discussionparser.AddedDecision{
		By:        input.by,
		Title:     input.title,
		Decision:  input.decision,
		Rationale: input.rationale,
		Date:      deps.now().UTC(),
	}
	findings, err := deps.append(added)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return refuse("the run has no decision record at %s; %s", deps.recordPath, decisionWayNoRecord)
	case errors.Is(err, discussionparser.ErrNoDecisionsHeading):
		return refuse("%s; %s", err, decisionWayNoHeading)
	case err != nil:
		return refuse("%s; %s", err, decisionWayRetry)
	case len(findings) > 0:
		return refuse("the discussion check flags the record after the append: %s; %s", strings.Join(renderFindings(findings), "; "), decisionWayFindings)
	}
	if err := deps.commit(); err != nil {
		return refuse("the entry is appended but the commit failed: %s; way forward: run \"lyx fabric commit\" to commit the record; a session lyx refuses the verb from reports status: FAILED and the orch runs it", err)
	}
	if darn {
		held, err := committedHoldsEntry(deps, added)
		if err != nil || !held {
			cause := "the committed record holds no entry"
			if err != nil {
				cause = "the committed record could not be read: " + err.Error()
			}
			return refuse("%s under %q; %s", cause, addedDecisionHeading(added), decisionWayEntryLost)
		}
	}
	return output.Ok(out, map[string]any{
		"slug":    slug,
		"record":  deps.recordPath,
		"heading": addedDecisionHeading(added),
	})
}

// addedDecisionHeading is the heading AppendDecision writes for d, without its leading marker.
func addedDecisionHeading(d discussionparser.AddedDecision) string {
	return fmt.Sprintf("Added after Discussion (%s, %s): %s", strings.TrimSpace(d.By), d.Date.UTC().Format("2006-01-02"), strings.TrimSpace(d.Title))
}

// decisionState carries what the decision group's pre-run resolved for its verb.
type decisionState struct {
	deps  decisionDeps
	slug  string
	input decisionInput
}

// preRun resolves the invoking cwd, then the target worktree, then the verb's side effects.
func (s *decisionState) preRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == decisionGroup {
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
	target, err := resolveReviewTarget(decisionGroup, cmd.Name(), location, slug, defaultReviewTargetDeps())
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	s.slug = target.WorktreeName
	recordPath := loomengine.DiscussionDecisionRecord(target)
	supportLogPath := loomengine.DiscussionSupportLog(target)
	s.deps = decisionDeps{
		strandName: os.Getenv(agentname.StrandNameEnv),
		parent:     func() (hubgeom.Parent, error) { return hubgeom.ResolveParent(target) },
		arbiterStrand: func(name string) bool {
			geometry, err := hubgeom.ReedGeometry(target)
			if err != nil {
				return false
			}
			parsed, err := agentname.Parse(name)
			return err == nil && parsed.Shortname == geometry.NameShortname && parsed.Slug == geometry.NameSlug && loomengine.IsArbiterStrand(name)
		},
		readStatus: circlingStatusReader(shedrun.StatusFile(target, shedrun.SelfRunID), shedrun.StatusLock(target, shedrun.SelfRunID)),
		recordPath: recordPath,
		append: func(d discussionparser.AddedDecision) ([]discussionparser.Finding, error) {
			return discussionparser.AppendDecision(recordPath, supportLogPath, d)
		},
		commit: func() error { return commitDiscussion(target) },
		now:    time.Now,
		darn: func() (bool, error) {
			seed, found, err := shedrun.ReadSeed(target, shedrun.SelfRunID)
			if err != nil {
				return false, err
			}
			return resolveSeedRecipe(seed, found) == shedrun.RecipeDarn, nil
		},
		readRecord: func() ([]byte, error) { return os.ReadFile(recordPath) },
		committed: func() ([]byte, bool, error) {
			return fabricengine.CommittedAnchoredFile(target, loomengine.DiscussionDecisionRecordRel())
		},
		history: discussionHistory(target),
	}
	return nil
}

// decisionCmd builds the `decision` subtree.
func (c *loomCLI) decisionCmd() *cobra.Command {
	st := &decisionState{}

	group := &cobra.Command{
		Use:   decisionGroup,
		Short: "record a design call made after the Discussion",
		Long: `decision holds the verb through which a design call made after the Discussion
ended is recorded in the decision record. "decision add" is the only verb.

The verb takes an optional slug naming the task worktree. A task worktree
addresses itself; from the prime the slug is required. It refuses a task
session's label: a caller whose reed strand name differs from the run's
recorded parent is refused, and the operator's own unnamed shell always passes.
The one other caller is the run's own arbiter strand, which passes --by arbiter.
--by is a label that is not verified.

Example:
  lyx loom decision add <slug> --by parent --title "..." --decision "..." --rationale "..."`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: st.preRun,
	}

	add := &cobra.Command{
		Use:         "add [<slug>]",
		Short:       "append one decision to the decision record and commit it",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `add appends one entry under an "Added after Discussion" heading, carrying the
--by label and today's date, to the end of the decision record's Decisions
section. It re-runs the discussion check over the record, restores the record
when the check flags it, and commits the record. It resumes nothing and
changes neither the support log nor the plan.

--by is parent, operator or arbiter; arbiter is the run's own arbiter strand's
label, refused from any other caller. --title, --decision and --rationale are
all required. It is refused for a named session other than the run's recorded
parent or, for --by arbiter, its arbiter: ask the parent to run it, or run it
from the operator's own shell. It is
refused while Discussion-Write is running, since that row owns the record;
message the Discussion-Write session with the decision instead.

On a darn run it works once Darn-Write has validated and committed its record,
including while Darn-Write runs. It then refuses a record that is missing,
not committed, differing from its committed copy or missing an entry its
history held, and a commit after which the committed record lacks the entry.
That bounds the append to a record the writer validated and has not touched,
and to an entry that reached the fabric.

Example:
  lyx loom decision add <slug> --by parent --title "Keep the cache" --decision "Keep the cache per run." --rationale "A shared cache races."`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), decisionVerb(cmd.OutOrStdout(), st.slug, st.deps, st.input))
			return nil
		},
	}
	add.Flags().StringVar(&st.input.by, "by", "", "who made the call: parent, operator or arbiter")
	add.Flags().StringVar(&st.input.title, "title", "", "the decision's heading title")
	add.Flags().StringVar(&st.input.decision, "decision", "", "what was decided")
	add.Flags().StringVar(&st.input.rationale, "rationale", "", "why it was decided")

	group.AddCommand(add)
	return group
}
