// remoteteardown_rerun_integration_test.go covers the re-run of a teardown whose detached child ended before its job did.
// Each test builds a one-repository hub with hubforge.NewOneRepoHub.
// It drops a first run's remote half through the Topology seam.
// It then reads origin with git for-each-ref for the archive tag of a tip that only the first run's local tag held.
// It shares the fixture helpers of the one-repository lifecycle tests and the TestMain in testmain_test.go.

package fabricengine_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requireOnePendingTeardown fails t unless the hub lists one pending teardown, of slug, with the way forward given.
func requireOnePendingTeardown(t *testing.T, h *hubforge.Hub, slug, wantWayForward string) {
	t.Helper()

	pending, err := fabricengine.PendingTeardowns(h.Path)
	if err != nil {
		t.Fatalf("PendingTeardowns() error = %v", err)
	}
	if len(pending) != 1 || pending[0].Slug != slug || pending[0].WayForward != wantWayForward {
		t.Errorf("PendingTeardowns() = %+v; want one record of %s with the way forward %q", pending, slug, wantWayForward)
	}
}

// TestRemove_RerunAfterItsTeardownChildEndedArchivesTheLocalRecordsTip covers a removal's re-run.
// The first run's child ended before its first push.
// The pair held a records commit origin lacks, which the removal tagged locally before it deleted the branch.
// The pending record names the hand push of that tag before the re-run.
// The re-run pushes the tag before it deletes the records branch on origin.
func TestRemove_RerunAfterItsTeardownChildEndedArchivesTheLocalRecordsTip(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "ended-remove"
	h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
	bare := h.CodeBare
	recordsBranch := fabriclayout.RecordsBranchName(hubforge.AddPair(t, h, slug).Branch)
	gitkit.CommitFile(t, h.PairRecordsSibling(slug), "unpushed.txt", "unpushed\n", "unpushed records")

	fabricengine.SetTeardownChildEndsForTest(h.Topology, true)
	ended, err := h.Topology.Remove(h.Location, slug, false, true, "")
	if err != nil {
		t.Fatalf("Remove(%s) with its teardown child ended: %v", slug, err)
	}
	localTag := ended.ArchiveTag
	localTip := gitkit.RevParse(t, h.PrimeRecords(), "refs/tags/"+localTag)
	if got := oneRepoRecordsTip(t, bare, recordsBranch); got == localTip {
		t.Fatalf("origin's %s is at the local records tip %s; the fixture needs a tip origin lacks", recordsBranch, localTip)
	}
	requireOnePendingTeardown(t, h, slug, "push the archive tag with `git -C "+h.PrimeRecords()+" push origin refs/tags/"+localTag+":"+namespaceTagsPrefix+localTag+
		"`, then when origin still holds a branch of the pair, run `lym fabric remove "+slug+" --remote` again")

	fabricengine.SetTeardownChildEndsForTest(h.Topology, false)
	if _, err := h.Topology.Remove(h.Location, slug, false, true, ""); err != nil {
		t.Fatalf("re-run Remove(%s): %v", slug, err)
	}

	if got := refLines(t, bare, "%(objectname)", namespaceTagsPrefix+localTag); !slices.Equal(got, []string{localTip}) {
		t.Errorf("origin's archive tag of the local records tip = %v; want one at %s", got, localTip)
	}
	if got := refLines(t, bare, "%(refname)", namespaceHeadsPrefix+recordsBranch); len(got) != 0 {
		t.Errorf("origin still holds %v after the re-run; want the records branch deleted", got)
	}
}

// TestRemove_PendingRecordNamesTheRerunItsCallerGave covers a removal whose caller names the command that retries it.
// The first run's child ended before its first push, with a records commit origin lacks tagged locally.
// The pending record names the hand push of that tag and then the caller's command, with no condition on origin.
// Settling the teardown keeps the record while origin holds the pair's branches.
// Once origin holds none, it pushes the local tag and drops the record.
func TestRemove_PendingRecordNamesTheRerunItsCallerGave(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "cancelled-remove"
	const rerun = "lym batten cancel " + slug + " --remove"
	h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
	bare := h.CodeBare
	branch := hubforge.AddPair(t, h, slug).Branch
	gitkit.CommitFile(t, h.PairRecordsSibling(slug), "unpushed.txt", "unpushed\n", "unpushed records")

	fabricengine.SetTeardownChildEndsForTest(h.Topology, true)
	ended, err := h.Topology.Remove(h.Location, slug, false, true, rerun)
	if err != nil {
		t.Fatalf("Remove(%s) with its teardown child ended: %v", slug, err)
	}
	localTip := gitkit.RevParse(t, h.PrimeRecords(), "refs/tags/"+ended.ArchiveTag)
	wayForward := "push the archive tag with `git -C " + h.PrimeRecords() + " push origin refs/tags/" + ended.ArchiveTag + ":" + namespaceTagsPrefix + ended.ArchiveTag +
		"`, then run `" + rerun + "` again"

	requireOnePendingTeardown(t, h, slug, wayForward)

	if err := h.Topology.SettleGonePairTeardown(h.Location, slug, rerun); err != nil {
		t.Fatalf("SettleGonePairTeardown(%s) with the pair on origin: %v", slug, err)
	}
	requireOnePendingTeardown(t, h, slug, wayForward)

	for _, ref := range []string{namespaceHeadsPrefix + fabriclayout.RecordsBranchName(branch), "refs/heads/" + branch} {
		gitkit.MustRun(t, bare, "git", "update-ref", "-d", ref)
	}
	if err := h.Topology.SettleGonePairTeardown(h.Location, slug, rerun); err != nil {
		t.Fatalf("SettleGonePairTeardown(%s) with the pair gone: %v", slug, err)
	}

	if got := refLines(t, bare, "%(objectname)", namespaceTagsPrefix+ended.ArchiveTag); !slices.Equal(got, []string{localTip}) {
		t.Errorf("origin's archive tag of the local records tip = %v; want one at %s", got, localTip)
	}
	if pending, err := fabricengine.PendingTeardowns(h.Path); err != nil || len(pending) != 0 {
		t.Errorf("PendingTeardowns() after settling a gone pair = (%+v, %v); want none", pending, err)
	}
}

// TestDiscardTaskBranch_RerunAfterItsTeardownChildEndedArchivesTheTaskTip covers a discard's re-run.
// The first run's child ended before its first push.
// The task branch held a commit origin lacks, which the discard tagged locally before it deleted the branch.
// The pending record names the hand push of that tag before the re-run of the command the discard was given.
// The re-run is the removal and then the discard, as the command that discards a task branch makes them.
// It deletes the task branch on origin only with the tag of the local tip on origin.
func TestDiscardTaskBranch_RerunAfterItsTeardownChildEndedArchivesTheTaskTip(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "ended-discard"
	const rerun = "lym batten cancel " + slug + " --remove"
	h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
	bare := h.CodeBare
	branch := hubforge.AddPair(t, h, slug).Branch
	gitkit.CommitFile(t, h.PairCodeWorktree(slug), "pushed.txt", "pushed\n", "unlanded work origin holds")
	if _, err := oneRepoPairFabric(t, h, slug).PushBranch(fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("PushBranch: %v", err)
	}
	localTip := gitkit.CommitFile(t, h.PairCodeWorktree(slug), "unpushed.txt", "unpushed\n", "unlanded work origin lacks")
	localTag := fabriclayout.ArchiveTagName(slug, localTip)
	// The removal keeps the unlanded task branch, locally and on origin, for the discard that follows it.
	if _, err := h.Topology.Remove(h.Location, slug, false, true, ""); err != nil {
		t.Fatalf("Remove(%s): %v", slug, err)
	}

	fabricengine.SetTeardownChildEndsForTest(h.Topology, true)
	if _, err := h.Topology.DiscardTaskBranch(h.Location, slug, rerun); err != nil {
		t.Fatalf("DiscardTaskBranch(%s) with its teardown child ended: %v", slug, err)
	}
	requireOnePendingTeardown(t, h, slug, "push the archive tag with `git -C "+h.PrimeWorktree()+" push origin refs/tags/"+localTag+":"+namespaceCodeTagsPrefix+localTag+
		"`, then run `"+rerun+"` again")

	fabricengine.SetTeardownChildEndsForTest(h.Topology, false)
	if _, err := h.Topology.Remove(h.Location, slug, false, true, ""); err != nil {
		t.Fatalf("re-run Remove(%s): %v", slug, err)
	}
	if _, err := h.Topology.DiscardTaskBranch(h.Location, slug, rerun); err != nil {
		t.Fatalf("re-run DiscardTaskBranch(%s): %v", slug, err)
	}

	if got := refLines(t, bare, "%(objectname)", namespaceCodeTagsPrefix+localTag); !slices.Equal(got, []string{localTip}) {
		t.Errorf("origin's archive tag of the local task tip = %v; want one at %s", got, localTip)
	}
	if gitkit.BranchExists(t, bare, branch) {
		t.Errorf("task branch %q still on origin after the re-run; want it deleted under its archive tag", branch)
	}
}
