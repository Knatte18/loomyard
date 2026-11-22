// pawldrain_integration_test.go drives the drain closures of a beam's Beam-Await row over a real beam pair and real git.

package loomcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPawlDeps_DrainSeamsReadTheBeamsCommitsAndWork pins what the Beam-Await drain reads.
// HoldsCommits reads whether the beam holds a commit its parent branch lacks.
// WorkRemains reads true while an open member entry or a pair forked from the beam stands.
// The checks change the one hub's board, pairs and beam branch in turn.
func TestPawlDeps_DrainSeamsReadTheBeamsCommitsAndWork(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "dr-beam", fabricengine.AddOptions{SkipPush: true, Beam: true})
	beam, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("dr-beam"))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(dr-beam) error = %v; want nil", err)
	}
	deps, err := pawlDeps(beam, "dr-beam")
	if err != nil {
		t.Fatalf("pawlDeps() error = %v; want nil", err)
	}
	boardPath := filepath.Join(h.BoardDir(), "board.json")
	beamEntry := `{"id":0,"slug":"dr-beam","title":"Beam","kind":"project","labels":["bug"],"beam":true}`
	memberEntry := `{"id":1,"slug":"dr-member","title":"Member","kind":"task","labels":["bug"],"project":"dr-beam","status":"draft"}`

	if held, err := deps.HoldsCommits(); err != nil || held {
		t.Errorf("HoldsCommits() before a commit = %v, %v; want false, nil", held, err)
	}
	gitkit.CommitFile(t, h.PairCodeWorktree("dr-beam"), "drain.txt", "work\n", "work on the beam")
	if held, err := deps.HoldsCommits(); err != nil || !held {
		t.Errorf("HoldsCommits() after a commit = %v, %v; want true, nil", held, err)
	}

	writeConfigFile(t, boardPath, `{"version":1,"entries":[`+beamEntry+`]}`)
	if remains, err := deps.WorkRemains(); err != nil || remains {
		t.Errorf("WorkRemains() with no member and no child pair = %v, %v; want false, nil", remains, err)
	}

	writeConfigFile(t, boardPath, `{"version":1,"entries":[`+beamEntry+`,`+memberEntry+`]}`)
	if remains, err := deps.WorkRemains(); err != nil || !remains {
		t.Errorf("WorkRemains() with an open draft member = %v, %v; want true, nil", remains, err)
	}

	writeConfigFile(t, boardPath, `{"version":1,"entries":[`+beamEntry+`]}`)
	hubforge.AddPairWith(t, h, "dr-child", fabricengine.AddOptions{SkipPush: true, Parent: "dr-beam"})
	if remains, err := deps.WorkRemains(); err != nil || !remains {
		t.Errorf("WorkRemains() with a child pair = %v, %v; want true, nil", remains, err)
	}
}
