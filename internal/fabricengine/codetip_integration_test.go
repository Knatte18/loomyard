// codetip_integration_test.go covers OriginCodeTip over pair fixtures whose records origin holds planted transitions.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOriginCodeTip reads the code tip the last transition on origin names, from the refs a fetch left.
func TestOriginCodeTip(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "tipread"
	tests := []struct {
		name string
		// plant makes origin's records branch hold transitions and returns the reading wanted, its time aside.
		plant func(t *testing.T, p *pairFixture) fabriclayout.CodeTipReading
		slug  string
	}{
		{
			"ATipOriginsCodeBranchHoldsIsNotMissing",
			func(t *testing.T, p *pairFixture) fabriclayout.CodeTipReading {
				held := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
				plantRunTransition(t, p, slug, held, "oslo")
				return fabriclayout.CodeTipReading{Tip: held}
			},
			slug,
		},
		{
			"ATipOriginsCodeBranchLacksIsMissing",
			func(t *testing.T, p *pairFixture) fabriclayout.CodeTipReading {
				lacked := gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp\n", "unpushed code")
				plantRunTransition(t, p, slug, lacked, "oslo")
				return fabriclayout.CodeTipReading{Tip: lacked, Missing: true}
			},
			slug,
		},
		{
			"ALaterFabricCommitWithoutTheTransitionTrailerDoesNotMoveTheTip",
			func(t *testing.T, p *pairFixture) fabriclayout.CodeTipReading {
				lacked := gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp\n", "unpushed code")
				plantRunTransition(t, p, slug, lacked, "oslo")
				held := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
				p.originCommit(p.weftBranch, "fabric-note.txt", "note\n", "fabric commit\n\n"+fabriclayout.WarpSHATrailerKey+": "+held)
				return fabriclayout.CodeTipReading{Tip: lacked, Missing: true}
			},
			slug,
		},
		{
			"ASlugWithNoRecordsBranchOnOriginReadsNotMissing",
			func(t *testing.T, p *pairFixture) fabriclayout.CodeTipReading {
				return fabriclayout.CodeTipReading{}
			},
			"ghost",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newPairFixture(t, slug)
			want := tt.plant(t, p)
			gitkit.Git(t, p.weftPath, "fetch")

			got, err := fabricengine.OriginCodeTip(p.hub.Location, tt.slug)

			if err != nil {
				t.Fatalf("OriginCodeTip(%q) error = %v; want none", tt.slug, err)
			}
			wantAt := want.Tip != ""
			if got.At.IsZero() == wantAt {
				t.Errorf("OriginCodeTip(%q) time = %v; want a commit time exactly when a tip is named", tt.slug, got.At)
			}
			got.At = time.Time{}
			// The machine is the status codec's verdict, which this test takes as given.
			got.Machine = ""
			if got != want {
				t.Errorf("OriginCodeTip(%q) = %+v; want %+v", tt.slug, got, want)
			}
		})
	}
}
