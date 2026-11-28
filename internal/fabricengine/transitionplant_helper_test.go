// transitionplant_helper_test.go plants a run transition on a pair's records origin.
// The code-tip tests and the pair push's owner-guard tests share it.

package fabricengine_test

import (
	"encoding/json"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
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
	message := "transition\n\n" + fabriclayout.TransitionTrailerKey + ": " + slug + "\n" + fabriclayout.WarpSHATrailerKey + ": " + codeTip
	p.originCommit(p.weftBranch, statusRel, string(record), message)
	gitkit.Git(t, p.weftPath, "fetch")
	gitkit.Git(t, p.weftPath, "merge", "--ff-only", "origin/"+p.weftBranch)
}
