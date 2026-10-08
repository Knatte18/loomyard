// reset_test.go pins the PlanReset refusals that need no real git: the held run lock, the missing state, the --batch pairing and each target's resolution refusal.
// Tier 1: package websterengine_test over the package's fakeGit, a reports directory under t.TempDir() and a fake reed; no git process.

package websterengine_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestPlanReset_HeldRunLockIsTransientBusy(t *testing.T) {
	t.Parallel()

	scratch := t.TempDir()
	// "run.lock" is the run lock's file name inside the scratch directory.
	held, locked, err := lock.TryAcquireWriteLock(filepath.Join(scratch, "run.lock"))
	if err != nil || !locked {
		t.Fatalf("hold run lock: locked=%v err=%v", locked, err)
	}
	defer held.Release()

	deps := websterengine.ResetDeps{
		Geom:         websterengine.Geometry{ScratchDir: scratch, Git: newFakeGit()},
		State:        &websterengine.State{},
		ParentBranch: func() (string, error) { return "main", nil },
		Branch:       func() (string, error) { return "task", nil },
	}
	_, err = websterengine.PlanReset(deps, websterengine.ResetToStart, 0)
	if !errors.Is(err, websterengine.ErrRunBusy) {
		t.Fatalf("PlanReset error = %v, want ErrRunBusy", err)
	}
	if !strings.Contains(err.Error(), "wait for it to finish, or check `lyx webster status`") {
		t.Errorf("error = %v, want the wait way forward", err)
	}

	// report-head is the one target Master runs inside its run, so it passes the lock and reaches the batch check.
	_, err = websterengine.PlanReset(deps, websterengine.ResetToReportHead, 1)
	if errors.Is(err, websterengine.ErrRunBusy) || err == nil || !strings.Contains(err.Error(), "batch 1 is not begun") {
		t.Errorf("PlanReset(report-head) under the lock = %v, want the not-begun refusal", err)
	}
}

func TestPlanReset_NoStateNamesRun(t *testing.T) {
	t.Parallel()

	_, err := websterengine.PlanReset(websterengine.ResetDeps{Geom: websterengine.Geometry{ScratchDir: t.TempDir()}}, websterengine.ResetToPreFix, 0)
	if err == nil {
		t.Fatal("PlanReset error = nil, want the no-state refusal")
	}
	if !strings.Contains(err.Error(), "way forward: run `lyx webster run` first") {
		t.Errorf("error = %v, want the run-first way forward", err)
	}
}

// resolutionFixture is a run in its second batch over a fake git: batch 1 done at head, batch 2 in flight from that head with a report on disk, batch 3 not begun.
type resolutionFixture struct {
	t     *testing.T
	git   *fakeGit
	state *websterengine.State
	reed  *shuttlefake.Reed
	geom  websterengine.Geometry
	// root is the run's start commit, batch1Head the head batch 1 recorded and batch2Head the head batch 2's report names.
	root, batch1Head, batch2Head string
}

func newResolutionFixture(t *testing.T) *resolutionFixture {
	t.Helper()
	git := newFakeGit()
	fx := &resolutionFixture{t: t, git: git, reed: &shuttlefake.Reed{}, root: git.head}
	fx.batch1Head = git.commit()
	fx.batch2Head = git.commit()
	fx.state = &websterengine.State{
		Partition: []websterengine.PartitionBatch{{Cards: []string{"01-one"}}, {Cards: []string{"02-two"}}, {Cards: []string{"03-three"}}},
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "one", Kind: "fork", StartSHA: fx.root, Terminal: true, Status: "done", Digest: &websterengine.Digest{HeadSHA: fx.batch1Head}},
			2: {Slug: "two", Kind: "fork", StartSHA: fx.batch1Head},
		},
	}
	fx.geom = websterengine.Geometry{ScratchDir: t.TempDir(), ReportsDir: t.TempDir(), WorktreeRoot: t.TempDir(), Git: git}
	fx.writeReport(2, "two", fx.batch2Head)
	return fx
}

// writeReport puts the OK report of batch number naming head in the reports directory.
func (fx *resolutionFixture) writeReport(number int, slug, head string) {
	fx.t.Helper()
	report := &websterengine.Report{Status: websterengine.ReportStatusOK, HeadSHA: head}
	if err := websterengine.WriteReport(filepath.Join(fx.geom.ReportsDir, websterengine.ReportFileName(number, slug)), report); err != nil {
		fx.t.Fatal(err)
	}
}

func (fx *resolutionFixture) deps() websterengine.ResetDeps {
	return websterengine.ResetDeps{
		Geom:         fx.geom,
		State:        fx.state,
		Reed:         fx.reed,
		ParentBranch: func() (string, error) { return "main", nil },
		Branch:       func() (string, error) { return "task", nil },
	}
}

// TestPlanReset_Resolution pins each refusal PlanReset reaches from state, report files, the reed seam and a fake Git alone:
// the --batch pairing, an unknown target, and each target's own resolution.
// The plans the targets resolve to are pinned in TestPlanReset over a real repository.
func TestPlanReset_Resolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		to    websterengine.ResetTarget
		batch int
		setup func(fx *resolutionFixture)
		want  []string
	}{
		{"unknown target", "bogus", 0, nil, []string{`"bogus" is not one of start, pre-fix, report-head, last-batch-head, batch-start`}},
		{"report-head needs a batch", websterengine.ResetToReportHead, 0, nil, []string{"needs the batch", "re-run `lyx webster reset --to report-head --batch NN`"}},
		{"batch-start needs a batch", websterengine.ResetToBatchStart, 0, nil, []string{"needs the batch", "re-run `lyx webster reset --to batch-start --batch NN`"}},
		{"start refuses a batch", websterengine.ResetToStart, 2, nil, []string{"--batch 2 does not apply", "re-run `lyx webster reset --to start` without --batch"}},
		{"last-batch-head refuses a batch", websterengine.ResetToLastBatchHead, 2, nil, []string{"--batch 2 does not apply", "re-run `lyx webster reset --to last-batch-head` without --batch"}},
		{"report-head of a batch not begun", websterengine.ResetToReportHead, 3, nil, []string{"batch 3 is not begun", "`lyx webster status`"}},
		{"report-head of a terminal batch", websterengine.ResetToReportHead, 1, nil, []string{"batch 1 already reached a terminal record (done)", "run `lyx webster reset --to last-batch-head`"}},
		{"report-head with a later begun batch", websterengine.ResetToReportHead, 2, func(fx *resolutionFixture) {
			fx.state.Batches[3] = &websterengine.BatchState{Slug: "three", Kind: "fork", StartSHA: fx.batch2Head}
		}, []string{"batch 3 was begun after batch 2", "run `lyx webster reset --to start`"}},
		{"report-head with a live recovery strand", websterengine.ResetToReportHead, 2, func(fx *resolutionFixture) {
			fx.state.Batches[2].Kind, fx.state.Batches[2].StrandGUID = "recovery", "strand-2"
			fx.reed.Strands = []reedengine.StrandStatus{{GUID: "strand-2", Live: true}}
		}, []string{"a recovery strand of batch 2 is live", "wait for its `lyx webster recover-batch 02` to finish", "re-run `lyx webster reset --to report-head --batch 02`"}},
		{"report-head with no report", websterengine.ResetToReportHead, 2, func(fx *resolutionFixture) {
			fx.state.Batches[2].Slug = "renamed"
		}, []string{"batch 2 has no readable report", "wait for the fork's report, or run `lyx webster recover-batch 02`"}},
		{"report-head not descending from the start", websterengine.ResetToReportHead, 2, func(fx *resolutionFixture) {
			fx.writeReport(2, "two", fx.root)
		}, []string{"does not descend from batch 2's start %batch1Head", "run `lyx webster reset --to start`"}},
		{"report-head missing from the repository", websterengine.ResetToReportHead, 2, func(fx *resolutionFixture) {
			fx.writeReport(2, "two", strings.Repeat("ab", 20))
		}, []string{strings.Repeat("ab", 20), "not in this repository"}},
		{"last-batch-head with no recorded head", websterengine.ResetToLastBatchHead, 0, func(fx *resolutionFixture) {
			fx.state.Batches[1].Digest = nil
		}, []string{"no batch recorded a head commit", "run `lyx webster reset --to start`"}},
		{"last-batch-head of diverging heads", websterengine.ResetToLastBatchHead, 0, func(fx *resolutionFixture) {
			_, unrelatedTip := fx.git.merge("")
			fx.state.Batches[2].Terminal, fx.state.Batches[2].Digest = true, &websterengine.Digest{HeadSHA: unrelatedTip}
		}, []string{"share no single latest commit", "run `lyx webster reset --to start`"}},
		{"batch-start of a batch with no start", websterengine.ResetToBatchStart, 3, nil, []string{"batch 3 recorded no start commit", "run `lyx webster reset --to start`"}},
		{"batch-start with a later recorded start", websterengine.ResetToBatchStart, 1, nil, []string{"batch 1 is not the run's last begun batch", "commits of batch 2", "run `lyx webster reset --to start`"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newResolutionFixture(t)
			if tt.setup != nil {
				tt.setup(fx)
			}
			_, err := websterengine.PlanReset(fx.deps(), tt.to, tt.batch)
			if err == nil {
				t.Fatalf("PlanReset(%s, %d) error = nil, want a refusal", tt.to, tt.batch)
			}
			for _, part := range tt.want {
				part = strings.ReplaceAll(part, "%batch1Head", fx.batch1Head)
				if !strings.Contains(err.Error(), part) {
					t.Errorf("PlanReset(%s, %d) error = %v, want it to contain %q", tt.to, tt.batch, err, part)
				}
			}
		})
	}
}
