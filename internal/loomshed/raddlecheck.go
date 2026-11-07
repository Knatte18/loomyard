// raddlecheck.go holds the checks a raddle proposal on disk is judged by and the raddle gate that runs them.

package loomshed

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// checkProposalUnparsable names the finding of a proposal file that is missing or does not parse.
const checkProposalUnparsable = "proposal-unparsable"

// RaddleCheck carries the inputs of the checks run over a raddle proposal.
// ProposalPath and RecordPath are absolute.
type RaddleCheck struct {
	// ProposalPath is the proposal file.
	ProposalPath string
	// RecordPath is the approved decision record a Discussion writes for the discussed entry.
	RecordPath string
	// Board builds the context from the board, the raddle store and the profiles, with filing set by the caller.
	Board func(filing bool) (raddleparser.BoardContext, error)
}

// readProposal reads and parses the proposal file.
// A file that is missing or does not parse comes back as the finding that names it, with a nil error.
// A caller tells that finding from a read fault, which comes back as the error.
func readProposal(path string) (proposal raddleparser.Proposal, unparsable *raddleparser.Finding, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return proposal, &raddleparser.Finding{Check: checkProposalUnparsable, Detail: fmt.Sprintf("no proposal is written at %s; way forward: write it there in the form of the raddle proposal spec", path)}, nil
	}
	if err != nil {
		return proposal, nil, err
	}
	proposal, err = raddleparser.Parse(data)
	if err != nil {
		return proposal, &raddleparser.Finding{Check: checkProposalUnparsable, Detail: err.Error()}, nil
	}
	return proposal, nil, nil
}

// CheckRaddleProposal judges the proposal on disk against the decision record and the board.
// It returns one finding per broken rule.
// A proposal file that is missing or does not parse yields the one finding `proposal-unparsable`.
// That finding carries the parser's way forward.
// A read error of the decision record or a failure to build the board context is returned, with no findings.
// It passes filing to the board context; Raddle-File's run sets it.
func CheckRaddleProposal(check RaddleCheck, filing bool) ([]raddleparser.Finding, error) {
	proposal, unparsable, err := readProposal(check.ProposalPath)
	if err != nil {
		return nil, err
	}
	if unparsable != nil {
		return []raddleparser.Finding{*unparsable}, nil
	}

	record, err := os.ReadFile(check.RecordPath)
	if err != nil {
		return nil, err
	}
	board, err := check.Board(filing)
	if err != nil {
		return nil, err
	}
	return raddleparser.Check(proposal, record, board), nil
}

// RecheckRaddleBoard judges the proposal on disk against the board as it is now.
// It builds the board context for a filing run, so the entries a part's dependency dropped are named.
// A proposal that is missing or does not parse is a returned error, since the re-check follows a check that passed.
func RecheckRaddleBoard(check RaddleCheck) (raddleparser.Recheck, error) {
	proposal, unparsable, err := readProposal(check.ProposalPath)
	if err != nil {
		return raddleparser.Recheck{}, err
	}
	if unparsable != nil {
		return raddleparser.Recheck{}, fmt.Errorf("loomshed: re-check the proposal: %s", unparsable.Detail)
	}
	board, err := check.Board(true)
	if err != nil {
		return raddleparser.Recheck{}, err
	}
	return raddleparser.RecheckBoard(proposal, board), nil
}

// FormatRaddleFindings renders findings as one semicolon-separated list, each as `check[/part]: detail`.
func FormatRaddleFindings(findings []raddleparser.Finding) string {
	parts := make([]string, len(findings))
	for i, finding := range findings {
		head := finding.Check
		if finding.Part != "" {
			head += "/" + finding.Part
		}
		parts[i] = head + ": " + finding.Detail
	}
	return strings.Join(parts, "; ")
}

// NewRaddleGate returns the shuttleengine.Gate of the Raddle-Write row.
// The gate checks the proposal once, without filing, and maps the result onto the gate contract.
// A returned error is an infrastructure fault and burns no attempt.
// Findings fail the gate.
// The gate joins them into the result's Findings and logs them, since the findings file is ephemeral.
// A passing check calls onPass before reporting the pass, and an error from onPass is an infrastructure error too.
func NewRaddleGate(check RaddleCheck, onPass func() error) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		findings, err := CheckRaddleProposal(check, false)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(findings) > 0 {
			formatted := FormatRaddleFindings(findings)
			logger.Warn("loomshed: raddle gate failed validation", "gate", "Raddle-Gate", "proposal", check.ProposalPath, "findings", formatted)
			return shuttleengine.GateResult{Passed: false, Findings: formatted}, nil
		}
		if err := onPass(); err != nil {
			return shuttleengine.GateResult{}, err
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}
}
