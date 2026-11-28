// recordhistory_test.go tests the cut of a decision record's history at its newest replace.
// The dropped-entry check reads the history so cut.

package loomcli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	historyRel = "_lym/discussion/decision-record.md"
	// historyWithE holds the added entry E, and historyX is the replaced record without it.
	// historyY is historyX with a later entry Y.
	historyWithE = "## Decisions\n\n### D1 — One\n\nOne.\n\n### Added after Discussion (parent, 2026-03-01): E\n\nDecision: e.\n"
	historyX     = "## Decisions\n\n### D1 — One\n\nOne, folding E.\n"
	historyY     = historyX + "\n### Added after Discussion (parent, 2026-03-05): Y\n\nDecision: y.\n"
)

// historyVersion is a record version at a first-parent position.
func historyVersion(position int, data string) fabricengine.AnchoredFileVersion {
	return fabricengine.AnchoredFileVersion{Position: position, Data: []byte(data)}
}

// replaceTrailerAt is a replace trailer commit at position naming the record at rel with the sha256 of written.
func replaceTrailerAt(position int, rel, written string) fabricengine.TrailerCommit {
	return fabricengine.TrailerCommit{Position: position, Value: recordReplacedValue(decisionByParent, rel, []byte(written))}
}

// TestRecordHistoryCut asserts which versions the dropped-entry check reads after a replace.
// It so asserts which dropped entries the check still reports.
func TestRecordHistoryCut(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// record is the record checked, and versions its first-parent history newest first.
		// trailers are the replace trailers, newest first.
		record   string
		versions []fabricengine.AnchoredFileVersion
		trailers []fabricengine.TrailerCommit
		// wantCut is whether the history is cut; wantDropped lists the titles the check reports dropped from the read history.
		wantCut     bool
		wantDropped []string
	}{
		{
			name:     "AReplaceThatDropsAnEntry_ReportsNoDrop",
			record:   historyX,
			versions: []fabricengine.AnchoredFileVersion{historyVersion(4, historyX), historyVersion(2, historyWithE)},
			trailers: []fabricengine.TrailerCommit{replaceTrailerAt(4, historyRel, historyX)},
			wantCut:  true,
		},
		{
			name:     "ReplacedBytesLandedBeforeTheTrailerCommit_ReportsNoDrop",
			record:   historyX,
			versions: []fabricengine.AnchoredFileVersion{historyVersion(3, historyX), historyVersion(1, historyWithE)},
			trailers: []fabricengine.TrailerCommit{replaceTrailerAt(5, historyRel, historyX)},
			wantCut:  true,
		},
		{
			name:   "AnEntryAddedAfterTheReplaceThenDropped_IsReported",
			record: historyX,
			versions: []fabricengine.AnchoredFileVersion{
				historyVersion(8, historyX), historyVersion(6, historyY), historyVersion(4, historyX), historyVersion(2, historyWithE),
			},
			trailers:    []fabricengine.TrailerCommit{replaceTrailerAt(4, historyRel, historyX)},
			wantCut:     true,
			wantDropped: []string{"Y"},
		},
		{
			name:   "AnEmptyTrailerCommitOverASweptReplace_StillReportsALaterDrop",
			record: historyX,
			versions: []fabricengine.AnchoredFileVersion{
				historyVersion(8, historyX), historyVersion(6, historyY), historyVersion(3, historyX), historyVersion(1, historyWithE),
			},
			trailers:    []fabricengine.TrailerCommit{replaceTrailerAt(4, historyRel, historyX)},
			wantCut:     true,
			wantDropped: []string{"Y"},
		},
		{
			name:     "ATrailerNamingAnotherRecord_CutsNothing",
			record:   historyX,
			versions: []fabricengine.AnchoredFileVersion{historyVersion(4, historyX), historyVersion(2, historyWithE)},
			trailers: []fabricengine.TrailerCommit{replaceTrailerAt(4, "_lym/projects/the-beam/decision-record.md", historyX)},
		},
		{
			name:     "TheNewestTrailerOfThisRecordCuts_PastAnotherRecordsNewerOne",
			record:   historyX,
			versions: []fabricengine.AnchoredFileVersion{historyVersion(4, historyX), historyVersion(2, historyWithE)},
			trailers: []fabricengine.TrailerCommit{
				replaceTrailerAt(6, "_lym/projects/the-beam/decision-record.md", historyWithE), replaceTrailerAt(4, historyRel, historyX),
			},
			wantCut: true,
		},
		{
			name:     "ATrailerWhoseSumMatchesNoEarlierVersion_CutsNothing",
			record:   historyX,
			versions: []fabricengine.AnchoredFileVersion{historyVersion(6, historyX), historyVersion(2, historyWithE)},
			trailers: []fabricengine.TrailerCommit{replaceTrailerAt(4, historyRel, historyX)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			whole := make([][]byte, 0, len(tt.versions))
			for _, version := range tt.versions {
				whole = append(whole, version.Data)
			}
			readWhole := false
			history, err := recordHistoryFrom(historyRel, tt.trailers,
				func() ([]fabricengine.AnchoredFileVersion, error) { return tt.versions, nil },
				func() ([][]byte, error) { readWhole = true; return whole, nil })
			if err != nil {
				t.Fatalf("recordHistoryFrom() error = %v; want nil", err)
			}
			if cut := !readWhole; cut != tt.wantCut {
				t.Fatalf("cut = %t; want %t", cut, tt.wantCut)
			}
			if !tt.wantCut && !slices.EqualFunc(history, whole, bytes.Equal) {
				t.Fatalf("history = %q; want the whole history %q", history, whole)
			}
			var dropped []string
			for _, finding := range discussionparser.CheckAddedDecisionsKept([]byte(tt.record), history) {
				dropped = append(dropped, finding.Detail)
			}
			if !tt.wantCut {
				if len(dropped) != 1 {
					t.Errorf("findings = %q; want the uncut history to report the dropped entry E", dropped)
				}
				return
			}
			if len(dropped) != len(tt.wantDropped) {
				t.Fatalf("findings = %q; want drops of %q", dropped, tt.wantDropped)
			}
			for i, title := range tt.wantDropped {
				if want := "): " + title + "`"; !strings.Contains(dropped[i], want) {
					t.Errorf("finding %q; want it to name the entry %q", dropped[i], title)
				}
			}
		})
	}
}
