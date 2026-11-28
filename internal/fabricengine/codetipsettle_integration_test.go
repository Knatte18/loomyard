// codetipsettle_integration_test.go covers ReturnToPreviousTransition.
// Its pair fixtures have a records origin that holds transitions.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// plantTwoTransitions makes origin's records branch hold two transitions of slug, with a fabric commit between and after.
// The first names the tip origin's code branch holds and the second a commit only the pair's code branch holds.
// The pair's code HEAD then moves past the second.
// It returns the first tip and the second.
func plantTwoTransitions(t *testing.T, p *pairFixture, slug string) (first, second string) {
	t.Helper()

	first = bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
	plantRunTransition(t, p, slug, first, "oslo")
	p.originCommit(p.weftBranch, "fabric-note-1.txt", "note\n", "fabric commit\n\n"+fabriclayout.WarpSHATrailerKey+": "+first)
	second = gitkit.CommitFile(t, p.warpPath, "second.txt", "second\n", "second tip")
	plantRunTransition(t, p, slug, second, "bergen")
	gitkit.CommitFile(t, p.warpPath, "later.txt", "later\n", "later local code")
	p.originCommit(p.weftBranch, "fabric-note-2.txt", "note\n", "fabric commit\n\n"+fabriclayout.WarpSHATrailerKey+": "+first)
	gitkit.Git(t, p.weftPath, "fetch")
	gitkit.Git(t, p.weftPath, "merge", "--ff-only", "origin/"+p.weftBranch)
	return first, second
}

// TestOneRepositoryPairTips_RefuseARecordsStoreThatLostItsFetchRefspecs asserts the settle reads the layout first.
// With the records store's origin fetch refspecs unset, a fetch moves no tracking ref of the records side.
// Each read refuses naming the refspecs to restore, and leaves the tracking refs where they were.
func TestOneRepositoryPairTips_RefuseARecordsStoreThatLostItsFetchRefspecs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "tips-layout"
	const wantErrorHas = "config --replace-all remote.origin.fetch"
	tests := []struct {
		name string
		read func(pair *lymcwd.Location) error
	}{
		{"FetchPairTips", func(pair *lymcwd.Location) error { return fabricengine.FetchPairTips(pair) }},
		{"FetchRecords", func(pair *lymcwd.Location) error { return fabricengine.FetchRecords(pair) }},
		{"ReturnToPreviousTransition", func(pair *lymcwd.Location) error {
			_, err := fabricengine.ReturnToPreviousTransition(pair, slug)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
			recordsBranch := fabriclayout.RecordsBranchName(hubforge.AddPair(t, h, slug).Branch)
			sibling := h.PairRecordsSibling(slug)
			before := gitkit.RevParse(t, sibling, "HEAD")
			gitkit.CommitFile(t, sibling, "ahead.txt", "ahead\n", "origin's next records commit")
			oneRepoPushNamespaced(t, sibling, recordsBranch)
			gitkit.Git(t, sibling, "reset", "--hard", before)
			trackingBefore := gitkit.Git(t, sibling, "for-each-ref", "--format=%(refname) %(objectname)", "refs/remotes/")
			gitkit.Git(t, sibling, "config", "--unset-all", "remote.origin.fetch")
			fabricengine.ForgetOriginNamespaceForTest(sibling)
			pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
			if err != nil {
				t.Fatalf("ResolveWorktree(%s): %v", slug, err)
			}

			err = tt.read(pair)

			if err == nil || !strings.Contains(err.Error(), wantErrorHas) {
				t.Fatalf("%s error = %v; want one naming %q", tt.name, err, wantErrorHas)
			}
			if got := gitkit.Git(t, sibling, "for-each-ref", "--format=%(refname) %(objectname)", "refs/remotes/"); got != trackingBefore {
				t.Errorf("tracking refs = %q after the refused read; want them left as %q", got, trackingBefore)
			}
		})
	}
}

// TestReturnToPreviousTransition gives up the last transition's code tip by committing the transition before it.
func TestReturnToPreviousTransition(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "giveup"
	_, statusRel, err := shedrun.TreeRunFileRels(slug)
	if err != nil {
		t.Fatalf("TreeRunFileRels(%q): %v", slug, err)
	}

	t.Run("CommitsThePreviousTransitionsStatusAndNamesItsTipNotThePairsCodeHead", func(t *testing.T) {
		t.Parallel()
		p := newPairFixture(t, slug)
		first, second := plantTwoTransitions(t, p, slug)
		codeHead := gitkit.RevParse(t, p.warpPath, "HEAD")

		returned, err := fabricengine.ReturnToPreviousTransition(p.location, slug)

		if err != nil || !returned {
			t.Fatalf("ReturnToPreviousTransition(%q) = %v, %v; want true, nil", slug, returned, err)
		}
		message := gitkit.Git(t, p.weftPath, "log", "-1", "--format=%B")
		for _, want := range []string{
			fabriclayout.WarpSHATrailerKey + ": " + first,
			fabriclayout.TransitionTrailerKey + ": " + slug,
		} {
			if !strings.Contains(message, want) {
				t.Errorf("commit message = %q; want it to hold %q", message, want)
			}
		}
		for _, unwanted := range []string{second, codeHead} {
			if strings.Contains(message, unwanted) {
				t.Errorf("commit message = %q; want it to name neither the given-up tip nor the pair's code HEAD (%s)", message, unwanted)
			}
		}
		var status shedengine.Status
		if err := json.Unmarshal([]byte(gitkit.Git(t, p.weftPath, "show", "HEAD:"+statusRel)), &status); err != nil {
			t.Fatalf("decode the committed status: %v", err)
		}
		if len(status.History) != 1 || status.History[0].Machine != "oslo" {
			t.Errorf("committed status history = %+v; want the first transition's single entry of machine oslo", status.History)
		}
	})

	t.Run("AGivenUpTipStaysOffOriginWhenTheOwingMachineHandsOver", func(t *testing.T) {
		t.Parallel()
		p := newPairFixture(t, slug)
		first, _ := plantTwoTransitions(t, p, slug)
		if returned, err := fabricengine.ReturnToPreviousTransition(p.location, slug); err != nil || !returned {
			t.Fatalf("ReturnToPreviousTransition(%q) = %v, %v; want true, nil", slug, returned, err)
		}
		gitkit.Git(t, p.weftPath, "push")
		entry := shedengine.Takeover{Machine: "oslo", Subject: "sub", At: "2026-11-17T10:00:00Z"}

		res, err := fabricengine.HandOverTips(p.hub.Location, fabricengine.HandOverRequest{Slug: slug, Pair: p.location, Entry: entry})

		if err != nil {
			t.Fatalf("HandOverTips() error = %v; want none", err)
		}
		if res.TipPushed {
			t.Errorf("HandOverTips() TipPushed = true; want false for a tip the last transition no longer names")
		}
		if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != first {
			t.Errorf("origin's code branch = %s; want %s, the tip the previous transition names", got, first)
		}
	})

	t.Run("ARecordsBranchWithOneTransitionReturnsFalseAndCommitsNothing", func(t *testing.T) {
		t.Parallel()
		p := newPairFixture(t, slug)
		plantRunTransition(t, p, slug, bareBranchTip(t, p.hub.CodeBare, p.warpBranch), "oslo")
		headBefore := gitkit.RevParse(t, p.weftPath, "HEAD")

		returned, err := fabricengine.ReturnToPreviousTransition(p.location, slug)

		if err != nil || returned {
			t.Fatalf("ReturnToPreviousTransition(%q) = %v, %v; want false, nil", slug, returned, err)
		}
		if got := gitkit.RevParse(t, p.weftPath, "HEAD"); got != headBefore {
			t.Errorf("records HEAD = %s; want %s, unchanged", got, headBefore)
		}
	})
}
