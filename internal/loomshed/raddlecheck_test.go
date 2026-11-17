// raddlecheck_test.go exercises the proposal check, the board re-check and the raddle gate over a proposal and decision record in a temp directory.

package loomshed_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// raddleRecord is a decision record of two decisions with two risks, two criteria and two PATTERN changes; § stands for a backtick.
var raddleRecord = strings.NewReplacer("§", "`").Replace(`## Decisions

### D1 — First

The first decision.

### D2 — Second

The second decision.

## Open risks

- Risk one
- Risk two

## Acceptance criteria

- D1: first holds.
- D2: second holds.

## PATTERN changes

- §PATTERN-a§ — adds: Rule a.
- §PATTERN-b§ — adds: Rule b.
`)

// validRaddleProposal returns a proposal of two parts that passes every check against raddleRecord and validRaddleBoard.
func validRaddleProposal() raddleparser.Proposal {
	return raddleparser.Proposal{
		Sketch:       "sketch-x",
		Shape:        raddleparser.ShapeTasks,
		BoardChecked: []raddleparser.BoardCheck{{Slug: "other-open", Revised: "2026-11-01T10:00:00Z"}},
		Parts: []raddleparser.Part{
			{
				Slug: "part-a", Title: "Part A", Type: "feature", Recipe: raddleparser.RecipeTapestry, Profile: "standard",
				Decisions: []string{"D1"}, Packages: []raddleparser.Package{{Path: "internal/a", Size: 10}},
				OpenRisks: []string{"Risk one"}, Criteria: []string{"D1: first holds."}, PatternChanges: []string{"PATTERN-a"},
				CostLine: "one tapestry part",
			},
			{
				Slug: "part-b", Title: "Part B", Type: "feature", Recipe: raddleparser.RecipeTapestry, Profile: "standard",
				Decisions: []string{"D2"}, Packages: []raddleparser.Package{{Path: "internal/b", Size: 10}}, DependsOn: []string{"part-a"},
				OpenRisks: []string{"Risk two"}, Criteria: []string{"D2: second holds."}, PatternChanges: []string{"PATTERN-b"},
				CostLine: "one tapestry part",
			},
		},
		Cost: raddleparser.Cost{Line: "two tapestry parts", NextBest: raddleparser.NextBest{Split: "one part", Line: "one tapestry part"}},
	}
}

// validRaddleBoard returns a board context under which validRaddleProposal passes.
func validRaddleBoard() raddleparser.BoardContext {
	return raddleparser.BoardContext{
		Sketch:       "sketch-x",
		Entry:        "sketch-x",
		ModuleLabels: []string{"mod-a"},
		Entries: []raddleparser.BoardEntry{
			{Slug: "sketch-x", Kind: raddleparser.KindTask, Labels: []string{"mod-a", "sketch"}, Open: true, Revised: "2026-11-01T09:00:00Z"},
			{Slug: "other-open", Kind: raddleparser.KindTask, Labels: []string{"mod-z"}, Open: true, Revised: "2026-11-01T10:00:00Z"},
		},
		TypeLabels: []string{"feature", "fix", "sketch"},
		Profiles:   map[string][]string{raddleparser.RecipeTapestry: {"standard"}, raddleparser.RecipeDarn: {"quick"}},
	}
}

// raddleCheckFixture writes proposal as JSON and the record into a temp directory and returns the check over them, whose Board answers board and boardErr and records the filing flag it was asked for.
func raddleCheckFixture(t *testing.T, proposal []byte, board raddleparser.BoardContext, boardErr error, askedFiling *[]bool) loomshed.RaddleCheck {
	t.Helper()
	dir := t.TempDir()
	check := loomshed.RaddleCheck{
		ProposalPath: filepath.Join(dir, "proposal.yaml"),
		RecordPath:   filepath.Join(dir, "decision.md"),
		Board: func(filing bool) (raddleparser.BoardContext, error) {
			if askedFiling != nil {
				*askedFiling = append(*askedFiling, filing)
			}
			return board, boardErr
		},
	}
	if proposal != nil {
		if err := os.WriteFile(check.ProposalPath, proposal, 0o644); err != nil {
			t.Fatalf("write the proposal: %v", err)
		}
	}
	if err := os.WriteFile(check.RecordPath, []byte(raddleRecord), 0o644); err != nil {
		t.Fatalf("write the record: %v", err)
	}
	return check
}

// marshalRaddleProposal returns the proposal as the JSON object the file holds.
func marshalRaddleProposal(t *testing.T, proposal raddleparser.Proposal) []byte {
	t.Helper()
	data, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("marshal the proposal: %v", err)
	}
	return data
}

func TestCheckRaddleProposal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boardFault := errors.New("board unavailable")
	dependsUnknown := validRaddleProposal()
	dependsUnknown.Parts[1].DependsOn = []string{"ghost"}

	tests := []struct {
		name         string
		proposal     []byte
		removeRecord bool
		boardErr     error
		filing       bool
		wantChecks   []string
		wantErr      error
	}{
		{name: "ValidProposalHasNoFinding", proposal: marshalRaddleProposal(t, validRaddleProposal())},
		{name: "FindingPassesThrough", proposal: marshalRaddleProposal(t, dependsUnknown), wantChecks: []string{raddleparser.CheckDependsUnknown}},
		{name: "UnparsableFileIsOneFinding", proposal: []byte("not json"), wantChecks: []string{"proposal-unparsable"}},
		{name: "MissingFileIsOneFinding", proposal: nil, wantChecks: []string{"proposal-unparsable"}},
		{name: "RecordReadErrorIsReturned", proposal: marshalRaddleProposal(t, validRaddleProposal()), removeRecord: true, wantErr: os.ErrNotExist},
		{name: "BoardErrorIsReturned", proposal: marshalRaddleProposal(t, validRaddleProposal()), boardErr: boardFault, wantErr: boardFault},
		{name: "FilingFlagReachesTheBoard", proposal: marshalRaddleProposal(t, validRaddleProposal()), filing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var asked []bool
			check := raddleCheckFixture(t, tt.proposal, validRaddleBoard(), tt.boardErr, &asked)
			if tt.removeRecord {
				if err := os.Remove(check.RecordPath); err != nil {
					t.Fatalf("remove the record: %v", err)
				}
			}

			findings, err := loomshed.CheckRaddleProposal(check, tt.filing)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CheckRaddleProposal() error = %v; want %v", err, tt.wantErr)
			}
			var got []string
			for _, finding := range findings {
				got = append(got, finding.Check)
			}
			if !slices.Equal(got, tt.wantChecks) {
				t.Errorf("CheckRaddleProposal() checks = %v; want %v", got, tt.wantChecks)
			}
			if tt.filing && !slices.Equal(asked, []bool{true}) {
				t.Errorf("Board was asked for filing %v; want [true]", asked)
			}
		})
	}
}

func TestRecheckRaddleBoard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("NamesAMovedEntryAndLandedOnes", func(t *testing.T) {
		t.Parallel()
		proposal := validRaddleProposal()
		proposal.BoardChecked = append(proposal.BoardChecked, raddleparser.BoardCheck{Slug: "gone", Revised: "2026-11-01T08:00:00Z"})
		board := validRaddleBoard()
		board.Entries[1].Revised = "2026-11-02T10:00:00Z"
		var asked []bool
		check := raddleCheckFixture(t, marshalRaddleProposal(t, proposal), board, nil, &asked)

		recheck, err := loomshed.RecheckRaddleBoard(check)
		if err != nil {
			t.Fatalf("RecheckRaddleBoard() error = %v; want nil", err)
		}
		if len(recheck.Failures) != 1 || recheck.Failures[0].Check != raddleparser.CheckRevisedMoved {
			t.Errorf("RecheckRaddleBoard() failures = %+v; want one %s", recheck.Failures, raddleparser.CheckRevisedMoved)
		}
		if !reflect.DeepEqual(recheck.Landed, []string{"gone"}) {
			t.Errorf("RecheckRaddleBoard() landed = %v; want [gone]", recheck.Landed)
		}
		if !slices.Equal(asked, []bool{true}) {
			t.Errorf("Board was asked for filing %v; want [true]", asked)
		}
	})

	t.Run("UnparsableProposalIsAnError", func(t *testing.T) {
		t.Parallel()
		check := raddleCheckFixture(t, []byte("not json"), validRaddleBoard(), nil, nil)
		if _, err := loomshed.RecheckRaddleBoard(check); err == nil {
			t.Error("RecheckRaddleBoard() = nil error; want one for an unparsable proposal")
		}
	})
}

func TestNewRaddleGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	onPassFault := errors.New("store failed")
	tests := []struct {
		name         string
		proposal     []byte
		onPassErr    error
		wantPassed   bool
		wantFindings string
		wantOnPass   int
		wantErr      error
	}{
		{name: "PassCallsOnPassOnce", proposal: marshalRaddleProposal(t, validRaddleProposal()), wantPassed: true, wantOnPass: 1},
		{name: "FailReturnsFindingsAndSkipsOnPass", proposal: []byte("not json"), wantFindings: "proposal-unparsable"},
		{name: "OnPassErrorIsReturned", proposal: marshalRaddleProposal(t, validRaddleProposal()), onPassErr: onPassFault, wantOnPass: 1, wantErr: onPassFault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			check := raddleCheckFixture(t, tt.proposal, validRaddleBoard(), nil, nil)
			onPassCalls := 0
			gate := loomshed.NewRaddleGate(check, func() error {
				onPassCalls++
				return tt.onPassErr
			})

			result, err := gate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("gate() error = %v; want %v", err, tt.wantErr)
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("gate() passed = %v; want %v", result.Passed, tt.wantPassed)
			}
			if tt.wantFindings != "" && !strings.Contains(result.Findings, tt.wantFindings) {
				t.Errorf("gate() findings = %q; want it to contain %q", result.Findings, tt.wantFindings)
			}
			if onPassCalls != tt.wantOnPass {
				t.Errorf("onPass calls = %d; want %d", onPassCalls, tt.wantOnPass)
			}
		})
	}
}
