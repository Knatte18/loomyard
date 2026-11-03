// readopt_integration_test.go drives a batten resume's re-adoption of a gone task worktree over a real hub with a pushed pair.

package battencli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// failedStopRecorder counts the notices and failed-stop commits a receiver's seams were asked for.
type failedStopRecorder struct {
	notices int
	commits int
}

// armFailedStop points c at the batten status of slug under h, writes status there, and swaps the notice and failed-stop commit seams for recorders.
func armFailedStop(t *testing.T, c *battenCLI, h *hubforge.Hub, slug string, status shedengine.Status) *failedStopRecorder {
	t.Helper()

	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     StatusFile(h.Location, slug),
		LockPath:       RunLock(h.Location, slug),
		StatusLockPath: StatusLock(h.Location, slug),
	}
	for _, path := range []string{c.shedPaths.StatusPath, c.shedPaths.StatusLockPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, status); err != nil {
		t.Fatalf("write the batten status: %v", err)
	}
	recorder := &failedStopRecorder{}
	c.readopt.Notice = func(string, string) error { recorder.notices++; return nil }
	c.readopt.CommitFailed = func(string, string) error { recorder.commits++; return nil }
	return recorder
}

// requireStatusState asserts the batten status of c reads want.
func requireStatusState(t *testing.T, c *battenCLI, want shedengine.State) {
	t.Helper()

	status, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil || !found || status.State != want {
		t.Errorf("batten status = %+v, found %v, %v; want state %s", status, found, err, want)
	}
}

// readoptReceiver builds a receiver over h whose re-adoption seams are the hub's, and returns it with the report its resume reconcile judged last.
func readoptReceiver(h *hubforge.Hub, slug string) (*battenCLI, *fabricengine.OriginReport) {
	c := &battenCLI{location: h.Location, slug: slug}
	c.readopt = hubReadoptSeams(h.Location)
	var second fabricengine.OriginReport
	c.reconcile = func(slug string) (fabricengine.OriginReport, error) {
		report, err := reconcileRun(h.Location, slug, false)
		second = report
		return report, err
	}
	return c, &second
}

// removePairDirectories deletes both worktree directories of slug's pair, leaving the registrations and branches behind.
func removePairDirectories(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()
	for _, dir := range []string{h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)} {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("delete %s: %v", dir, err)
		}
	}
}

// TestBattenIntegration_GoneTaskWorktree drives ownerPreflight over a hub whose pair lost both worktree directories.
// A pair whose branches are live is re-made, and the resume reconcile reads none of its branches stopped.
// A local branch origin lacks has its stale tracking ref dropped, so the pair's push creates it on origin again.
// taskWorktreeGone reads a plain file and an empty directory at the task path as gone, and a resolving worktree as present.
func TestBattenIntegration_GoneTaskWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	runShed := shedengine.Status{CurrentProducer: battenrecipe.NameRunShed, State: shedengine.StateRunning}

	tests := []struct {
		name string
		// lackRecords and lackTask delete that side's branch from origin before the pair's directories go.
		lackRecords, lackTask bool
	}{
		{name: "BranchesLiveOnOrigin"},
		{name: "RecordsBranchOriginLacks", lackRecords: true},
		{name: "TaskBranchOriginLacks", lackTask: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			const slug = "gone-pair"
			res := hubforge.AddPair(t, h, slug)
			taskBranch := res.Branch
			recordsBranch := fabricengine.RecordsBranchName(taskBranch)
			recordsRoot, err := fabricengine.RecordsRepoRoot(h.Location)
			if err != nil {
				t.Fatalf("RecordsRepoRoot: %v", err)
			}
			if tt.lackRecords {
				gitkit.Git(t, h.RecordsBare, "branch", "-D", recordsBranch)
			}
			if tt.lackTask {
				gitkit.Git(t, h.CodeBare, "branch", "-D", taskBranch)
			}
			removePairDirectories(t, h, slug)
			c, second := readoptReceiver(h, slug)

			if err := c.ownerPreflight(true, runShed); err != nil {
				t.Fatalf("ownerPreflight() = %v; want nil", err)
			}

			gone, err := taskWorktreeGone(h.Location, slug)
			if err != nil || gone {
				t.Errorf("taskWorktreeGone() = %v, %v after the re-adoption; want false, nil", gone, err)
			}
			for _, entry := range second.Entries {
				if entry.Slug == slug && entry.State == fabricengine.EntryStopped {
					t.Errorf("resume reconcile entry = %+v; want the re-made pair not stopped", entry)
				}
			}
			for _, branch := range second.Branches {
				if branch.Slug == slug && branch.State == fabricengine.BranchGone {
					t.Errorf("resume reconcile branch = %+v; want no branch of the re-made pair gone", branch)
				}
			}
			if !gitkit.BranchExists(t, h.RecordsBare, recordsBranch) || !gitkit.BranchExists(t, h.CodeBare, taskBranch) {
				t.Errorf("origin holds the records branch %v and the task branch %v after the pair's push; want both", gitkit.BranchExists(t, h.RecordsBare, recordsBranch), gitkit.BranchExists(t, h.CodeBare, taskBranch))
			}
			if tt.lackRecords {
				if _, err := gitexec.Run([]string{"config", "--get", "branch." + recordsBranch + ".remote"}, recordsRoot); err != nil {
					t.Errorf("the records branch has no upstream after the push: %v; want the push to set it again", err)
				}
			}
		})
	}

	t.Run("ARemovedPairBeforeLandFailsTheRunNamingTheCancel", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		const slug = "removed-pair"
		hubforge.AddPair(t, h, slug)
		if _, err := h.Topology.Remove(h.Location, slug, false, true); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		c, _ := readoptReceiver(h, slug)
		recorder := armFailedStop(t, c, h, slug, runShed)

		err := c.ownerPreflight(true, runShed)

		if err == nil || !strings.Contains(err.Error(), "lym batten cancel "+slug+" --remove") {
			t.Fatalf("ownerPreflight() = %v; want a refusal naming the cancel", err)
		}
		requireStatusState(t, c, shedengine.StateFailed)
		if recorder.notices != 1 || recorder.commits != 1 {
			t.Errorf("notices = %d and failed-stop commits = %d; want 1 and 1", recorder.notices, recorder.commits)
		}
	})

	t.Run("ALandedPairIsRestoredFromItsArchiveTagAndReadopted", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		const slug = "landed-pair"
		res := hubforge.AddPair(t, h, slug)
		taskBranch := res.Branch
		recordsBranch := fabricengine.RecordsBranchName(taskBranch)
		recordsRoot, err := fabricengine.RecordsRepoRoot(h.Location)
		if err != nil {
			t.Fatalf("RecordsRepoRoot: %v", err)
		}
		prime := h.PrimeWorktree()

		task, err := taskWorktreeLocation(h.Location, slug)
		if err != nil {
			t.Fatalf("taskWorktreeLocation: %v", err)
		}
		childStatus, childLock := shedrun.StatusFile(task, shedrun.SelfRunID), shedrun.StatusLock(task, shedrun.SelfRunID)
		for _, path := range []string{childStatus, childLock} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := state.WriteJSON(childStatus, childLock, shedengine.Status{State: shedengine.StateDone}); err != nil {
			t.Fatalf("write the child status: %v", err)
		}
		if _, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), task, []string{shedrun.StatusRel(task, shedrun.SelfRunID)}, "test: child run done", fabricengine.EnvSyncOptions()); err != nil {
			t.Fatalf("commit the child status: %v", err)
		}
		gitkit.CommitFile(t, h.PairCodeWorktree(slug), "work.txt", "work\n", "task work")
		gitkit.Git(t, h.PairCodeWorktree(slug), "push", "--quiet", "origin", taskBranch)
		parentBranch := gitkit.CurrentBranch(t, prime)
		gitkit.Git(t, prime, "merge", "--squash", taskBranch)
		gitkit.Git(t, prime, "commit", "-m", "land "+slug)
		gitkit.Git(t, prime, "push", "--quiet", "origin", "HEAD")
		if _, err := h.Topology.Remove(h.Location, slug, false, true); err != nil {
			t.Fatalf("Remove: %v", err)
		}

		atLand := shedengine.Status{CurrentProducer: battenrecipe.NameLand, State: shedengine.StateRunning}
		c, _ := readoptReceiver(h, slug)
		recorder := armFailedStop(t, c, h, slug, atLand)

		err = c.ownerPreflight(true, atLand)

		if err == nil {
			t.Fatal("ownerPreflight() = nil; want the failed stop of a pair that is not live")
		}
		requireStatusState(t, c, shedengine.StateFailed)
		if recorder.notices != 1 || recorder.commits != 1 {
			t.Errorf("notices = %d and failed-stop commits = %d; want 1 and 1", recorder.notices, recorder.commits)
		}
		tags := strings.Fields(gitkit.Git(t, recordsRoot, "tag", "--list", fabricengine.ArchiveTagPrefix+slug+"/*"))
		if len(tags) != 1 {
			t.Fatalf("archive tags = %q; want the one tag the removal pushed", tags)
		}
		for _, want := range []string{"branch " + recordsBranch + " " + tags[0], "branch " + taskBranch + " " + parentBranch, "lym batten run " + slug} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ownerPreflight() = %q; want it to carry %q", err.Error(), want)
			}
		}
		if strings.Contains(err.Error(), "fetch origin tag") {
			gitkit.Git(t, recordsRoot, "fetch", "origin", "tag", tags[0])
		}
		gitkit.Git(t, recordsRoot, "branch", recordsBranch, tags[0])
		gitkit.Git(t, prime, "branch", taskBranch, parentBranch)

		failed := shedengine.Status{CurrentProducer: battenrecipe.NameLand, State: shedengine.StateFailed}
		if err := c.ownerPreflight(true, failed); err != nil {
			t.Fatalf("ownerPreflight() after the restore = %v; want the pair re-adopted", err)
		}

		requireStatusState(t, c, shedengine.StateFailed)
		readopted, err := taskWorktreeLocation(h.Location, slug)
		if err != nil {
			t.Fatalf("the re-adopted task worktree does not resolve: %v", err)
		}
		if _, err := os.Stat(shedrun.StatusFile(readopted, shedrun.SelfRunID)); err != nil {
			t.Errorf("the child's status is missing from the re-adopted pair: %v", err)
		}
	})

	t.Run("TaskWorktreeGoneReadsStrayPathsAsGone", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, "present-pair")
		for name, setup := range map[string]func(path string) error{
			"plain-file":      func(path string) error { return os.WriteFile(path, []byte("stray"), 0o644) },
			"empty-directory": func(path string) error { return os.MkdirAll(path, 0o755) },
		} {
			if err := setup(fabricengine.WorktreePath(h.Location, name)); err != nil {
				t.Fatalf("plant %s: %v", name, err)
			}
			gone, err := taskWorktreeGone(h.Location, name)
			if err != nil || !gone {
				t.Errorf("taskWorktreeGone(%q) = %v, %v; want true, nil", name, gone, err)
			}
		}
		gone, err := taskWorktreeGone(h.Location, "present-pair")
		if err != nil || gone {
			t.Errorf("taskWorktreeGone(\"present-pair\") = %v, %v; want false, nil", gone, err)
		}
	})
}
