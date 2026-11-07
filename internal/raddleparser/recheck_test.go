// recheck_test.go covers RecheckBoard: the board unchanged, an entry revised, an open entry added, a slug taken, and a weighed entry landed.

package raddleparser

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRecheckBoard starts from the fixture board, under which the fixture proposal passes.
// It moves that board one way per table row.
func TestRecheckBoard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(p *Proposal, b *BoardContext)
		wantFail    []string
		wantLanded  []string
		wantDropped []string
	}{
		{"BoardUnchanged", func(*Proposal, *BoardContext) {}, nil, nil, nil},
		{"RevisedStampMoved", func(_ *Proposal, b *BoardContext) { b.Entries[1].Revised = "2026-11-02T10:00:00Z" }, []string{CheckRevisedMoved}, nil, nil},
		{"SameInstantInAnotherZoneIsUnmoved", func(_ *Proposal, b *BoardContext) { b.Entries[1].Revised = "2026-11-01T11:00:00+01:00" }, nil, nil, nil},
		{"OpenEntryCreatedAfterTheRead", func(_ *Proposal, b *BoardContext) {
			b.Entries = append(b.Entries, BoardEntry{Slug: "new-task", Kind: KindTask, Labels: []string{"mod-a"}, Open: true, Revised: "2026-11-03T10:00:00Z"})
		}, []string{CheckBoardUnchecked}, nil, nil},
		{"NotePromotedWithItsOldDates", func(_ *Proposal, b *BoardContext) {
			b.Entries = append(b.Entries, BoardEntry{Slug: "promoted", Kind: KindTask, Labels: []string{"mod-a"}, Open: true, Revised: "2025-01-01T00:00:00Z"})
		}, []string{CheckBoardUnchecked}, nil, nil},
		{"SlugTakenOnTheBoard", func(p *Proposal, _ *BoardContext) { p.Parts[1].Slug = "other-open" }, []string{CheckSlugTaken}, nil, nil},
		{"SlugTakenInTheStore", func(p *Proposal, b *BoardContext) {
			p.Parts[1].Slug = "stored"
			b.Store = []string{"stored"}
		}, []string{CheckSlugTaken}, nil, nil},
		{"WeighedEntryNowDone", func(_ *Proposal, b *BoardContext) { b.Entries[1].Open = false }, nil, []string{"other-open"}, nil},
		{"WeighedEntryGoneFromTheBoard", func(_ *Proposal, b *BoardContext) { b.Entries = b.Entries[:1] }, nil, []string{"other-open"}, nil},
		{"DependencyOnAGoneEntryNotDroppedOutsideFiling", func(p *Proposal, b *BoardContext) {
			p.Parts[1].DependsOn = append(p.Parts[1].DependsOn, "other-open")
			b.Entries = b.Entries[:1]
		}, nil, []string{"other-open"}, nil},
		{"DependencyOnAGoneEntryDroppedWhileFiling", func(p *Proposal, b *BoardContext) {
			p.Parts[1].DependsOn = append(p.Parts[1].DependsOn, "other-open")
			b.Entries = b.Entries[:1]
			b.Filing = true
		}, nil, []string{"other-open"}, []string{"other-open"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			proposal, board := validProposal(), validBoard()
			tt.mutate(&proposal, &board)

			got := RecheckBoard(proposal, board)

			if failed := checkNames(got.Failures); !slices.Equal(failed, tt.wantFail) {
				t.Errorf("RecheckBoard() failures = %v; want %v", failed, tt.wantFail)
			}
			if !slices.Equal(got.Landed, tt.wantLanded) {
				t.Errorf("RecheckBoard() landed = %v; want %v", got.Landed, tt.wantLanded)
			}
			if !slices.Equal(got.Dropped, tt.wantDropped) {
				t.Errorf("RecheckBoard() dropped = %v; want %v", got.Dropped, tt.wantDropped)
			}
		})
	}
}
