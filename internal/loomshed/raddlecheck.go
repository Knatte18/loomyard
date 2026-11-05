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

// RaddleCheck carries the told values of the checks run over a raddle proposal.
// ProposalPath and RecordPath are absolute.
// Board builds the board context the checks read, with filing set at Raddle-File's run.
type RaddleCheck struct {
	// ProposalPath is the proposal file.
	ProposalPath string
	// RecordPath is the Sketch's decision record.
	RecordPath string
	// Board builds the context from the board, the raddle store and the profiles.
	Board func(filing bool) (raddleparser.BoardContext, error)
}

// readProposal reads and parses the proposal file.
// A file that is missing or does not parse comes back as the finding that names it, with a nil error, so a caller tells that from a fault of the read.
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

// CheckRaddleProposal judges the proposal on disk against the Sketch's decision record and the board, and returns one finding per broken rule.
// A proposal file that is missing or does not parse is the one finding `proposal-unparsable`, carrying the parser's way forward.
// A read error of the decision record or a failure to build the board context is returned, with no findings.
// filing is told to the board context, which Raddle-File's run sets.
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

// RecheckRaddleBoard judges the proposal on disk against the board as it is now, with the board context built for a filing run so the entries a part's dependency dropped are named.
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

// NewRaddleGate returns the raddle gate closure of the Raddle-Write row: a shuttleengine.Gate that runs CheckRaddleProposal(check, false) once and maps the result onto the gate contract.
// A returned error is an infrastructure fault and burns no attempt.
// Findings fail the gate, with the findings joined as the result's Findings and logged, since the findings file is ephemeral.
// A pass calls onPass before it passes, and an error of onPass is an infrastructure error too.
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
