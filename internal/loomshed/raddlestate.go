// raddlestate.go holds the raddle run's state record and the one-time storing of the cold proposal.

package loomshed

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/state"
)

// RaddleMode is how a raddle run gets its approval.
// An auto run files without an operator; an interactive run waits for the operator's approval.
type RaddleMode string

// The two modes of a raddle run.
const (
	RaddleModeAuto        RaddleMode = "auto"
	RaddleModeInteractive RaddleMode = "interactive"
)

// RaddleApprovalRecord is the approval of one proposal:
// the hash of the proposal file approved and the Raddle-Write session it was approved in.
// It is the content of the run's approval file and of the approval the state records.
type RaddleApprovalRecord struct {
	Hash    string `json:"hash"`
	Session int    `json:"session"`
}

// RaddleState is the raddle run's state record, `state.json` in the run's raddle directory.
// It is read and written through internal/state, so a key it does not declare is accepted and survives a rewrite.
// Mode is empty until Raddle-Write's first spawn records it.
// Session numbers the current Raddle-Write session, raised at every call of the row.
// ProposedSession is the session of the last passing propose.
// NoticeSession is the session whose waiting notice went out.
// Approval is nil while no proposal is approved.
// Returns counts Raddle-File's returns.
// ReturnReason holds the reason of a return no new session has taken up yet.
// EntryPosition is the history position of the row entry the last fresh-entry reset took up.
// NoticeSent marks Raddle-File's notice of what it filed.
type RaddleState struct {
	Mode            RaddleMode            `json:"mode"`
	ColdStored      bool                  `json:"cold_stored"`
	Session         int                   `json:"session"`
	ProposedSession int                   `json:"proposed_session"`
	NoticeSession   int                   `json:"notice_session"`
	Approval        *RaddleApprovalRecord `json:"approval,omitempty"`
	Returns         int                   `json:"returns"`
	ReturnReason    string                `json:"return_reason,omitempty"`
	EntryPosition   int                   `json:"entry_position"`
	NoticeSent      bool                  `json:"notice_sent"`
}

// ReadRaddleState reads the run's state at path under the lock at lockPath.
// An absent file reads as the zero state.
func ReadRaddleState(path, lockPath string) (RaddleState, error) {
	current, _, err := state.ReadJSON[RaddleState](path, lockPath)
	return current, err
}

// UpdateRaddleState applies mutate to the run's state at path under one exclusive lock, writes the result and returns it.
// It starts from the zero state when the file is absent.
func UpdateRaddleState(path, lockPath string, mutate func(*RaddleState)) (RaddleState, error) {
	var updated RaddleState
	err := state.UpdateJSON(path, lockPath, func(current RaddleState, _ bool) (RaddleState, error) {
		mutate(&current)
		updated = current
		return current, nil
	})
	return updated, err
}

// StoreColdProposal copies the proposal at proposalPath to coldPath and sets cold_stored in the state, in one state update.
// It does nothing once cold_stored is set, so the cold proposal is stored once per run and never overwritten.
func StoreColdProposal(proposalPath, coldPath, statePath, lockPath string) error {
	return state.UpdateJSON(statePath, lockPath, func(current RaddleState, _ bool) (RaddleState, error) {
		if current.ColdStored {
			return current, nil
		}
		proposal, err := os.ReadFile(proposalPath)
		if err != nil {
			return current, fmt.Errorf("loomshed: read the proposal to store as the cold proposal: %w", err)
		}
		if err := fsx.AtomicWriteBytes(coldPath, proposal); err != nil {
			return current, fmt.Errorf("loomshed: store the cold proposal: %w", err)
		}
		current.ColdStored = true
		return current, nil
	})
}
