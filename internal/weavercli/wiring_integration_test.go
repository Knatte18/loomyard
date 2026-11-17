// wiring_integration_test.go drives the seams resetDepsFor builds over a wired hub fixture.
// It runs Reset through them against a task pair:
// the state history, the plan read at a records commit, the code reset, the commit and the dirty tracked paths.

package weavercli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestResetDepsFor_SeamsResetATaskPairThroughTheHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "weaver-wiring"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, slug)
	taskTree := h.PairCodeWorktree(slug)
	loc, err := lymcwd.ResolveWorktree(taskTree)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", taskTree, err)
	}
	deps, err := resetDepsFor(loc)
	if err != nil {
		t.Fatalf("resetDepsFor() error = %v; want nil", err)
	}
	geom := deps.Geom

	start := gitkit.CommitFile(t, taskTree, "own.txt", "base", "own base")
	gitkit.CommitFile(t, taskTree, "later.txt", "later", "later commit")
	gitkit.Git(t, taskTree, "push", "origin", "HEAD")

	card := plankit.Card{Number: 1, Slug: "one", Groups: []plankit.Group{{Label: "Create", Targets: []string{"internal/x/one.go"}}}}
	plankit.Write(t, geom.PlanDir, plankit.Plan{Approved: true, Language: "none", Framing: "A test plan.", Cards: []plankit.Card{card}})
	cardPath := filepath.Join(geom.PlanDir, "01-one.md")
	original, err := os.ReadFile(cardPath)
	if err != nil {
		t.Fatal(err)
	}
	planRel, err := filepath.Rel(geom.AnchorRoot, geom.PlanDir)
	if err != nil {
		t.Fatal(err)
	}
	planCommit, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), loc, []string{filepath.ToSlash(planRel)}, "seed the plan", fabricengine.SyncOptions{})
	if err != nil || !committed {
		t.Fatalf("CommitAnchoredPaths(plan) committed = %v, err = %v", committed, err)
	}

	cards := []string{"01-one"}
	st := &weaverengine.State{
		RunGUID:           "guid",
		PlanRecordsCommit: planCommit,
		Attempts:          []*weaverengine.AttemptState{{Batch: 1, Number: 1, Outcome: "failed", Uncommitted: []string{"own.txt"}}},
	}
	st.Batches = map[int]*runplan.BatchState{1: {Slug: "one", Cards: cards, StartSHA: start, Kind: "strand"}}
	st.Partition = []runplan.PartitionBatch{{Cards: cards}}
	if err := os.MkdirAll(geom.RunDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := weaverengine.SaveState(geom.RunDir, geom.ScratchDir, st); err != nil {
		t.Fatal(err)
	}
	if err := deps.Commit("seed the run state"); err != nil {
		t.Fatalf("Commit() error = %v; want nil", err)
	}

	versions, err := deps.StateHistory()
	if err != nil || len(versions) != 1 || !strings.Contains(string(versions[0].Data), `"runGuid": "guid"`) && !strings.Contains(string(versions[0].Data), `"runGuid":"guid"`) {
		t.Fatalf("StateHistory() = %v, %v; want the committed state as its one version", versions, err)
	}
	if data, found, err := deps.ReadPlanAt(planCommit, filepath.Base(cardPath)); err != nil || !found || string(data) != string(original) {
		t.Fatalf("ReadPlanAt() = %q, %v, %v; want the card as the records commit held it", data, found, err)
	}

	if err := os.WriteFile(cardPath, []byte("edited since the run began\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskTree, "own.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskTree, "untracked.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dirty, err := deps.DirtyTracked(); err != nil || !slices.Equal(dirty, []string{"own.txt"}) {
		t.Fatalf("DirtyTracked() = %v, %v; want the tracked path alone", dirty, err)
	}

	result, err := weaverengine.Reset(deps, false)

	if err != nil {
		t.Fatalf("Reset() error = %v; want nil", err)
	}
	if got := gitkit.RevParse(t, taskTree, "HEAD"); got != start || result.StartSHA != start {
		t.Errorf("HEAD = %q, result start = %q; want the run's start %q", got, result.StartSHA, start)
	}
	if _, err := os.Stat(filepath.Join(taskTree, "later.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat later.txt = %v; want the later commit discarded", err)
	}
	if got, err := os.ReadFile(filepath.Join(taskTree, "own.txt")); err != nil || string(got) != "base" {
		t.Errorf("own.txt = %q, %v; want the run's own edit discarded", got, err)
	}
	if got, err := os.ReadFile(cardPath); err != nil || string(got) != string(original) {
		t.Errorf("card = %q, %v; want the plan as the run began it", got, err)
	}
	if _, err := os.Stat(filepath.Join(geom.RunDir, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat state.json = %v; want it archived", err)
	}
	if subject := gitkit.Git(t, h.PairRecordsSibling(slug), "log", "-1", "--format=%s"); strings.TrimSpace(subject) != "weaver: reset" {
		t.Errorf("records head subject = %q; want the reset's commit", subject)
	}
}
