// delivery_test.go covers RecordDelivery, the decision verb's record of a notice sent to a Weaver strand.

package weaverengine_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestRecordDelivery(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const attemptStrand, fixerStrand = "hub:task:weaver", "hub:task:verify-fixer"
	tests := []struct {
		name        string
		strand      string
		headings    []string
		wantAttempt []string
		wantFixer   []string
	}{
		{name: "MatchingAttemptRecordsTheNewHeadingsOnce", strand: attemptStrand, headings: []string{"## a", "## old"}, wantAttempt: []string{"## old", "## a"}},
		{name: "MatchingFixerRecordsTheHeadings", strand: fixerStrand, headings: []string{"## a"}, wantAttempt: []string{"## old"}, wantFixer: []string{"## a"}},
		{name: "UnknownStrandChangesNothing", strand: "hub:task:other", headings: []string{"## a"}, wantAttempt: []string{"## old"}},
		{name: "EmptyStrandNameChangesNothing", strand: "", headings: []string{"## a"}, wantAttempt: []string{"## old"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			runDir, scratchDir := weaverengine.Dir(anchor), weaverengine.ScratchDir(anchor)
			planted := &weaverengine.State{
				Attempts: []*weaverengine.AttemptState{{Batch: 1, Number: 1, StrandName: attemptStrand, Delivered: []string{"## old"}}},
				Fixers:   []*weaverengine.FixerState{{Number: 1, StrandName: fixerStrand}},
			}
			if err := weaverengine.SaveState(runDir, scratchDir, planted); err != nil {
				t.Fatal(err)
			}

			// A second call records nothing more: a heading is kept once.
			for range 2 {
				if err := weaverengine.RecordDelivery(anchor, tt.strand, tt.headings); err != nil {
					t.Fatalf("RecordDelivery() error = %v", err)
				}
			}

			got, err := weaverengine.LoadState(runDir, scratchDir)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.wantAttempt, got.Attempts[0].Delivered); diff != "" {
				t.Errorf("attempt Delivered mismatch (-want +got):\n%s", diff)
			}
			if !slices.Equal(got.Fixers[0].Delivered, tt.wantFixer) {
				t.Errorf("fixer Delivered = %v; want %v", got.Fixers[0].Delivered, tt.wantFixer)
			}
		})
	}

	t.Run("AbsentStateChangesNothing", func(t *testing.T) {
		t.Parallel()
		anchor := t.TempDir()
		if err := weaverengine.RecordDelivery(anchor, attemptStrand, []string{"## a"}); err != nil {
			t.Fatalf("RecordDelivery() error = %v", err)
		}
		got, err := weaverengine.LoadState(weaverengine.Dir(anchor), weaverengine.ScratchDir(anchor))
		if err != nil || got != nil {
			t.Errorf("LoadState() = %v, %v; want nil, nil", got, err)
		}
	})
}
