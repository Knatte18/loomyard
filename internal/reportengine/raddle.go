// raddle.go declares the raddle report's result types and ComputeRaddle, which sets each filed Sketch's predicted cost against what its filed parts cost.

package reportengine

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/raddleparser"
)

// RaddleSelection records what a raddle report was computed over.
type RaddleSelection struct {
	BoardBranch   string    `json:"board_branch"`
	BoardTip      string    `json:"board_tip"`
	RecordsBranch string    `json:"records_branch"`
	RecordsTip    string    `json:"records_tip"`
	SnapshotAt    time.Time `json:"snapshot_at"`
}

// RaddleRow is one Sketch a raddle run filed.
// PredictedUSD is the approved proposal's expected cost and is nil when the proposal states none.
// ActualUSD is the summed session cost of the filed parts' runs and is nil when no session cost is known.
// ActualPartial says a run of a filed part holds a session of unknown cost, no session or an unreadable usage directory, so ActualUSD leaves it out.
// Unlanded lists the filed parts with no archive tag.
// ProposalChanged says the approved proposal's hash differs from the cold proposal's.
type RaddleRow struct {
	Sketch          string   `json:"sketch"`
	PredictedUSD    *float64 `json:"predicted_usd"`
	ActualUSD       *float64 `json:"actual_usd"`
	ActualPartial   bool     `json:"actual_partial"`
	Unlanded        []string `json:"unlanded"`
	ProposalChanged bool     `json:"proposal_changed"`
}

// RaddleReport is the result of ComputeRaddle.
type RaddleReport struct {
	Selection RaddleSelection `json:"selection"`
	Rows      []RaddleRow     `json:"rows"`
	Findings  []Finding       `json:"findings"`
}

// ComputeRaddle lists every Sketch of the raddle store at the board branch's tip and sets its predicted cost against the cost of its filed parts.
// A filed part's runs are the archive tags of its slug, each read through its usage files; the filed project, which never runs, is no part.
// A store directory without a store.json holds nothing.
// A Sketch whose store files are missing or do not decode is skipped and listed as a finding that names it.
// An error is a failed read of the repositories themselves.
func ComputeRaddle(src Sources) (RaddleReport, error) {
	report := RaddleReport{
		Selection: RaddleSelection{BoardBranch: src.BoardBranch, BoardTip: src.BoardTip, RecordsBranch: src.RecordsBranch, RecordsTip: src.RecordsTip, SnapshotAt: src.SnapshotAt},
		Rows:      []RaddleRow{},
		Findings:  []Finding{},
	}

	sketches, err := src.Board.SubdirsAtRevision(src.BoardTip, boardengine.RaddleStoreRootRel())
	if err != nil {
		return RaddleReport{}, fmt.Errorf("reportengine: list the raddle store at %s: %w", src.BoardTip, err)
	}
	tagsBySlug := map[string][]gitrepo.TagRef{}
	for _, tag := range src.Tags {
		if slug, named := fabricengine.ArchiveTagSlug(tag.Name); named {
			tagsBySlug[slug] = append(tagsBySlug[slug], tag)
		}
	}

	for _, sketch := range sketches {
		row, findings, kept, err := raddleRow(src, sketch, tagsBySlug)
		if err != nil {
			return RaddleReport{}, err
		}
		report.Findings = append(report.Findings, findings...)
		if kept {
			report.Rows = append(report.Rows, row)
		}
	}
	return report, nil
}

// raddleRow builds the row of the Sketch sketch from its store files at the board tip.
// kept is false for a store directory that holds nothing and for a Sketch whose files are missing or do not decode, the latter with a finding.
func raddleRow(src Sources, sketch string, tagsBySlug map[string][]gitrepo.TagRef) (row RaddleRow, findings []Finding, kept bool, err error) {
	recordRel := boardengine.RaddleStoreRecordRel(sketch)
	stored, found, err := boardFileAtTip(src, recordRel)
	if err != nil || !found {
		return RaddleRow{}, nil, false, err
	}
	entry, err := boardengine.ParseRaddleStoreEntry(stored)
	if err != nil {
		return RaddleRow{}, []Finding{raddleFinding(sketch, recordRel, err.Error())}, false, nil
	}

	approvedRel, coldRel := boardengine.RaddleProposalRel(sketch), boardengine.RaddleColdProposalRel(sketch)
	approved, found, err := boardFileAtTip(src, approvedRel)
	if err != nil {
		return RaddleRow{}, nil, false, err
	}
	if !found {
		return RaddleRow{}, []Finding{raddleFinding(sketch, approvedRel, "the store directory holds no approved proposal")}, false, nil
	}
	cold, found, err := boardFileAtTip(src, coldRel)
	if err != nil {
		return RaddleRow{}, nil, false, err
	}
	if !found {
		return RaddleRow{}, []Finding{raddleFinding(sketch, coldRel, "the store directory holds no cold proposal")}, false, nil
	}
	proposal, err := raddleparser.Parse(approved)
	if err != nil {
		return RaddleRow{}, []Finding{raddleFinding(sketch, approvedRel, err.Error())}, false, nil
	}

	row = RaddleRow{
		Sketch:          sketch,
		PredictedUSD:    proposal.Cost.ExpectedUSD,
		Unlanded:        []string{},
		ProposalChanged: raddleparser.Hash(approved) != raddleparser.Hash(cold),
	}
	total, priced := 0.0, 0
	for _, part := range entry.Filed {
		if part == entry.Project {
			continue
		}
		tags := tagsBySlug[part]
		if len(tags) == 0 {
			row.Unlanded = append(row.Unlanded, part)
			continue
		}
		for _, tag := range tags {
			usage, finding, err := readRunUsage(src.Records, tag)
			if err != nil {
				return RaddleRow{}, nil, false, err
			}
			if finding != nil {
				findings = append(findings, *finding)
				row.ActualPartial = true
				continue
			}
			if len(usage.Sessions) == 0 {
				row.ActualPartial = true
			}
			for _, session := range usage.Sessions {
				if session.CostUSD == nil {
					row.ActualPartial = true
					continue
				}
				total += *session.CostUSD
				priced++
			}
		}
	}
	if priced > 0 {
		row.ActualUSD = &total
	}
	return row, findings, true, nil
}

// boardFileAtTip reads rel from the board's tip; found is false for a path the tip's tree lacks.
func boardFileAtTip(src Sources, rel string) (data []byte, found bool, err error) {
	data, err = src.Board.FileAtRevision(src.BoardTip, rel)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reportengine: read %s at %s: %w", rel, src.BoardTip, err)
	}
	return data, true, nil
}

// raddleFinding is the finding of a store file of the Sketch sketch that is missing or does not decode.
func raddleFinding(sketch, file, message string) Finding {
	return Finding{File: file, Error: fmt.Sprintf("Sketch %q: %s", sketch, message)}
}
