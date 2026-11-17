// renumber.go implements the `insert-card`, `remove-card` and `restore-renumber` loom verbs.
// They are the sanctioned way for an agent to add or drop one card of a plan and renumber the cards after it.
// They also undo an interrupted renumber.
// Each cobra command is a thin shell over a verb function that takes its side effects as renumberDeps.
// That keeps the refusal table testable without a worktree or a run.

package loomcli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/spf13/cobra"
)

// renumberDeps holds every side effect the card verbs perform, so tests can supply fakes and observe the lease.
type renumberDeps struct {
	// planDir and journalPath locate the plan directory and the renumber journal.
	planDir     string
	journalPath string
	// ops performs every file operation of the renumber, so tests can substitute a fake.
	ops planparser.RenumberIO
	// loadState reads the active implementer's run-plan state, nil when no run has started.
	loadState func() (*runplan.State, error)
	// acquireLease takes the active implementer's state-mutation lease and returns its release.
	acquireLease func() (release func() error, err error)
}

// errRenumberRawIO marks a failed run-record read or lease acquisition, which joins the raw I/O refusal.
var errRenumberRawIO = errors.New("raw I/O failed")

// errRenumberSeam marks a refusal of the active-implementer seam, whose own text carries the way forward.
var errRenumberSeam = errors.New("the run's active implementer cannot be read")

// seamRefusal is a refusal of the active-implementer seam that matches errRenumberSeam.
// It words itself as the seam's refusal alone, so the message does not repeat the sentinel's text.
type seamRefusal struct{ cause error }

// Error returns the seam's refusal text.
func (e seamRefusal) Error() string { return e.cause.Error() }

// Unwrap returns the seam's refusal.
func (e seamRefusal) Unwrap() error { return e.cause }

// Is reports whether target is errRenumberSeam.
func (e seamRefusal) Is(target error) bool { return target == errRenumberSeam }

// rawIORefusal words err, a failed run-record read or lease acquisition, as a raw I/O refusal naming what failed.
// A refusal of the seam stays as it is, since it is not raw I/O.
func rawIORefusal(what string, err error) error {
	if errors.Is(err, errRenumberSeam) {
		return err
	}
	return fmt.Errorf("%w: %s: %w", errRenumberRawIO, what, err)
}

// begunBoundError marks a position at or before the last begun card.
type begunBoundError struct {
	position int
	bound    int
}

func (e begunBoundError) Error() string {
	return fmt.Sprintf("position %d is at or before card %d, the last card the implementer has begun", e.position, e.bound)
}

// newRenumberDeps builds the real side effects of the card verbs over the anchor and seam the loom CLI resolved.
func (c *loomCLI) newRenumberDeps() renumberDeps {
	return renumberDepsFor(c.env.AnchorPath, c.env.ImplementerSeam)
}

// renumberDepsFor builds the card verbs' side effects over anchor and the active-implementer seam active.
// The run-plan state and the state-mutation lease are the active implementer's.
// A renumber and a batch begin of that implementer therefore serialize on one lease.
// The implementer is resolved before the lease is taken, and a refusal of the seam is a seamRefusal.
func renumberDepsFor(anchor string, active weaverengine.ActiveDeps) renumberDeps {
	return renumberDeps{
		planDir:     planparser.PlanDir(anchor),
		journalPath: planparser.RenumberJournalPath(anchor),
		ops:         planparser.OSRenumberIO(),
		loadState: func() (*runplan.State, error) {
			_, st, err := weaverengine.Active(active)
			if err != nil {
				return nil, seamRefusal{cause: err}
			}
			return st, nil
		},
		acquireLease: func() (func() error, error) {
			implementer, _, err := weaverengine.Active(active)
			if err != nil {
				return nil, seamRefusal{cause: err}
			}
			_, scratchDir := weaverengine.RunDirs(implementer, anchor)
			lease, err := runplan.AcquireStateMutation(scratchDir)
			if err != nil {
				return nil, err
			}
			return lease.Release, nil
		},
	}
}

// insertCardVerb inserts a heading-only card at position and renumbers the cards from there up.
// It returns the process exit code and emits exactly one envelope on out.
func insertCardVerb(out io.Writer, d renumberDeps, position, slug, intent string) int {
	return runCardVerb(out, d, "insert-card", position, func(number int) (*planparser.RenumberResult, error) {
		return planparser.InsertCard(d.ops, d.planDir, d.journalPath, number, slug, intent)
	})
}

// removeCardVerb removes the card at position and renumbers the cards after it.
// It returns the process exit code and emits exactly one envelope on out.
func removeCardVerb(out io.Writer, d renumberDeps, position string) int {
	return runCardVerb(out, d, "remove-card", position, func(number int) (*planparser.RenumberResult, error) {
		return planparser.RemoveCard(d.ops, d.planDir, d.journalPath, number)
	})
}

// runCardVerb holds the lease around the begun-bound check and the renumber, and releases it before it prints.
func runCardVerb(out io.Writer, d renumberDeps, verb, position string, renumber func(number int) (*planparser.RenumberResult, error)) int {
	release, err := d.acquireLease()
	if err != nil {
		return output.Err(out, renumberRefusal(verb, rawIORefusal("take the active implementer's state-mutation lease", err)))
	}
	result, err := func() (*planparser.RenumberResult, error) {
		number, err := strconv.Atoi(position)
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not an integer", planparser.ErrRenumberPosition, position)
		}
		st, err := d.loadState()
		if err != nil {
			return nil, rawIORefusal("read the active implementer's run record", err)
		}
		if bound := runplan.LastBegunCard(st); number <= bound {
			return nil, begunBoundError{position: number, bound: bound}
		}
		return renumber(number)
	}()
	// A failed release leaves a lock file the process exit frees,
	// and the renumber's own outcome is the verb's result.
	_ = release()
	if err != nil {
		return output.Err(out, renumberRefusal(verb, err))
	}
	return output.Ok(out, renumberEnvelope(d.planDir, result))
}

// restoreRenumberVerb undoes an interrupted renumber from its journal.
// It returns the process exit code and emits exactly one envelope on out.
func restoreRenumberVerb(out io.Writer, d renumberDeps) int {
	const verb = "restore-renumber"
	release, err := d.acquireLease()
	if err != nil {
		return output.Err(out, renumberRefusal(verb, rawIORefusal("take the active implementer's state-mutation lease", err)))
	}
	result, err := planparser.RestoreRenumber(d.ops, d.planDir, d.journalPath)
	_ = release()
	if err != nil {
		return output.Err(out, renumberRefusal(verb, err))
	}
	return output.Ok(out, map[string]any{
		"plan_dir": d.planDir,
		"journal":  d.journalPath,
		"restored": nonNil(result.Restored),
		"removed":  nonNil(result.Removed),
	})
}

// nonNil returns names, or an empty slice for nil, so the envelope renders an empty list rather than null.
func nonNil(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}

// renumberWaysForward maps each refusal of a card verb onto the step its message names, most specific first.
var renumberWaysForward = []struct {
	sentinel error
	step     string
}{
	{planparser.ErrRenumberJournalExists, "run lym loom restore-renumber, then re-run the verb"},
	{planparser.ErrRenumberReversalFailed, "run lym loom restore-renumber"},
	{planparser.ErrRenumberPlanInconsistent, "run lym loom validate-plan and fix its index-file-mismatch and card-numbering findings, then re-run the verb"},
	{planparser.ErrRenumberPosition, "pass a position from first_card to the last card, or one past it for insert-card"},
	{planparser.ErrRenumberOnlyCard, "insert the replacement card first, then remove"},
	{planparser.ErrRenumberSlug, "pass lowercase letters and digits joined by single hyphens"},
	{planparser.ErrRenumberIntent, "pass a non-empty one-line --intent"},
	{planparser.ErrRenumberNoJournal, "nothing to restore, lym loom insert-card or lym loom remove-card is the verb to run"},
	{planparser.ErrRenumberJournalInvalid, "delete the named journal file, then run lym loom validate-plan, whose findings name any card left inconsistent"},
	{planparser.ErrRenumberRestoreFailed, "transient, re-run lym loom restore-renumber, the journal is kept"},
	{planparser.ErrRenumberIOFailed, "transient, re-run the refused verb; nothing is mutated"},
	{errRenumberSeam, "take the way forward the message names, then re-run the verb"},
	{errRenumberRawIO, "transient, re-run the refused verb; nothing is mutated"},
}

// renumberRefusal words one refusal of a card verb as `loom: <verb>: <cause>; way forward: <step>`.
// It is the one place the card verbs' refusals are worded, so planparser never names a lym verb.
// An error that matches none of the known refusals is a plan that does not parse.
// Its way forward names the file to fix.
func renumberRefusal(verb string, err error) string {
	cause := strings.TrimPrefix(err.Error(), "planparser: ")
	step := "fix the file the message names so lym loom validate-plan parses it, then re-run the verb"

	var bound begunBoundError
	if errors.As(err, &bound) {
		step = fmt.Sprintf("a begun card is frozen, so insert or remove after card %d", bound.bound)
	} else {
		for _, row := range renumberWaysForward {
			if errors.Is(err, row.sentinel) {
				step = row.step
				break
			}
		}
	}
	return "loom: " + verb + ": " + cause + "; way forward: " + step
}

// renumberEnvelope is the success envelope of insert-card and remove-card.
// The rewritten refs are the card ids and bare card references that the renumber moved to their new numbers.
// The prose mentions are lines left with a bare reference and no faithful rewrite; the agent fixes them by hand.
func renumberEnvelope(planDir string, res *planparser.RenumberResult) map[string]any {
	moved := make([]map[string]any, 0, len(res.Moved))
	for _, m := range res.Moved {
		moved = append(moved, map[string]any{
			"old_number": m.OldNumber,
			"new_number": m.NewNumber,
			"old_file":   m.OldFile,
			"new_file":   m.NewFile,
		})
	}
	rewritten := make([]map[string]any, 0, len(res.RewrittenRefs))
	for _, r := range res.RewrittenRefs {
		rewritten = append(rewritten, map[string]any{"file": r.File, "line": r.Line, "from": r.From, "to": r.To})
	}
	mentions := make([]map[string]any, 0, len(res.ProseMentions))
	for _, p := range res.ProseMentions {
		mentions = append(mentions, map[string]any{"file": p.File, "line": p.Line, "text": p.Text})
	}

	envelope := map[string]any{
		"plan_dir":       planDir,
		"moved":          moved,
		"rewritten_refs": rewritten,
		"prose_mentions": mentions,
	}
	if len(res.Created) > 0 {
		envelope["created"] = res.Created[0]
	}
	if len(res.Removed) > 0 {
		envelope["removed"] = res.Removed[0]
	}
	return envelope
}

// insertCardCmd builds the `insert-card` subcommand.
func (c *loomCLI) insertCardCmd() *cobra.Command {
	var intent string
	cmd := &cobra.Command{
		Use:         "insert-card <position> <slug> --intent <intent>",
		Short:       "insert a heading-only card at position N and renumber the cards after it",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `insert-card adds a card numbered N and moves every card from N up by one:
its file name prefix, its heading number, its explicit Commit: prefix and its
Card Index number. The new card is a heading-only stub the agent fills with
Edit or Write, and its Card Index line carries --intent. A card id naming a
moved card, <N>-<slug> or its zero-padded file form, and a bare reference
naming one, such as "card 4" or "cards 4-6", are rewritten to the new number
in the overview and every card file and reported under rewritten_refs. Every
reference is read as the numbering before the call. A bare range the new card
falls inside is left as is and its line reported under prose_mentions, so fix
each by hand.

N runs from first_card to one past the last card, and must come after the last
card the active implementer has begun. The call is journaled, and an interrupted one is
undone by lym loom restore-renumber. It never commits and never approves.

Example:
  lym loom insert-card 3 add-retry --intent "retry a failed fetch once"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), insertCardVerb(cmd.OutOrStdout(), c.newRenumberDeps(), args[0], args[1], intent))
			return nil
		},
	}
	cmd.Flags().StringVar(&intent, "intent", "", "the new card's one-line Card Index text")
	_ = cmd.MarkFlagRequired("intent")
	return cmd
}

// removeCardCmd builds the `remove-card` subcommand.
func (c *loomCLI) removeCardCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "remove-card <position>",
		Short:       "remove card N and renumber the cards after it",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `remove-card deletes card N and moves every later card down by one: its file
name prefix, its heading number, its explicit Commit: prefix and its Card Index
number. A card id naming a moved card, <N>-<slug> or its zero-padded file
form, and a bare reference naming one, such as "card 5" or "cards 5-7", are
rewritten to the new number in the overview and every card file and reported
under rewritten_refs. Every reference is read as the numbering before the
call. A bare reference naming card N, or a range card N falls inside, is left
as is and its line reported under prose_mentions, so fix each by hand.

N runs from first_card to the last card, must come after the last card the
active implementer has begun, and cannot be the plan's only card. The call is journaled, and an
interrupted one is undone by lym loom restore-renumber. It never commits and
never approves.

Example:
  lym loom remove-card 4`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), removeCardVerb(cmd.OutOrStdout(), c.newRenumberDeps(), args[0]))
			return nil
		},
	}
}

// restoreRenumberCmd builds the `restore-renumber` subcommand.
func (c *loomCLI) restoreRenumberCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "restore-renumber",
		Short:       "undo an interrupted insert-card or remove-card from its journal",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `restore-renumber undoes an insert-card or remove-card that was interrupted:
it removes every file the call created, writes every file it rewrote or
deleted back whole, and removes the journal last. It discards any edit made
to those files since the interrupted call, the heading-only stub's body
included. A re-run after a failed restore completes it. It never commits and
never approves.

Example:
  lym loom restore-renumber`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), restoreRenumberVerb(cmd.OutOrStdout(), c.newRenumberDeps()))
			return nil
		},
	}
}
