//go:build integration

// verifygate_integration_test.go drives the verify gate's closure over a real scratch repository:
// the clean-tree check at a done arrival, and the fixer commit check through the real fixCommitRejection.
// It reuses the package's hermetic TestMain (testmain_test.go) and gitwrap_test.go's merge helpers.

package websterengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// verifyGateRepo is a scratch repository with a plan whose verify command leaves a sentinel file outside the worktree when it runs.
type verifyGateRepo struct {
	geom     Geometry
	sentinel string
}

// newVerifyGateRepo builds the repo, a done outcome.yaml and a plan carrying a verify command that writes the sentinel.
func newVerifyGateRepo(t *testing.T) verifyGateRepo {
	t.Helper()
	dir := gitwrapNewScratchRepo(t)
	gitkit.CommitFile(t, dir, "base.txt", "base", "base")

	sentinel := filepath.Join(t.TempDir(), "verify-ran")
	planDir := t.TempDir()
	plankit.Write(t, planDir, plankit.Plan{
		Approved: true,
		Framing:  "Framing.",
		Sections: []plankit.Section{{Heading: "verify:", Body: "```\n: > " + sentinel + "\n```"}},
		Cards: []plankit.Card{{
			Number: 1, Slug: "one", Summary: "one", Intent: "placeholder card.",
			Groups: []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}},
		}},
	})

	websterDir := t.TempDir()
	if err := os.WriteFile(OutcomePath(websterDir), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
		t.Fatalf("write outcome.yaml: %v", err)
	}
	return verifyGateRepo{
		geom: Geometry{
			WorktreeRoot: dir,
			WebsterDir:   websterDir,
			ScratchDir:   t.TempDir(),
			ReportsDir:   t.TempDir(),
			PlanDir:      planDir,
			VerifyDir:    t.TempDir(),
		},
		sentinel: sentinel,
	}
}

// verifyRan reports whether the plan's verify command ran.
func (r verifyGateRepo) verifyRan() bool {
	_, err := os.Stat(r.sentinel)
	return err == nil
}

func TestVerifyGate_DirtyTreeFailsWithPathsAndRunsNoVerify(t *testing.T) {
	r := newVerifyGateRepo(t)
	if err := os.WriteFile(filepath.Join(r.geom.WorktreeRoot, "loose.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write loose file: %v", err)
	}
	gate, _ := NewVerifyGate(r.geom, 3, nil, nil, "", testFixPromptPath)

	res, err := gate()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	if res.Passed || res.Terminal {
		t.Fatalf("gate() = %+v; want a plain failure", res)
	}
	if !strings.Contains(res.Findings, "loose.txt") {
		t.Errorf("findings = %q; want the dirty path", res.Findings)
	}
	if r.verifyRan() {
		t.Error("the verify command ran on a dirty tree; want it skipped")
	}
}

func TestVerifyGate_CleanTreePassesAndRecordsTheTree(t *testing.T) {
	r := newVerifyGateRepo(t)
	gate, _ := NewVerifyGate(r.geom, 3, nil, nil, "", testFixPromptPath)

	res, err := gate()
	if err != nil || !res.Passed {
		t.Fatalf("gate() = %+v, %v; want a pass", res, err)
	}
	if !r.verifyRan() {
		t.Error("the verify command did not run on a clean tree")
	}
}

// failFirstThenFixerMerge drives gate through a dirty first arrival, so a pre-fix head is recorded, then cleans the tree and lets move add the fixer's commits.
func failFirstThenFixerMerge(t *testing.T, r verifyGateRepo, gate shuttleengine.Gate, move func()) shuttleengine.GateResult {
	t.Helper()
	loose := filepath.Join(r.geom.WorktreeRoot, "loose.txt")
	if err := os.WriteFile(loose, []byte("x"), 0o644); err != nil {
		t.Fatalf("write loose file: %v", err)
	}
	if res, err := gate(); err != nil || res.Passed {
		t.Fatalf("first gate() = %+v, %v; want a dirty failure", res, err)
	}
	if err := os.Remove(loose); err != nil {
		t.Fatalf("remove loose file: %v", err)
	}
	move()
	res, err := gate()
	if err != nil {
		t.Fatalf("second gate() error = %v", err)
	}
	return res
}

func TestVerifyGate_MergeAboveTheFixBaseOtherThanACleanParentMergeFailsTerminal(t *testing.T) {
	r := newVerifyGateRepo(t)
	gate, _ := NewVerifyGate(r.geom, 3, nil, gitwrapParent, "", testFixPromptPath)

	res := failFirstThenFixerMerge(t, r, gate, func() {
		gitwrapParentCommit(t, r.geom.WorktreeRoot, "p.txt", "p")
		gitwrapMergeSide(t, r.geom.WorktreeRoot, "unrelated")
	})
	if res.Passed || !res.Terminal {
		t.Fatalf("second gate() = %+v; want a Terminal failure", res)
	}
	if !strings.Contains(res.Findings, "not on the run's parent branch") {
		t.Errorf("findings = %q; want the rejection reason", res.Findings)
	}
	if r.verifyRan() {
		t.Error("the verify command ran after a rejected fix commit; want it skipped")
	}
}

func TestVerifyGate_CleanParentMergeAboveTheFixBasePasses(t *testing.T) {
	r := newVerifyGateRepo(t)
	gate, _ := NewVerifyGate(r.geom, 3, nil, gitwrapParent, "", testFixPromptPath)

	res := failFirstThenFixerMerge(t, r, gate, func() {
		gitkit.CommitFile(t, r.geom.WorktreeRoot, "fix.txt", "fix", "fix")
		gitwrapMergeSide(t, r.geom.WorktreeRoot, gitwrapParentBranch)
	})
	if res.Terminal {
		t.Fatalf("second gate() = %+v; want the clean parent merge accepted", res)
	}
	if !res.Passed {
		t.Fatalf("second gate() = %+v; want a pass once the tree is clean and verify succeeds", res)
	}
}

func TestVerifyGate_NoParentBranchAcceptsNoMerge(t *testing.T) {
	r := newVerifyGateRepo(t)
	gate, _ := NewVerifyGate(r.geom, 3, nil, nil, "", testFixPromptPath)

	res := failFirstThenFixerMerge(t, r, gate, func() {
		gitwrapMergeSide(t, r.geom.WorktreeRoot, gitwrapParentBranch)
	})
	if res.Passed || !res.Terminal {
		t.Fatalf("second gate() = %+v; want a Terminal failure, since standalone mode accepts no merge", res)
	}
}
