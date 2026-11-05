// raddletable.go implements RaddleTable, the Raddle-Write row's table factory: a pure composer that resolves the raddle agent's model, names the file it ends on and returns the seatengine.Table the row's seat producer runs.
// It does no spawning, polling or filesystem writing itself.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// raddleRole is the agent-name role this module's Raddle-Write spawn carries.
const raddleRole = "raddle"

// raddleStencil is the stencil that opens the raddle agent's seat.
const raddleStencil = "raddle-template-agent"

// raddleSkills are the skills the Raddle-Write spawn loads, in order.
var raddleSkills = []string{"scribe:prose", "scribe:conversation"}

// raddleModeRules returns the {{.mode_rules}} block of the raddle agent: in auto mode the agent ends on the complete proposal file with no operator present, and in interactive mode it checks the proposal, puts it to the operator and records the operator's approval.
func raddleModeRules(interactive bool, proposalPath string) string {
	if !interactive {
		return "This session is running in auto mode: no operator is present. " +
			"Settle every question yourself from the Sketch, the board and the code, and end your turn on the complete proposal file at `" + proposalPath + "`. " +
			"The proposal is checked when you end, and what it finds comes back to you."
	}
	return "This session is running interactively: an operator is present at the pane. " +
		"Once the proposal is complete, run `lym loom raddle-propose`, which checks the file and tells your parent that a proposal waits. " +
		"Then put the proposal to the operator in the pane as plain text, and revise the file while the operator questions or corrects the split, the order, the recipes or the profiles; run `lym loom raddle-propose` again after each revision. " +
		"Run `lym loom raddle-approve` only on the operator's explicit approval, never without it, and then end your turn."
}

// raddleReturnNote returns the {{.return_note}} block: empty on a first session, and after a return the reason the filing returned the proposal with the file to start from.
// Auto mode sets the returned proposal aside at returnedPath; interactive mode keeps it at proposalPath with its approval cleared.
func raddleReturnNote(interactive bool, returnReason, proposalPath, returnedPath string) string {
	if returnReason == "" {
		return ""
	}
	if !interactive {
		return fmt.Sprintf("The filing returned your last proposal: %s\nIt is set aside at `%s`. Start from it, fix what the reason names, and write the corrected proposal at `%s`.", returnReason, returnedPath, proposalPath)
	}
	return fmt.Sprintf("The filing returned your last proposal and cleared its approval: %s\nThe proposal is still at `%s`. Start from it, fix what the reason names, run `lym loom raddle-propose`, and put the corrected proposal to the operator again.", returnReason, proposalPath)
}

// RaddleTable builds the seatengine.Table of one Raddle-Write run: one lead that writes the raddle proposal.
// The lead's model is cfg.Raddle resolved through reg, and the seat is interactive when interactive is set.
// Its one output is the approval record in interactive mode, written by `lym loom raddle-approve`, and the proposal file in auto mode, so the session ends on the operator's approval or on the finished proposal.
// returnReason is the reason a Raddle-File return gave, empty on a first session; it fills the prompt's return note with the set-aside proposal's path.
// The opening stencil is read at call time from stencilsDir, the prompt points at the proposal format spec under specsDir, and the seat's parent directive is rendered by the seat engine from its geometry.
// The table carries no gate; the producer entry sets it.
func RaddleTable(layout *lymcwd.Location, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry, slug string, interactive bool, returnReason string) (seatengine.Table, error) {
	if slug == "" {
		return seatengine.Table{}, fmt.Errorf("loom: RaddleTable: slug must not be empty")
	}
	choice, err := resolveModelChoice(cfg.Raddle, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: RaddleTable: raddle role model-spec: %w", err)
	}

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
			"mode_rules":           raddleModeRules(interactive, RaddleProposal(layout)),
			"return_note":          raddleReturnNote(interactive, returnReason, RaddleProposal(layout), RaddleReturnedProposal(layout)),
		},
		Optional: []string{"return_note"},
		Seats: []seatengine.Seat{{
			Stencil:     raddleStencil,
			ModelChoice: choice,
			Outputs:     []string{output},
			Skills:      raddleSkills,
			ModelKey:    shuttleengine.ModelKey{Module: loomModule, Key: "raddle"},
		}},
	}, nil
}
