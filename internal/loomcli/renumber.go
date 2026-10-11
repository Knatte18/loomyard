// renumber.go implements the `insert-card`, `remove-card` and `restore-renumber` loom verbs: the sanctioned way for an agent editing a plan to add or drop one card and renumber the cards after it, and to undo an interrupted renumber.
// Each cobra shell is thin over a verb function that takes its side effects as renumberDeps, so the refusal table is testable without a worktree or a webster run.

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
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// renumberDeps is every side effect the card verbs perform, injected so tests supply fakes and watch the lease.
type renumberDeps struct {
	// planDir and journalPath locate the plan directory and the renumber journal.
	planDir     string
	journalPath string
	// ops is the raw file seam the renumber performs every file operation through.
	ops planparser.RenumberIO
	// loadState reads webster's run record, nil when no run has started.
	loadState func() (*websterengine.State, error)
	// acquireLease takes webster's state-mutation lease and returns its release.
	acquireLease func() (release func() error, err error)
}

// errRenumberRawIO marks a failed run-record read or lease acquisition, which joins the raw I/O refusal.
var errRenumberRawIO = errors.New("raw I/O failed")

// begunBoundError marks a position at or before the last begun card.
type begunBoundError struct {
	position int
	bound    int
}

func (e begunBoundError) Error() string {
	return fmt.Sprintf("position %d is at or before card %d, the last card webster has begun", e.position, e.bound)
}

// newRenumberDeps builds the real side effects of the card verbs over the anchor the loom CLI resolved.
func (c *loomCLI) newRenumberDeps() renumberDeps {
	anchor := c.env.AnchorPath
	websterDir := websterengine.Dir(anchor)
	scratchDir := websterengine.ScratchDir(anchor)
	return renumberDeps{
		planDir:     planparser.PlanDir(anchor),
		journalPath: planparser.RenumberJournalPath(anchor),
		ops:         planparser.OSRenumberIO(),
		loadState: func() (*websterengine.State, error) {
			return websterengine.LoadState(websterDir, scratchDir)
		},
		acquireLease: func() (func() error, error) {
			lease, err := websterengine.AcquireStateMutation(scratchDir)
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
		return output.Err(out, renumberRefusal(verb, fmt.Errorf("%w: take webster's state-mutation lease: %w", errRenumberRawIO, err)))
	}
	result, err := func() (*planparser.RenumberResult, error) {
		number, err := strconv.Atoi(position)
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not an integer", planparser.ErrRenumberPosition, position)
		}
		st, err := d.loadState()
		if err != nil {
			return nil, fmt.Errorf("%w: read webster's run record: %w", errRenumberRawIO, err)
		}
		if bound := websterengine.LastBegunCard(st); number <= bound {
			return nil, begunBoundError{position: number, bound: bound}
		}
		return renumber(number)
	}()
	// A failed release leaves a lock file the process exit frees, and the renumber's own outcome is the verb's result.
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
		return output.Err(out, renumberRefusal(verb, fmt.Errorf("%w: take webster's state-mutation lease: %w", errRenumberRawIO, err)))
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
	{planparser.ErrRenumberJournalExists, "run lyx loom restore-renumber, then re-run the verb"},
	{planparser.ErrRenumberReversalFailed, "run lyx loom restore-renumber"},
	{planparser.ErrRenumberPlanInconsistent, "run lyx loom validate-plan and fix its index-file-mismatch and card-numbering findings, then re-run the verb"},
	{planparser.ErrRenumberPosition, "pass a position from first_card to the last card, or one past it for insert-card"},
	{planparser.ErrRenumberOnlyCard, "insert the replacement card first, then remove"},
	{planparser.ErrRenumberSlug, "pass a slug of lowercase letters and digits joined by single hyphens"},
	{planparser.ErrRenumberIntent, "pass a non-empty one-line --intent"},
	{planparser.ErrRenumberNoJournal, "nothing to restore, lyx loom insert-card or lyx loom remove-card is the verb to run"},
	{planparser.ErrRenumberJournalInvalid, "delete the named journal file, then run lyx loom validate-plan, whose findings name any card left inconsistent"},
	{planparser.ErrRenumberRestoreFailed, "transient, re-run lyx loom restore-renumber, the journal is kept"},
	{planparser.ErrRenumberIOFailed, "transient, re-run the refused verb; nothing is mutated"},
	{errRenumberRawIO, "transient, re-run the refused verb; nothing is mutated"},
}

// renumberRefusal words one refusal of a card verb as `loom: <verb>: <cause>; way forward: <step>`.
// It is the one place the card verbs' refusals are worded, so planparser never names a lyx verb.
// An error that is none of the known refusals is a plan that does not parse, and that is the way forward its message takes.
func renumberRefusal(verb string, err error) string {
	cause := strings.TrimPrefix(err.Error(), "planparser: ")
	step := "fix the file the message names so lyx loom validate-plan parses it, then re-run the verb"

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
// The prose mentions are lines naming a moved card by its old number, which the agent checks by hand.
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
	mentions := make([]map[string]any, 0, len(res.ProseMentions))
	for _, p := range res.ProseMentions {
		mentions = append(mentions, map[string]any{"file": p.File, "line": p.Line, "text": p.Text})
	}

	envelope := map[string]any{
		"plan_dir":       planDir,
		"moved":          moved,
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
Edit or Write, and its Card Index line carries --intent. Lines that name a
moved card by its old number are reported under prose_mentions and never
rewritten, so check each by hand.

N runs from first_card to one past the last card, and must come after the last
card webster has begun. The call is journaled, and an interrupted one is
undone by lyx loom restore-renumber. It never commits and never approves.

Example:
  lyx loom insert-card 3 add-retry --intent "retry a failed fetch once"`,
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
number. Lines that name a moved card by its old number are reported under
prose_mentions and never rewritten, so check each by hand.

N runs from first_card to the last card, must come after the last card webster
has begun, and cannot be the plan's only card. The call is journaled, and an
interrupted one is undone by lyx loom restore-renumber. It never commits and
never approves.

Example:
  lyx loom remove-card 4`,
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
  lyx loom restore-renumber`,
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
