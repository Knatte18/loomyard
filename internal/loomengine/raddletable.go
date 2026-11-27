// raddletable.go implements RaddleTable, the table factory of the Raddle-Write row, and the raddle run's modes.
// RaddleTable is a pure composer: it resolves the raddle agent's model and names the file the agent ends on,
// and it returns the seatengine.Table that the row's seat producer runs.
// It does no spawning, polling or filesystem writing itself.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// raddleRole is the agent-name role that the Raddle-Write spawn carries.
const raddleRole = "raddle"

// raddleStencil is the stencil that opens the raddle agent's seat.
const raddleStencil = "raddle-template-agent"

// RaddleMode is how a raddle run gets the approval that files its proposal.
// Raddle-Write records it in the run's raddle state at its first call, and every later reader takes it from there.
type RaddleMode string

// The modes of a raddle run.
const (
	// RaddleModeAuto files the proposal once its gate passes, with no operator present.
	RaddleModeAuto RaddleMode = "auto"
	// RaddleModeInteractive files the proposal only on the operator's approval, given in the agent's pane.
	RaddleModeInteractive RaddleMode = "interactive"
	// RaddleModePerch files the proposal on the approval of the split perch the recipe's rows hold.
	// It allows only one task or one project under the run's own entry.
	RaddleModePerch RaddleMode = "perch"
)

// raddleModeRules returns the {{.mode_rules}} block of the raddle agent.
// In auto mode the agent ends on the complete proposal file with no operator present.
// In perch mode it does the same, and the text names the two outcomes the checks allow, with slug as the project's slug.
// In interactive mode it checks the proposal and puts it to the operator.
// It shows a table of the parts with their recipes, profiles and dependencies.
// It names the order departures with their costs, and says that the tasks land as drafts.
// The prose around the table keeps only the reasons for each recipe and profile and the alternatives set aside.
// It then records the operator's approval.
func raddleModeRules(mode RaddleMode, proposalPath, slug string) string {
	switch mode {
	case RaddleModeInteractive:
		return "This session is running interactively: an operator is present at the pane. " +
			"Once the proposal is complete, run `lym loom raddle-propose`, which checks the file and tells your parent that a proposal waits. " +
			"Then put the proposal to the operator in the pane as plain text. " +
			"Show the parts as a table with a column of each part's recipe, such as `darn`, a column of its profile, such as `solid`, " +
			"and a column of its dependencies, by part slug, so what each part runs as and the order read from the table alone. " +
			"In the text around the table, give only the reasons for each recipe and profile and the alternatives you set aside, never the recipe or profile again. " +
			"Name each order departure the proposal lists, with its cost. " +
			"Say that every task lands on the board as a draft, which the operator releases with `lym board set-status`, so a project can be made a beam before any of its tasks starts. " +
			"Revise the file while the operator questions or corrects the split, the order, the recipes or the profiles; run `lym loom raddle-propose` again after each revision. " +
			"Run `lym loom raddle-approve` only on the operator's explicit approval, never without it, and then end your turn."
	case RaddleModePerch:
		return "This session is running in perch mode: no operator is present, and the run's split perch reviews the proposal once you end. " +
			"Settle every question yourself from the Sketch, the board and the code, and end your turn on the complete proposal file at `" + proposalPath + "`. " +
			"The proposal ends in one of two outcomes, and the checks fail any other: " +
			"one task, as shape `tapestry` or as shape `tasks` with one part; " +
			"or a family, as shape `project` whose `project.slug` is `" + slug + "`, this run's own entry, under which every part is filed as a draft. " +
			"Never propose shape `beam`, or shape `tasks` with more than one part. " +
			"The proposal is checked when you end, and what it finds comes back to you."
	default:
		return "This session is running in auto mode: no operator is present. " +
			"Settle every question yourself from the Sketch, the board and the code, and end your turn on the complete proposal file at `" + proposalPath + "`. " +
			"The proposal is checked when you end, and what it finds comes back to you."
	}
}

// raddleReturnNote returns the {{.return_note}} block.
// It is empty on a first session;
// after a return it gives the reason the filing returned the proposal, with the file to start from.
// Auto and perch mode set the returned proposal aside at returnedPath;
// interactive mode keeps it at proposalPath with its approval cleared.
func raddleReturnNote(mode RaddleMode, returnReason, proposalPath, returnedPath string) string {
	if returnReason == "" {
		return ""
	}
	if mode != RaddleModeInteractive {
		return fmt.Sprintf("The filing returned your last proposal: %s\nIt is set aside at `%s`. Start from it, fix what the reason names, and write the corrected proposal at `%s`.", returnReason, returnedPath, proposalPath)
	}
	return fmt.Sprintf("The filing returned your last proposal and cleared its approval: %s\nThe proposal is still at `%s`. Start from it, fix what the reason names, run `lym loom raddle-propose`, and put the corrected proposal to the operator again.", returnReason, proposalPath)
}

// RaddleTable builds the seatengine.Table of one Raddle-Write run: one lead that writes the raddle proposal.
// The lead's model is cfg.Raddle resolved through reg, and the seat is interactive in interactive mode.
// Its one output is the approval record in interactive mode, written by `lym loom raddle-approve`.
// In auto and perch mode the output is the proposal file,
// so the session ends on the operator's approval or on the finished proposal.
// mode also selects the prompt's mode text.
// returnReason is the reason a Raddle-File return gave, empty on a first session;
// it fills the prompt's return note with the set-aside proposal's path.
// The opening stencil is read at call time from stencilsDir.
// The prompt points at the proposal format spec under specsDir.
// The seat engine renders the seat's parent directive from its geometry.
// The table carries no gate; the producer entry sets it.
func RaddleTable(layout *lymcwd.Location, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry, slug string, mode RaddleMode, returnReason string) (seatengine.Table, error) {
	if slug == "" {
		return seatengine.Table{}, fmt.Errorf("loom: RaddleTable: slug must not be empty")
	}
	choice, err := resolveModelChoice(cfg.Raddle, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: RaddleTable: raddle role model-spec: %w", err)
	}

	interactive := mode == RaddleModeInteractive
	output := RaddleProposal(layout)
	if interactive {
		output = RaddleApproval(layout)
	}
	return seatengine.Table{
		RolePrefix:  raddleRole,
		Segment:     segmentcolor.Discussion,
		Timeout:     time.Duration(cfg.DiscussionTimeoutMin) * time.Minute,
		Interactive: interactive,
		Values: map[string]string{
			"slug":                 slug,
			"specs_dir":            specsDir,
			"decision_record_path": DiscussionDecisionRecord(layout),
			"proposal_path":        RaddleProposal(layout),
			subagentModelKey:       cfg.SubagentModel,
			"mode_rules":           raddleModeRules(mode, RaddleProposal(layout), slug),
			"return_note":          raddleReturnNote(mode, returnReason, RaddleProposal(layout), RaddleReturnedProposal(layout)),
		},
		Optional: []string{"return_note"},
		Seats: []seatengine.Seat{{
			Stencil:     raddleStencil,
			ModelChoice: choice,
			Outputs:     []string{output},
			ConfigRole:  agentconfig.RoleRaddle,
			ModelKey:    shuttleengine.ModelKey{Module: loomModule, Key: "raddle"},
		}},
	}, nil
}
