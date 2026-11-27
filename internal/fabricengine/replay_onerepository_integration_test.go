// replay_onerepository_integration_test.go covers the replay of a one-repository pair's local commits.
// The replay targets a moved origin.
// Only the records-only commits are rewritten, and a local commit that changes a code path stops the replay.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// originCommit commits rel on the pair's branch from a second clone of origin and pushes it.
// It returns the new origin tip.
func (p oneRepoPair) originCommit(t *testing.T, rel, content string) string {
	t.Helper()

	other := cloneBareForTest(t, p.hub.CodeBare, p.branch)
	sha := gitkit.CommitFile(t, other, rel, content, "origin "+rel)
	gitkit.Git(t, other, "push", "origin", p.branch)
	return sha
}

// messageOf returns the full message of the commit rev in the pair's worktree.
func (p oneRepoPair) messageOf(t *testing.T, rev string) string {
	t.Helper()
	return gitkit.Git(t, p.path, "log", "-1", "--format=%B", rev)
}

// TestReplay_OneRepository covers the replay of a one-repository pair's local commits onto a moved origin.
func TestReplay_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("RecordsOnlyCommitsReplayWithTrailersRewrittenToTheForkPoint", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "trailers")
		fork := gitkit.RevParse(t, p.path, "HEAD")
		first := p.commitTransition(t, "first")
		p.commitTransition(t, "second")
		p.originCommit(t, "_lym/origin.txt", "origin\n")
		gitkit.Git(t, p.path, "fetch")

		outcome, err := fabricengine.ReplayWeftForTest(p.location, p.path, nil)

		if err != nil || len(outcome.Replayed) != 2 {
			t.Fatalf("ReplayWeftForTest() = %+v, %v; want both records-only commits replayed", outcome, err)
		}
		for _, rev := range []string{"HEAD", "HEAD~1"} {
			message := p.messageOf(t, rev)
			if !strings.Contains(message, fabricengine.WarpSHATrailerKey+": "+fork) || strings.Contains(message, first) {
				t.Errorf("message of %s = %q; want its Warp-SHA at the fork point %s and no mention of the replaced %s", rev, message, fork, first)
			}
		}
	})

	t.Run("WarpSHATrailerNamingACommitNotReplayedStays", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "trailer-stays")
		fork := gitkit.RevParse(t, p.path, "HEAD")
		p.commitTransition(t, "first")
		p.commitTransition(t, "second")
		originTip := p.originCommit(t, "_lym/origin.txt", "origin\n")
		gitkit.Git(t, p.path, "fetch")
		gitkit.CommitFile(t, p.path, "_lym/third.txt", "third\n", "transition third\n\n"+fabricengine.WarpSHATrailerKey+": "+originTip)

		outcome, err := fabricengine.ReplayWeftForTest(p.location, p.path, nil)

		if err != nil || len(outcome.Replayed) != 3 {
			t.Fatalf("ReplayWeftForTest() = %+v, %v; want the three records-only commits replayed", outcome, err)
		}
		if message := p.messageOf(t, "HEAD"); !strings.Contains(message, fabricengine.WarpSHATrailerKey+": "+originTip) {
			t.Errorf("message of HEAD = %q; want its Warp-SHA still naming the commit that was not replayed, %s", message, originTip)
		}
		if message := p.messageOf(t, "HEAD~1"); !strings.Contains(message, fabricengine.WarpSHATrailerKey+": "+fork) {
			t.Errorf("message of HEAD~1 = %q; want its Warp-SHA naming a replayed commit rewritten to the fork point %s", message, fork)
		}
	})

	t.Run("CodeCommitAmongOriginsNewCommitsStillReplaysAndNoTrailerNamesIt", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "origin-code")
		fork := gitkit.RevParse(t, p.path, "HEAD")
		p.commitTransition(t, "first")
		originCode := p.originCommit(t, "code.txt", "origin\n")
		gitkit.Git(t, p.path, "fetch")

		outcome, err := fabricengine.ReplayWeftForTest(p.location, p.path, nil)

		if err != nil || len(outcome.Replayed) != 1 {
			t.Fatalf("ReplayWeftForTest() = %+v, %v; want the records-only commit replayed", outcome, err)
		}
		gitkit.Git(t, p.path, "merge-base", "--is-ancestor", originCode, "HEAD")
		message := p.messageOf(t, "HEAD")
		if !strings.Contains(message, fabricengine.WarpSHATrailerKey+": "+fork) || strings.Contains(message, originCode) {
			t.Errorf("message = %q; want its Warp-SHA at %s and none naming origin's code commit %s", message, fork, originCode)
		}
	})

	t.Run("EndedRunCommitsAreSetAsideAmongTheRecordsOnlyCommits", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "ended")
		endedRel := filepath.ToSlash(filepath.Join("_lym", "shed", "gone", "status.json"))
		if want := filepath.ToSlash(shedrun.RunDirRel(p.location, "gone")); !strings.HasPrefix(endedRel, want) {
			t.Fatalf("test path %s is not under the run directory %s", endedRel, want)
		}
		endedCommit := gitkit.CommitFile(t, p.path, endedRel, "{}\n", "gone status")
		keep := gitkit.CommitFile(t, p.path, "_lym/keep.txt", "keep\n", "keep")
		p.originCommit(t, "_lym/origin.txt", "origin\n")
		gitkit.Git(t, p.path, "fetch")

		outcome, err := fabricengine.ReplayWeftForTest(p.location, p.path, map[string]string{"gone": "done"})

		if err != nil || len(outcome.SetAside) != 1 || outcome.SetAside[0].Commit != endedCommit || len(outcome.Replayed) != 1 || outcome.Replayed[0] != keep {
			t.Fatalf("ReplayWeftForTest() = %+v, %v; want %s set aside and %s replayed", outcome, err, endedCommit, keep)
		}
	})

	t.Run("LocalCodeCommitRewritesNothingAndStopsNamingIt", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "code-stop")
		p.commitTransition(t, "first")
		localCode := gitkit.CommitFile(t, p.path, "code.txt", "local\n", "local code")
		p.originCommit(t, "_lym/origin.txt", "origin\n")
		before := gitkit.RevParse(t, p.path, "HEAD")

		_, err := openOneRepositoryFabric(t, p.path).Pull(fabricengine.SyncOptions{})

		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) {
			t.Fatalf("Pull() error = %v; want a *WeftStopError", err)
		}
		for _, want := range []string{localCode, "git merge origin/" + p.branch, "lym fabric push"} {
			if !strings.Contains(stop.WayForward, want) {
				t.Errorf("way forward = %q; want it to name %q", stop.WayForward, want)
			}
		}
		if got := gitkit.RevParse(t, p.path, "HEAD"); got != before {
			t.Errorf("HEAD = %s; want the branch unmoved at %s", got, before)
		}
	})

	t.Run("RejectedPushWithALocalCodeCommitIsOwedAndNotified", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "code-owed")
		localCode := gitkit.CommitFile(t, p.path, "code.txt", "local\n", "local code")
		p.originCommit(t, "_lym/origin.txt", "origin\n")
		hubPath := p.hub.Location.HubPath
		fake := &fakePushNotifier{hub: hubPath, posted: true}

		_, err := fabricengine.PushPairAnchored(p.location, fake.options(func(time.Duration) {}), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})

		if !fabricengine.IsPushOwed(err) {
			t.Fatalf("PushPairAnchored() error = %v; want an owed push", err)
		}
		record := requireOneFailedPush(t, hubPath, "records", fabricengine.PushFailureDeferred)
		if len(record.Commits) != 1 || record.Commits[0] != localCode {
			t.Errorf("record commits = %v; want [%s]", record.Commits, localCode)
		}
		if len(fake.lines) != 1 || !strings.Contains(fake.lines[0], localCode) {
			t.Errorf("notices = %q; want one naming %s", fake.lines, localCode)
		}
	})

	t.Run("RejectedPushReadsTheTakeoverFirst", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "code-taken")
		entry := shedengine.Takeover{Machine: "oslo", Subject: "sub", At: "2026-11-16T10:00:00Z"}
		p.plantTakeover(t, entry)
		gitkit.CommitFile(t, p.path, "code.txt", "local\n", "local code")

		_, err := p.push(fabricengine.SyncOptions{}, fabricengine.MachineNames{Current: "neve"})

		var takeover *fabricengine.TakeoverError
		if !errors.As(err, &takeover) || takeover.Entry != entry {
			t.Errorf("push error = %v; want a *TakeoverError naming %+v", err, entry)
		}
		requireNoFailedPush(t, p.hub.Location.HubPath)
	})

	t.Run("CodeCommitCarryingTheMergeTrailerIsNotReplayed", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "merge-trailer")
		merged := gitkit.CommitFile(t, p.path, "code.txt", "local\n", "merge of a task\n\n"+fabricengine.MergeTrailerKey+": task")
		p.originCommit(t, "_lym/origin.txt", "origin\n")
		gitkit.Git(t, p.path, "fetch")
		before := gitkit.RevParse(t, p.path, "HEAD")

		_, err := fabricengine.ReplayWeftForTest(p.location, p.path, nil)

		if err == nil || errors.Is(err, fabricengine.ErrReplayConflict) || errors.Is(err, fabricengine.ErrReplayDirty) || !strings.Contains(err.Error(), merged) {
			t.Errorf("ReplayWeftForTest() error = %v; want a stop naming %s", err, merged)
		}
		if got := gitkit.RevParse(t, p.path, "HEAD"); got != before {
			t.Errorf("HEAD = %s; want the branch unmoved at %s", got, before)
		}
	})

	t.Run("FastForwardPairLevelsWithOriginKeepingRecordsCommits", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "fast-forward")
		p.commitTransition(t, "local")
		originTip := p.originCommit(t, "code.txt", "origin\n")

		res, err := openOneRepositoryFabric(t, p.path).FastForwardPair(fabricengine.SyncOptions{})

		if err != nil || res.Refusal != "" {
			t.Fatalf("FastForwardPair() = %+v, %v; want no refusal", res, err)
		}
		gitkit.Git(t, p.path, "merge-base", "--is-ancestor", originTip, "HEAD")
		if got := gitkit.Git(t, p.path, "show", "HEAD:_lym/local.txt"); got != "local" {
			t.Errorf("_lym/local.txt at HEAD = %q; want the local records commit kept", got)
		}
		if got := gitkit.RevParse(t, p.path, "HEAD"); res.NewHead != got {
			t.Errorf("NewHead = %s; want HEAD %s", res.NewHead, got)
		}
	})

	t.Run("FastForwardPairRefusesOverALocalCodeCommit", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "fast-forward-code")
		gitkit.CommitFile(t, p.path, "code.txt", "local\n", "local code")
		p.originCommit(t, "_lym/origin.txt", "origin\n")
		before := gitkit.RevParse(t, p.path, "HEAD")

		res, err := openOneRepositoryFabric(t, p.path).FastForwardPair(fabricengine.SyncOptions{})

		if err != nil || res.Refusal != fabricengine.FastForwardNotFastForward {
			t.Errorf("FastForwardPair() = %+v, %v; want the not-fast-forward refusal", res, err)
		}
		if got := gitkit.RevParse(t, p.path, "HEAD"); got != before {
			t.Errorf("HEAD = %s; want it unmoved at %s", got, before)
		}
	})

	t.Run("MergeIntoAParentBehindOriginWithRecordsCommitsSyncs", func(t *testing.T) {
		t.Parallel()
		h := hubforge.NewOneRepoHub(t, ".")
		prime := h.PrimeWorktree()
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		gitkit.CommitFile(t, task.Path, "task-code.txt", "task\n", "task code")
		gitkit.CommitFile(t, prime, "_lym/parent.txt", "parent\n", "parent records")
		primeBranch := gitkit.CurrentBranch(t, prime)
		other := cloneBareForTest(t, h.CodeBare, primeBranch)
		originTip := gitkit.CommitFile(t, other, "origin.txt", "origin\n", "origin code")
		gitkit.Git(t, other, "push", "origin", primeBranch)

		res, err := hubforge.OpenFabric(t, h).Merge(task.Branch, fabricengine.MergeOptions{})

		if err != nil || !res.Committed {
			t.Fatalf("Merge() = (committed %v, %v); want a committed merge", res.Committed, err)
		}
		gitkit.Git(t, prime, "merge-base", "--is-ancestor", originTip, "HEAD")
		for rel, want := range map[string]string{"_lym/parent.txt": "parent", "task-code.txt": "task", "origin.txt": "origin"} {
			if got := gitkit.Git(t, prime, "show", "HEAD:"+rel); got != want {
				t.Errorf("%s at HEAD = %q; want %q", rel, got, want)
			}
		}
	})
}
