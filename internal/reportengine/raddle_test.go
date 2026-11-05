// raddle_test.go tests ComputeRaddle over a fake board holding a raddle store and a fake records repository holding the filed parts' runs.

package reportengine

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// proposalText is the text of a proposal file whose expected cost is expected, a JSON number or null.
func proposalText(expected string) string {
	return fmt.Sprintf(`{"sketch":"s","cost":{"expected_usd":%s}}`, expected)
}

// storeSketch commits the store directory of sketch to the board: its store.json recording filed and project, the approved proposal and the cold proposal.
func storeSketch(t *testing.T, w *world, sketch string, filed []string, project, approved, cold string) {
	t.Helper()

	record, err := json.Marshal(boardengine.RaddleStoreEntry{Hash: raddleparser.Hash([]byte(approved)), Filed: filed, Project: project})
	if err != nil {
		t.Fatalf("marshal store record: %v", err)
	}
	w.board.commitOnChain("store-"+sketch, at(50), map[string]string{
		boardengine.RaddleStoreRecordRel(sketch):  string(record),
		boardengine.RaddleProposalRel(sketch):     approved,
		boardengine.RaddleColdProposalRel(sketch): cold,
	})
}

// landedPart adds the archive tag of the filed part slug at hour, its run holding one session per cost, each a JSON number or null.
func landedPart(w *world, slug string, hour int, costs ...string) {
	sessions := map[string]string{}
	for i, cost := range costs {
		sessions[fmt.Sprint(i)] = sessionText(cost, time.Time{}, time.Time{})
	}
	w.tagWith(slug, "sha-"+slug, at(hour), seedText("tapestry"), "", usageFiles(sessions, runText("tapestry", "p", false), ""))
}

// TestComputeRaddle covers the predicted and actual cost of a Sketch, its unlanded parts, a changed proposal, a partial actual cost, the filed project and an undecodable store file.
func TestComputeRaddle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		build        func(t *testing.T, w *world)
		want         []RaddleRow
		wantFindings []string
	}{
		{
			name: "AProposalWhosePartsAllLandedSetsPredictedAgainstActual",
			build: func(t *testing.T, w *world) {
				storeSketch(t, w, "alpha", []string{"a1", "a2"}, "", proposalText("4.5"), proposalText("4.5"))
				landedPart(w, "a1", 40, "2")
				landedPart(w, "a2", 41, "3")
			},
			want: []RaddleRow{{Sketch: "alpha", PredictedUSD: figure(4.5), ActualUSD: figure(5), Unlanded: []string{}}},
		},
		{
			name: "AnUnlandedPartIsListed",
			build: func(t *testing.T, w *world) {
				storeSketch(t, w, "beta", []string{"b1", "b2"}, "", proposalText("2"), proposalText("2"))
				landedPart(w, "b1", 40, "1")
			},
			want: []RaddleRow{{Sketch: "beta", PredictedUSD: figure(2), ActualUSD: figure(1), Unlanded: []string{"b2"}}},
		},
		{
			name: "AProposalTheOperatorChangedIsFlaggedAndANullPredictionStaysNull",
			build: func(t *testing.T, w *world) {
				storeSketch(t, w, "gamma", []string{"g1"}, "", proposalText("null"), proposalText("8"))
				landedPart(w, "g1", 40, "1")
			},
			want: []RaddleRow{{Sketch: "gamma", ActualUSD: figure(1), Unlanded: []string{}, ProposalChanged: true}},
		},
		{
			name: "AnUnknownCostSessionMakesTheActualPartial",
			build: func(t *testing.T, w *world) {
				storeSketch(t, w, "delta", []string{"d1"}, "", proposalText("3"), proposalText("3"))
				landedPart(w, "d1", 40, "1.5", "null")
			},
			want: []RaddleRow{{Sketch: "delta", PredictedUSD: figure(3), ActualUSD: figure(1.5), ActualPartial: true, Unlanded: []string{}}},
		},
		{
			name: "TheFiledProjectIsNoPart",
			build: func(t *testing.T, w *world) {
				storeSketch(t, w, "zeta", []string{"zproj", "z1"}, "zproj", proposalText("2"), proposalText("2"))
				landedPart(w, "z1", 40, "2")
			},
			want: []RaddleRow{{Sketch: "zeta", PredictedUSD: figure(2), ActualUSD: figure(2), Unlanded: []string{}}},
		},
		{
			name: "AnUndecodableStoreFileIsAFindingAndNoRow",
			build: func(t *testing.T, w *world) {
				w.board.commitOnChain("store-eps", at(50), map[string]string{boardengine.RaddleStoreRecordRel("eps"): "{"})
			},
			want:         []RaddleRow{},
			wantFindings: []string{boardengine.RaddleStoreRecordRel("eps")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			tt.build(t, w)

			got, err := ComputeRaddle(w.sources())

			if err != nil {
				t.Fatalf("ComputeRaddle() error = %v; want nil", err)
			}
			if diff := cmp.Diff(tt.want, got.Rows); diff != "" {
				t.Errorf("ComputeRaddle() rows mismatch (-want +got):\n%s", diff)
			}
			var gotFindings []string
			for _, finding := range got.Findings {
				gotFindings = append(gotFindings, finding.File)
			}
			if diff := cmp.Diff(tt.wantFindings, gotFindings); diff != "" {
				t.Errorf("ComputeRaddle() finding files mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
