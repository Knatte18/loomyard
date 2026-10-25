//go:build tier1

// verifygate_integration_test.go drives the verify gate's closure over a real scratch repository:
// the clean-tree check at a done arrival, and the fixer commit check through the real fixCommitRejection.
// It reuses the package's hermetic TestMain (testmain_test.go) and gitwrap_test.go's merge helpers.
// The gate runs the built `lyx` over a committed gate.yaml, so every test here puts that binary first on `PATH` and is not parallel.

package websterengine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyGateRepo is a scratch repository whose gate.yaml `final` tier leaves a sentinel file outside the worktree when it runs.
type verifyGateRepo struct {
	geom     Geometry
	sentinel string
}

// verifyGateConfig returns the gate.yaml a scratch worktree holds: a `webster` key over tier0, which runs a shell stand-in that writes sentinel.
func verifyGateConfig(sentinel string) string {
	return `slots: 1
scoper: none
tiers:
  tier0:
    about: shell stand-in
    command: ": > '` + filepath.ToSlash(sentinel) + `'"
    timeout_min: 1
    counter: go-json
gates:
  card: {tiers: [0], scope: card}
  batch: {tiers: [0], scope: card}
  webster: {tiers: [0], scope: all}
  parent-align: {tiers: [0], scope: all}
  finalize: {tiers: [0], scope: all}
  webster-fix: {tiers: [0], scope: diff}
  darn-write: {tiers: [0], scope: diff}
  code-burler-fix: {tiers: [0], scope: diff}
`
}

// newVerifyGateRepo builds the repo, a done outcome.yaml and a committed gate.yaml whose `final` tier writes the sentinel.
// A `lyx gate test` run records under the worktree's .lyx directory, which the repo excludes so the tree the gate verifies stays clean.
// It puts the `lyx` built from the tree first on `PATH`, so the calling test is not parallel.
func newVerifyGateRepo(t *testing.T) verifyGateRepo {
	t.Helper()
	lyxbin.PutFirstOnPath(t)
	dir := gitwrapNewScratchRepo(t)
	exclude := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		t.Fatalf("create the exclude directory: %v", err)
	}
	if err := os.WriteFile(exclude, []byte(lyxdirs.DotLyxDirName+"/\n"), 0o644); err != nil {
		t.Fatalf("write the exclude file: %v", err)
	}
	sentinel := filepath.Join(t.TempDir(), "verify-ran")
	configRel, err := filepath.Rel(dir, configengine.ConfigFile(dir, "gate"))
	if err != nil {
		t.Fatalf("locate gate.yaml: %v", err)
	}
	gitkit.CommitFile(t, dir, filepath.ToSlash(configRel), verifyGateConfig(sentinel), "gate config")
	gitkit.CommitFile(t, dir, "base.txt", "base", "base")

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
			VerifyDir:    t.TempDir(),
			GateConfig: func() (gateslot.Config, error) {
				return gateslot.LoadConfig(configengine.Dirs{Anchor: dir})
			},
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

// TestVerifyGate_TellsTheGeometryHoldOnTheFirstRunAndTheRerun pins that the gate tells geom.VerifyHold to every verify it runs:
// a verify that fails on its first run and passes on the rerun waits out the pause once before each.
func TestVerifyGate_TellsTheGeometryHoldOnTheFirstRunAndTheRerun(t *testing.T) {
	r := newVerifyGateRepo(t)
	failedOnce := filepath.ToSlash(filepath.Join(t.TempDir(), "failed-once"))
	config := strings.Replace(verifyGateConfig(r.sentinel),
		`": > '`+filepath.ToSlash(r.sentinel)+`'"`,
		`"[ -e '`+failedOnce+`' ] || { : > '`+failedOnce+`'; exit 1; }"`, 1)
	configRel, err := filepath.Rel(r.geom.WorktreeRoot, configengine.ConfigFile(r.geom.WorktreeRoot, "gate"))
	if err != nil {
		t.Fatalf("locate gate.yaml: %v", err)
	}
	gitkit.CommitFile(t, r.geom.WorktreeRoot, filepath.ToSlash(configRel), config, "failing-once gate config")

	waits := 0
	r.geom.VerifyHold = &verifytree.PauseHold{
		PastGrace: func() bool { return false },
		Wait: func(context.Context, func()) error {
			waits++
			return nil
		},
		Poll: time.Hour,
	}
	gate, _ := NewVerifyGate(r.geom, 3, nil, nil, "", testFixPromptPath)

	res, err := gate()
	if err != nil || !res.Passed {
		t.Fatalf("gate() = %+v, %v; want a pass on rerun", res, err)
	}
	if waits != 2 {
		t.Errorf("the hold waited %d time(s); want once before the first run and once before the rerun", waits)
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
