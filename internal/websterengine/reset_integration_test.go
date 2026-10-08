//go:build integration

// reset_integration_test.go drives PlanReset over a scratch git repository under t.TempDir():
// each refusal after the lock and state checks, the octopus merge-base of diverging starts, and the own-path set.
// It reuses the package's hermetic TestMain (testmain_test.go) and gitwrap_test.go's scratch-repo initialiser.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// resetFixture is a scratch repo on branch "task" with two commits: first, then head.
type resetFixture struct {
	root  string
	first string
	head  string
	deps  ResetDeps
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	root := gitwrapNewScratchRepo(t)
	first := gitkit.CommitFile(t, root, "a.txt", "one", "first")
	gitkit.Git(t, root, "checkout", "-b", "task")
	head := gitkit.CommitFile(t, root, "b.txt", "two", "second")
	return &resetFixture{root: root, first: first, head: head}
}

// step runs one case of the scenario against a fresh set of deps: one recorded batch started at the first commit, on branch "task" whose parent branch is "main".
// A step that changes the working tree or the index restores them before it ends, so the steps share the repository and none relies on another's state.
func (fx *resetFixture) step(t *testing.T, name string, run func(t *testing.T)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		fx.deps = ResetDeps{
			Geom:         Geometry{WorktreeRoot: fx.root, ScratchDir: t.TempDir()},
			State:        &State{MasterSessionID: "s1", Batches: map[int]*BatchState{1: {Slug: "one", StartSHA: fx.first}}},
			Engine:       &shuttlefake.Engine{},
			ParentBranch: func() (string, error) { return "main", nil },
			Branch:       func() (string, error) { return "task", nil },
		}
		run(t)
	})
}

func (fx *resetFixture) wantRefusal(t *testing.T, to ResetTarget, parts ...string) {
	t.Helper()
	fx.wantBatchRefusal(t, to, 0, parts...)
}

// wantBatchRefusal is wantRefusal for a target that takes --batch.
func (fx *resetFixture) wantBatchRefusal(t *testing.T, to ResetTarget, batch int, parts ...string) {
	t.Helper()
	_, err := PlanReset(fx.deps, to, batch)
	if err == nil {
		t.Fatalf("PlanReset(%s) error = nil, want a refusal containing %q", to, parts)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("PlanReset(%s) error = %v, want it to contain %q", to, err, p)
		}
	}
}

// wantPlanSHA asserts PlanReset resolves to to, with batch, to the commit want.
func (fx *resetFixture) wantPlanSHA(t *testing.T, to ResetTarget, batch int, want string) {
	t.Helper()
	plan, err := PlanReset(fx.deps, to, batch)
	if err != nil {
		t.Fatalf("PlanReset(%s) error = %v, want a plan", to, err)
	}
	if plan.SHA != want || plan.Target != to {
		t.Errorf("PlanReset(%s) = %+v, want SHA %s", to, plan, want)
	}
}

// TestPlanReset runs every PlanReset case over one scratch repository, one step per case.
func TestPlanReset(t *testing.T) {
	t.Parallel()
	fx := newResetFixture(t)

	fx.step(t, "merge in progress refuses", func(t *testing.T) {
		mergeHead := filepath.Join(fx.root, ".git", "MERGE_HEAD")
		if err := os.WriteFile(mergeHead, []byte(fx.first+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(mergeHead) })
		fx.wantRefusal(t, ResetToStart, "merge in progress")
	})

	fx.step(t, "detached head refuses", func(t *testing.T) {
		fx.deps.Branch = func() (string, error) { return "", errors.New("HEAD is detached") }
		fx.wantRefusal(t, ResetToStart, "HEAD is detached", "git switch", "re-run `lyx webster reset --to start`")
	})

	fx.step(t, "parent branch refuses", func(t *testing.T) {
		fx.deps.Branch = func() (string, error) { return "main", nil }
		fx.wantRefusal(t, ResetToPreFix, "parent branch", "git switch")
	})

	fx.step(t, "no recorded target refuses", func(t *testing.T) {
		fx.deps.State.Batches = map[int]*BatchState{}
		fx.wantRefusal(t, ResetToPreFix, "no pre-fix head", "way forward: run `lyx webster run`")
	})

	fx.step(t, "missing commit refuses", func(t *testing.T) {
		bogus := strings.Repeat("ab", 20)
		fx.deps.State.PreFixHead = bogus
		fx.wantRefusal(t, ResetToPreFix, bogus, "not in this repository")
	})

	// The side branch this step leaves holds a commit outside "task", which no later step reads.
	fx.step(t, "not an ancestor refuses", func(t *testing.T) {
		gitkit.Git(t, fx.root, "checkout", "-b", "side", fx.first)
		side := gitkit.CommitFile(t, fx.root, "c.txt", "side", "side")
		gitkit.Git(t, fx.root, "checkout", "task")
		fx.deps.State.PreFixHead = side
		fx.wantRefusal(t, ResetToPreFix, "not an ancestor of HEAD", "lyx webster reset --to start")
	})

	// The orphan commits this step makes sit on no branch, so no later step reads them.
	fx.step(t, "start with no common ancestor archives without moving", func(t *testing.T) {
		const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
		orphan1 := gitkit.Git(t, fx.root, "commit-tree", emptyTree, "-m", "orphan 1")
		orphan2 := gitkit.Git(t, fx.root, "commit-tree", emptyTree, "-m", "orphan 2")
		fx.deps.State.Batches = map[int]*BatchState{
			1: {Slug: "one", StartSHA: orphan1},
			2: {Slug: "two", StartSHA: orphan2},
		}
		plan, err := PlanReset(fx.deps, ResetToStart, 0)
		if err != nil {
			t.Fatalf("PlanReset error = %v, want an archive-only plan", err)
		}
		if !plan.ArchiveOnly || plan.SHA != "" || !strings.Contains(plan.Reason, "no common ancestor") {
			t.Errorf("plan = %+v, want an archive-only plan whose reason names the missing common ancestor", plan)
		}
	})

	fx.step(t, "foreign dirt refuses and names the path", func(t *testing.T) {
		t.Cleanup(func() { gitkit.Git(t, fx.root, "checkout", "--", "b.txt") })
		if err := os.WriteFile(filepath.Join(fx.root, "b.txt"), []byte("edited"), 0o644); err != nil {
			t.Fatal(err)
		}
		fx.wantRefusal(t, ResetToStart, "b.txt", "git checkout -- <path>", "re-run `lyx webster reset --to start`")
	})

	fx.step(t, "untracked file is not dirt", func(t *testing.T) {
		scratch := filepath.Join(fx.root, "scratch.txt")
		if err := os.WriteFile(scratch, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(scratch) })
		plan, err := PlanReset(fx.deps, ResetToStart, 0)
		if err != nil {
			t.Fatalf("PlanReset error = %v, want a plan", err)
		}
		if plan.SHA != fx.first {
			t.Errorf("plan.SHA = %s, want %s", plan.SHA, fx.first)
		}
	})

	// The two start branches this step leaves hold commits outside "task", which no later step reads.
	fx.step(t, "start targets the octopus merge-base of diverging starts", func(t *testing.T) {
		gitkit.Git(t, fx.root, "checkout", "-b", "s1", fx.first)
		s1 := gitkit.CommitFile(t, fx.root, "s1.txt", "1", "s1")
		gitkit.Git(t, fx.root, "checkout", "-b", "s2", fx.first)
		s2 := gitkit.CommitFile(t, fx.root, "s2.txt", "2", "s2")
		gitkit.Git(t, fx.root, "checkout", "task")
		fx.deps.State.Batches = map[int]*BatchState{
			1: {Slug: "one", StartSHA: s1},
			2: {Slug: "two", StartSHA: s2},
		}
		plan, err := PlanReset(fx.deps, ResetToStart, 0)
		if err != nil {
			t.Fatalf("PlanReset error = %v, want a plan", err)
		}
		if plan.SHA != fx.first || plan.Target != ResetToStart {
			t.Errorf("plan = %+v, want start at the merge-base %s", plan, fx.first)
		}
	})

	fx.step(t, "own paths hold a succeeded write and omit a failed one", func(t *testing.T) {
		t.Cleanup(func() { gitkit.Git(t, fx.root, "checkout", "--", "a.txt", "b.txt") })
		fx.deps.State.PreFixHead = fx.first
		for _, name := range []string{"a.txt", "b.txt"} {
			if err := os.WriteFile(filepath.Join(fx.root, name), []byte("edited"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		fx.deps.Engine = &shuttlefake.Engine{AuditForksFn: func(string, string) (shuttleengine.ForkAudit, error) {
			return shuttleengine.ForkAudit{
				ParentWriteEvents: []shuttleengine.WriteEvent{
					{Path: filepath.Join(fx.root, "a.txt"), Succeeded: true},
					{Path: filepath.Join(fx.root, "b.txt"), Succeeded: false},
				},
			}, nil
		}}
		// b.txt is dirty and its only recorded write failed, so it is foreign dirt.
		fx.wantRefusal(t, ResetToPreFix, "b.txt")

		gitkit.Git(t, fx.root, "checkout", "--", "b.txt")
		plan, err := PlanReset(fx.deps, ResetToPreFix, 0)
		if err != nil {
			t.Fatalf("PlanReset error = %v, want a plan", err)
		}
		if want := []string{"a.txt"}; !reflect.DeepEqual(plan.OwnPaths, want) {
			t.Errorf("OwnPaths = %v, want %v", plan.OwnPaths, want)
		}
		if plan.SHA != fx.first {
			t.Errorf("plan.SHA = %s, want %s", plan.SHA, fx.first)
		}
	})

	fx.step(t, "last-batch-head resolves to the last recorded batch head", func(t *testing.T) {
		fx.deps.State.Batches[1].Terminal = true
		fx.deps.State.Batches[1].Digest = &Digest{HeadSHA: fx.first}
		fx.wantPlanSHA(t, ResetToLastBatchHead, 0, fx.first)
	})

	fx.step(t, "batch-start resolves to the batch's recorded start", func(t *testing.T) {
		fx.wantPlanSHA(t, ResetToBatchStart, 1, fx.first)
	})

	// The side branch this step leaves holds a commit outside "task", which no later step reads.
	fx.step(t, "report-head resolves to the report head and keeps the ancestry and dirt guards", func(t *testing.T) {
		fx.deps.Geom.ReportsDir = t.TempDir()
		writeReport := func(head string) {
			t.Helper()
			path := filepath.Join(fx.deps.Geom.ReportsDir, ReportFileName(1, "one"))
			if err := WriteReport(path, &Report{Status: ReportStatusOK, HeadSHA: head}); err != nil {
				t.Fatal(err)
			}
		}

		writeReport(fx.head)
		fx.wantPlanSHA(t, ResetToReportHead, 1, fx.head)

		gitkit.Git(t, fx.root, "checkout", "-b", "report-side", fx.first)
		side := gitkit.CommitFile(t, fx.root, "d.txt", "side", "side")
		gitkit.Git(t, fx.root, "checkout", "task")
		writeReport(side)
		fx.wantBatchRefusal(t, ResetToReportHead, 1, "not an ancestor of HEAD", "lyx webster reset --to start")

		writeReport(fx.first)
		t.Cleanup(func() { gitkit.Git(t, fx.root, "checkout", "--", "b.txt") })
		if err := os.WriteFile(filepath.Join(fx.root, "b.txt"), []byte("edited"), 0o644); err != nil {
			t.Fatal(err)
		}
		fx.wantBatchRefusal(t, ResetToReportHead, 1, "b.txt", "re-run `lyx webster reset --to report-head --batch 01`")
	})
}
