// judgement.go — the sole parser of `_lym/drift/judgement.yaml`, the rulings the judge writes, and the model it parses to.

package driftengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Ruling is the judge's ruling on one cell or one case.
type Ruling string

// The rulings the judge gives.
const (
	// RulingMatches says the change fits the entry that expects it.
	RulingMatches Ruling = "matches"
	// RulingNoMatch says the change does not fit the entry that expects it.
	RulingNoMatch Ruling = "does not match"
	// RulingUnexpected says a failing compare is not a change the expectation explains, under DiffCommand only.
	RulingUnexpected Ruling = "unexpected"
)

// Judgement is the rulings one judge spawn wrote.
// Under DiffLines it holds Cells and OpenRanges, and under DiffCommand Cases.
type Judgement struct {
	// SpawnID names the spawn the judge was handed these cells or cases in.
	SpawnID string `yaml:"spawn_id"`
	// Cells are the rulings on the judged cells, one per cell id.
	Cells []CellRuling `yaml:"cells"`
	// OpenRanges notes what the judge checked for each open range covering a judged cell.
	OpenRanges []OpenRangeNote `yaml:"open_ranges"`
	// Cases are the rulings on the failing compare, one per case.
	Cases []CaseRuling `yaml:"cases"`
}

// CellRuling is the judge's ruling on one cell.
type CellRuling struct {
	// ID is the cell id the spawn handed the judge.
	ID string `yaml:"id"`
	// Ruling is RulingMatches or RulingNoMatch.
	Ruling Ruling `yaml:"ruling"`
	// Reason is why the judge ruled so.
	Reason string `yaml:"reason"`
}

// CaseRuling is the judge's ruling on one case of a failing compare.
type CaseRuling struct {
	// Case is the case's name.
	Case string `yaml:"case"`
	// Ruling is RulingMatches, RulingNoMatch or RulingUnexpected.
	Ruling Ruling `yaml:"ruling"`
	// Reason is why the judge ruled so.
	Reason string `yaml:"reason"`
}

// OpenRangeNote records that the judge checked what an open range covers.
type OpenRangeNote struct {
	// Case is the case holding the range.
	Case string `yaml:"case"`
	// Range is the range as the expectation spells it.
	Range string `yaml:"range"`
	// Checked is what the judge looked at to see the open range covers only intended changes.
	Checked string `yaml:"checked"`
}

// JudgementWant is what a spawn handed the judge, which the judgement must answer in full.
type JudgementWant struct {
	// Mode is DiffLines or DiffCommand.
	Mode string
	// SpawnID is the spawn's id.
	SpawnID string
	// CellIDs are the ids of the cells the judge must rule on, under DiffLines.
	CellIDs []string
	// OpenRanges are the open ranges covering a judged cell, as case and range, under DiffLines; Checked is ignored.
	OpenRanges []OpenRangeNote
	// CompareFailed reports that the compare failed, under DiffCommand, so the judgement must rule on at least one case.
	CompareFailed bool
}

// ParseJudgement parses judgement.yaml's bytes against what the spawn handed the judge.
// The file is agent-written, so an unknown field is refused.
//
// It refuses a spawn_id other than want's.
// Under DiffLines it refuses a missing, duplicate or unknown cell id, a ruling outside matches and does not match, an empty reason, an open range covering a judged cell that open_ranges does not list, and a cases list.
// Under DiffCommand it refuses a ruling outside matches, does not match and unexpected, an empty reason, a repeated case, no case at all when the compare failed, and a cells or open_ranges list.
// The error lists every finding, one per line, each naming its entry, since the judge's gate hands it back as the findings to fix.
func ParseJudgement(data []byte, want JudgementWant) (Judgement, error) {
	if want.Mode != DiffLines && want.Mode != DiffCommand {
		return Judgement{}, fmt.Errorf("judgement.yaml: mode %q is neither %q nor %q", want.Mode, DiffLines, DiffCommand)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var judgement Judgement
	if err := decoder.Decode(&judgement); err != nil && !errors.Is(err, io.EOF) {
		return Judgement{}, fmt.Errorf("judgement.yaml: %w", err)
	}

	var findings []string
	if judgement.SpawnID != want.SpawnID {
		findings = append(findings, fmt.Sprintf("spawn_id %q is not this spawn's %q", judgement.SpawnID, want.SpawnID))
	}
	if want.Mode == DiffLines {
		findings = append(findings, linesFindings(judgement, want)...)
	} else {
		findings = append(findings, commandFindings(judgement, want)...)
	}
	if len(findings) > 0 {
		return Judgement{}, errors.New("judgement.yaml: " + strings.Join(findings, "\njudgement.yaml: "))
	}
	return judgement, nil
}

// linesFindings returns what is wrong with a judgement under DiffLines.
func linesFindings(judgement Judgement, want JudgementWant) []string {
	var findings []string
	if len(judgement.Cases) > 0 {
		findings = append(findings, fmt.Sprintf("a cases list belongs to diff %q; under diff %q rule on cells", DiffCommand, DiffLines))
	}
	wanted := make(map[string]bool, len(want.CellIDs))
	for _, id := range want.CellIDs {
		wanted[id] = true
	}
	ruled := make(map[string]bool, len(judgement.Cells))
	for _, cell := range judgement.Cells {
		switch {
		case !wanted[cell.ID]:
			findings = append(findings, fmt.Sprintf("cell %q is not a cell of this spawn", cell.ID))
			continue
		case ruled[cell.ID]:
			findings = append(findings, fmt.Sprintf("cell %q is ruled on more than once", cell.ID))
			continue
		}
		ruled[cell.ID] = true
		if cell.Ruling != RulingMatches && cell.Ruling != RulingNoMatch {
			findings = append(findings, fmt.Sprintf("cell %q has ruling %q, which is neither %q nor %q", cell.ID, cell.Ruling, RulingMatches, RulingNoMatch))
		}
		if strings.TrimSpace(cell.Reason) == "" {
			findings = append(findings, fmt.Sprintf("cell %q has an empty reason", cell.ID))
		}
	}
	for _, id := range want.CellIDs {
		if !ruled[id] {
			findings = append(findings, fmt.Sprintf("cell %q has no ruling", id))
		}
	}

	listed := make(map[[2]string]bool, len(judgement.OpenRanges))
	for _, note := range judgement.OpenRanges {
		listed[[2]string{note.Case, note.Range}] = true
	}
	for _, open := range want.OpenRanges {
		if !listed[[2]string{open.Case, open.Range}] {
			findings = append(findings, fmt.Sprintf("open range %s of case %q covers a judged cell but open_ranges does not list it", open.Range, open.Case))
		}
	}
	return findings
}

// commandFindings returns what is wrong with a judgement under DiffCommand.
func commandFindings(judgement Judgement, want JudgementWant) []string {
	var findings []string
	if len(judgement.Cells) > 0 || len(judgement.OpenRanges) > 0 {
		findings = append(findings, fmt.Sprintf("a cells or open_ranges list belongs to diff %q; under diff %q rule on cases", DiffLines, DiffCommand))
	}
	if want.CompareFailed && len(judgement.Cases) == 0 {
		findings = append(findings, "the compare failed, so cases must rule on at least one case")
	}
	seen := make(map[string]bool, len(judgement.Cases))
	for _, ruling := range judgement.Cases {
		if seen[ruling.Case] {
			findings = append(findings, fmt.Sprintf("case %q is ruled on more than once", ruling.Case))
			continue
		}
		seen[ruling.Case] = true
		if ruling.Ruling != RulingMatches && ruling.Ruling != RulingNoMatch && ruling.Ruling != RulingUnexpected {
			findings = append(findings, fmt.Sprintf("case %q has ruling %q, which is none of %q, %q and %q", ruling.Case, ruling.Ruling, RulingMatches, RulingNoMatch, RulingUnexpected))
		}
		if strings.TrimSpace(ruling.Reason) == "" {
			findings = append(findings, fmt.Sprintf("case %q has an empty reason", ruling.Case))
		}
	}
	return findings
}
