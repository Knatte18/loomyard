// raddleverbs.go implements the `raddle-propose` and `raddle-approve` loom verbs.
// The first checks the raddle agent's proposal; the second records the operator's approval of it.
// Both are role verbs of the Raddle-Write row and write the run's state record;
// Raddle-File reads the approval it records.

package loomcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// raddleWriteEngine is the engine of the recipe row that writes the raddle proposal.
const raddleWriteEngine = "RaddleWrite"

// raddleVerbDeps carries the seams and paths that both raddle verbs read and write through.
type raddleVerbDeps struct {
	// row answers the name of the recipe's RaddleWrite row, and false when the recipe has none.
	row func() (string, bool, error)
	// currentRow answers the status file's current producer, empty when the run has no status file.
	currentRow func() (string, error)
	// check carries the proposal and decision record paths and the board context builder.
	check loomshed.RaddleCheck
	// statePath and stateLockPath are the run's state record and its lock.
	statePath     string
	stateLockPath string
	// coldPath is where the first passing proposal is stored; later passes leave it alone.
	coldPath string
	// approvalPath is the approval record the approve verb writes.
	approvalPath string
	// sketch is the slug of the Sketch the run splits.
	sketch string
	// notify posts the parent orch a line.
	notify func(line string) error
}

// raddleVerbFailure reports a seam that could not be read or written as an error envelope that names a re-run.
func raddleVerbFailure(out io.Writer, verb, what string, err error) int {
	return output.Err(out, fmt.Sprintf("loom: %s: %s: %s; way forward: run lym loom %s again, and tell the operator when it keeps failing", verb, what, err, verb))
}

// raddleFindingLines renders each finding as one line of an envelope's findings list.
func raddleFindingLines(findings []raddleparser.Finding) []string {
	lines := make([]string, len(findings))
	for i, finding := range findings {
		lines[i] = loomshed.FormatRaddleFindings([]raddleparser.Finding{finding})
	}
	return lines
}

// refuseOutsideRaddleWrite refuses, with the way forward, a run whose recipe has no RaddleWrite row;
// it refuses a run whose current row is not that row too.
// It returns the exit code and true when it refused or could not read the rows;
// it returns false when the run stands at the row.
func refuseOutsideRaddleWrite(out io.Writer, verb string, deps raddleVerbDeps) (int, bool) {
	row, hasRow, err := deps.row()
	if err != nil {
		return raddleVerbFailure(out, verb, "read the run's recipe rows", err), true
	}
	if !hasRow {
		return output.Err(out, fmt.Sprintf("loom: %s: this run's recipe has no Raddle-Write row, so it has no raddle proposal; way forward: none is needed, since only a raddle run proposes; `lym loom status` shows where the run stands", verb)), true
	}

	current, err := deps.currentRow()
	if err != nil {
		return raddleVerbFailure(out, verb, "read the run's current row", err), true
	}
	if current != row {
		if current == "" {
			current = "none"
		}
		return output.Err(out, fmt.Sprintf("loom: %s: the run's current row is %s, not %s; way forward: run lym loom %s while the run stands at %s; lym loom status shows the current row", verb, current, row, verb, row)), true
	}
	return 0, false
}

// raddleProposeVerb checks the proposal on disk and reports a JSON envelope naming the result, returning the exit code.
// It refuses, each with its way forward, a recipe with no RaddleWrite row and a run whose current row is not that row.
// Findings come back as an error envelope carrying them, and store nothing.
// A passing call stores the proposal as the cold proposal when none is stored yet,
// and it records the session as proposed.
// On the first pass of a Raddle-Write session it also posts the parent orch that a proposal waits for the operator.
// A failed notice leaves the session without one, so the next passing call posts it again;
// raddle-approve refuses until it is posted.
// A perch-mode run posts no notice, since its split perch, not the operator, reviews the proposal.
func raddleProposeVerb(out io.Writer, deps raddleVerbDeps) int {
	const verb = "raddle-propose"
	if code, refused := refuseOutsideRaddleWrite(out, verb, deps); refused {
		return code
	}

	findings, err := loomshed.CheckRaddleProposal(deps.check, false)
	if err != nil {
		return raddleVerbFailure(out, verb, "check the proposal", err)
	}
	if len(findings) > 0 {
		return output.ErrFields(out, "loom: raddle-propose: the proposal is not yet valid; way forward: fix each finding in the proposal file, then run lym loom raddle-propose again", map[string]any{
			"findings": raddleFindingLines(findings),
		})
	}

	if err := loomshed.StoreColdProposal(deps.check.ProposalPath, deps.coldPath, deps.statePath, deps.stateLockPath); err != nil {
		return raddleVerbFailure(out, verb, "store the cold proposal", err)
	}
	needsNotice := false
	recorded, err := loomshed.UpdateRaddleState(deps.statePath, deps.stateLockPath, func(stored *loomshed.RaddleState) {
		stored.ProposedSession = stored.Session
		needsNotice = stored.Mode != loomengine.RaddleModePerch && stored.NoticeSession < stored.Session
	})
	if err != nil {
		return raddleVerbFailure(out, verb, "record the passing proposal", err)
	}
	if needsNotice {
		line := fmt.Sprintf("raddle: the proposal for the Sketch %s passes its checks and waits for the operator's approval in the run's pane; it is at %s", deps.sketch, deps.check.ProposalPath)
		if err := deps.notify(line); err != nil {
			return raddleVerbFailure(out, verb, "post the parent the waiting notice", err)
		}
		if _, err := loomshed.UpdateRaddleState(deps.statePath, deps.stateLockPath, func(stored *loomshed.RaddleState) { stored.NoticeSession = recorded.Session }); err != nil {
			return raddleVerbFailure(out, verb, "record the waiting notice", err)
		}
	}
	note := "the proposal passes its checks; put it to the operator and run lym loom raddle-approve only on the operator's explicit approval"
	if recorded.Mode == loomengine.RaddleModePerch {
		note = "the proposal passes its checks; end your turn on it, and the run's split perch reviews it"
	}
	return output.Ok(out, map[string]any{
		"valid":         true,
		"session":       recorded.Session,
		"notice_posted": needsNotice,
		"note":          note,
	})
}

// raddleApproveVerb approves the proposal on disk and reports a JSON envelope naming the approval.
// It returns the exit code.
// It refuses, each with its way forward:
// a recipe with no RaddleWrite row;
// a run whose current row is not that row;
// a run in perch mode, whose split perch's approval files the proposal;
// a Raddle-Write session with no passing propose;
// a session whose waiting notice was not posted;
// a proposal that is not valid;
// or a board that moved since the proposal was checked.
// On success it records the hash of the proposal file with the session in the run's state;
// then it writes the approval record, and names the board entries that landed meanwhile.
// The verb cannot tell the operator from the agent,
// so an approval with no operator input is prevented only by the stencil.
func raddleApproveVerb(out io.Writer, deps raddleVerbDeps) int {
	const verb = "raddle-approve"
	if code, refused := refuseOutsideRaddleWrite(out, verb, deps); refused {
		return code
	}

	stored, err := loomshed.ReadRaddleState(deps.statePath, deps.stateLockPath)
	if err != nil {
		return raddleVerbFailure(out, verb, "read the run's state", err)
	}
	if stored.Mode == loomengine.RaddleModePerch {
		return output.Err(out, "loom: raddle-approve: this run is in perch mode, where the approval of the run's split perch files the proposal and no operator approval is recorded; way forward: none is needed: end your turn on the finished proposal, and the split perch reviews it")
	}
	if stored.ProposedSession == 0 || stored.ProposedSession < stored.Session {
		return output.Err(out, fmt.Sprintf("loom: raddle-approve: no proposal has passed its checks in this Raddle-Write session (session %d); way forward: run lym loom raddle-propose, fix what it finds, put the proposal to the operator, then run lym loom raddle-approve", stored.Session))
	}

	if stored.NoticeSession < stored.Session {
		return output.Err(out, fmt.Sprintf("loom: raddle-approve: the waiting notice of this Raddle-Write session (session %d) was not posted, so nothing has told the operator that a proposal waits; way forward: run lym loom raddle-propose again, which posts the waiting notice, then run lym loom raddle-approve on the operator's approval", stored.Session))
	}

	findings, err := loomshed.CheckRaddleProposal(deps.check, false)
	if err != nil {
		return raddleVerbFailure(out, verb, "check the proposal", err)
	}
	if len(findings) > 0 {
		return output.ErrFields(out, "loom: raddle-approve: the proposal is not valid; way forward: fix each finding in the proposal file, run lym loom raddle-propose, and put the revised proposal to the operator", map[string]any{
			"findings": raddleFindingLines(findings),
		})
	}
	recheck, err := loomshed.RecheckRaddleBoard(deps.check)
	if err != nil {
		return raddleVerbFailure(out, verb, "re-check the board", err)
	}
	if len(recheck.Failures) > 0 {
		return output.ErrFields(out, "loom: raddle-approve: the board moved since the proposal was checked; way forward: read the named entries again, revise the proposal to fit them, run lym loom raddle-propose, and put the revised proposal to the operator", map[string]any{
			"findings": raddleFindingLines(recheck.Failures),
		})
	}

	proposal, err := os.ReadFile(deps.check.ProposalPath)
	if err != nil {
		return raddleVerbFailure(out, verb, "read the proposal", err)
	}
	approval := loomshed.RaddleApprovalRecord{Hash: raddleparser.Hash(proposal), Session: stored.Session}
	if _, err := loomshed.UpdateRaddleState(deps.statePath, deps.stateLockPath, func(current *loomshed.RaddleState) { current.Approval = &approval }); err != nil {
		return raddleVerbFailure(out, verb, "record the approval", err)
	}
	record, err := json.MarshalIndent(approval, "", "  ")
	if err != nil {
		return raddleVerbFailure(out, verb, "render the approval record", err)
	}
	if err := fsx.AtomicWriteBytes(deps.approvalPath, append(record, '\n')); err != nil {
		return raddleVerbFailure(out, verb, "write the approval record", err)
	}
	return output.Ok(out, map[string]any{
		"approved": true,
		"hash":     approval.Hash,
		"session":  approval.Session,
		"landed":   append([]string{}, recheck.Landed...),
	})
}

// raddleVerbDeps assembles the seams of the raddle verbs for the run that c has armed.
// Every closure reads or writes on demand and opens nothing at build time.
func (c *loomCLI) raddleVerbDeps() raddleVerbDeps {
	return raddleVerbDeps{
		row: func() (string, bool, error) { return loomrecipe.EngineRow(c.recipe, raddleWriteEngine) },
		currentRow: func() (string, error) {
			status, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(c.location, shedrun.SelfRunID), shedrun.StatusLock(c.location, shedrun.SelfRunID))
			// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
			if errors.Is(err, fs.ErrNotExist) || (err == nil && !found) {
				return "", nil
			}
			return status.CurrentProducer, err
		},
		check:         raddleCheckFor(c.location),
		statePath:     loomengine.RaddleStatePath(c.location),
		stateLockPath: loomengine.RaddleStateLockPath(c.location),
		coldPath:      loomengine.RaddleColdProposal(c.location),
		approvalPath:  loomengine.RaddleApproval(c.location),
		sketch:        seedSlug(c.location.WorktreeName),
		notify: func(line string) error {
			_, err := orchcli.NotifyParent(c.location, loomProgram, line)
			return err
		},
	}
}

// raddleVerbCmd builds the subcommand for a raddle verb.
// Running it calls verb with the command's output and the deps of the armed run.
// It then sets the exit code from verb's result.
func (c *loomCLI) raddleVerbCmd(use, short, long string, verb func(io.Writer, raddleVerbDeps) int) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long:        long,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), verb(cmd.OutOrStdout(), c.raddleVerbDeps()))
			return nil
		},
	}
}

// raddleProposeCmd builds the `raddle-propose` subcommand.
func (c *loomCLI) raddleProposeCmd() *cobra.Command {
	return c.raddleVerbCmd("raddle-propose", "check the raddle proposal and mark it ready for the operator", `raddle-propose runs the checks of the raddle gate over the proposal file
in the run's raddle directory. The raddle agent runs it in the task
worktree once the proposal is complete, in interactive mode, before it puts
the proposal to the operator. On a pass it stores the run's first passing
proposal as the cold proposal, records the session as proposed, and posts the
parent orch that a proposal waits for the operator, once per Raddle-Write
session; a perch-mode run, whose split perch reviews the proposal, posts no
such notice. Findings come back in the envelope. It takes no arguments, and is
refused while the run stands at another row or its recipe has no Raddle-Write
row.

Example:
  lym loom raddle-propose`, raddleProposeVerb)
}

// raddleApproveCmd builds the `raddle-approve` subcommand.
func (c *loomCLI) raddleApproveCmd() *cobra.Command {
	return c.raddleVerbCmd("raddle-approve", "record the operator's approval of the raddle proposal", `raddle-approve records the approval of the proposal file as it stands. The
raddle agent runs it only on the operator's explicit approval. It is refused
until a raddle-propose has passed and posted its waiting notice in the current
Raddle-Write session, while
the proposal fails a check, and when the board moved since the proposal was
checked; the refusal names what to fix. It is refused in perch mode, where the
approval of the run's split perch files the proposal. On success it records the
hash of the proposal with the session and names the board entries that landed
since the proposal read them. It takes no arguments.

Example:
  lym loom raddle-approve`, raddleApproveVerb)
}
