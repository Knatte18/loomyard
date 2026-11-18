// codetip_integration_test.go covers OriginCodeTip over pair fixtures whose records origin holds planted transitions.
// It also holds the helper that plants one, which the owner-guard rows of the pair push share.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// plantRunTransition commits a transition of the run slug on origin's records branch from a second clone.
// The commit names codeTip and carries a child record whose last history entry names machine.
// The pair's own records worktree then fast-forwards to it, so its next push of that branch is no rejection.
func plantRunTransition(t *testing.T, p *pairFixture, slug, codeTip, machine string) {
	t.Helper()

	_, statusRel, err := shedrun.TreeRunFileRels(slug)
	if err != nil {
		t.Fatalf("TreeRunFileRels(%q): %v", slug, err)
	}
	record, err := json.Marshal(shedengine.Status{
		State:   shedengine.StateRunning,
		History: []shedengine.HistoryEntry{{Producer: "Code-Burler", Outcome: shedengine.Done, Machine: machine}},
	})
	if err != nil {
		t.Fatalf("marshal the child record: %v", err)
	}
	message := "transition\n\n" + fabricengine.TransitionTrailerKey + ": " + slug + "\n" + fabricengine.WarpSHATrailerKey + ": " + codeTip
	p.originCommit(p.weftBranch, statusRel, string(record), message)
	gitkit.Git(t, p.weftPath, "fetch")
	gitkit.Git(t, p.weftPath, "merge", "--ff-only", "origin/"+p.weftBranch)
}

// TestOriginCodeTip reads the code tip the last transition on origin names, from the refs a fetch left.
func TestOriginCodeTip(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "tipread"
	tests := []struct {
		name string
		// plant makes origin's records branch hold transitions and returns the reading wanted, its time aside.
		plant func(t *testing.T, p *pairFixture) fabricengine.CodeTipReading
		slug  string
	}{
		{
			"ATipOriginsCodeBranchHoldsIsNotMissing",
			func(t *testing.T, p *pairFixture) fabricengine.CodeTipReading {
				held := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
				plantRunTransition(t, p, slug, held, "oslo")
				return fabricengine.CodeTipReading{Tip: held, Machine: "oslo"}
			},
			slug,
		},
		{
			"ATipOriginsCodeBranchLacksIsMissingWithItsMachine",
			func(t *testing.T, p *pairFixture) fabricengine.CodeTipReading {
				lacked := gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp\n", "unpushed code")
				plantRunTransition(t, p, slug, lacked, "oslo")
				return fabricengine.CodeTipReading{Tip: lacked, Missing: true, Machine: "oslo"}
			},
			slug,
		},
		{
			"ALaterFabricCommitWithoutTheTransitionTrailerDoesNotMoveTheTip",
			func(t *testing.T, p *pairFixture) fabricengine.CodeTipReading {
				lacked := gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp\n", "unpushed code")
				plantRunTransition(t, p, slug, lacked, "oslo")
				held := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
				p.originCommit(p.weftBranch, "fabric-note.txt", "note\n", "fabric commit\n\n"+fabricengine.WarpSHATrailerKey+": "+held)
				return fabricengine.CodeTipReading{Tip: lacked, Missing: true, Machine: "oslo"}
			},
			slug,
		},
		{
			"ASlugWithNoRecordsBranchOnOriginReadsNotMissing",
			func(t *testing.T, p *pairFixture) fabricengine.CodeTipReading {
				return fabricengine.CodeTipReading{}
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
			if got != want {
				t.Errorf("OriginCodeTip(%q) = %+v; want %+v", tt.slug, got, want)
			}
		})
	}
}
