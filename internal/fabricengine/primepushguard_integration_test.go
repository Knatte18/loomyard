// primepushguard_integration_test.go covers the guard a one-repository prime's records push runs before it pushes.
// The hub is built by hubforge.NewOneRepoHub, so the prime's branch carries the records and the operator's code.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// guardRunSlug is the run the records commits of the guard fixture name in their Run-Transition trailer.
const guardRunSlug = "guard-run"

// primeGuardFixture is a one-repository hub whose prime holds the commits a row stages.
type primeGuardFixture struct {
	t      *testing.T
	hub    *hubforge.Hub
	prime  string
	branch string
	fake   *fakePushNotifier
}

// newPrimeGuardFixture builds the hub and installs a notifier counting its mails until the test ends.
func newPrimeGuardFixture(t *testing.T) *primeGuardFixture {
	t.Helper()

	h := hubforge.NewOneRepoHub(t, ".")
	fake := &fakePushNotifier{hub: h.Location.HubPath, posted: true}
	t.Cleanup(fabricengine.InstallPushNotifier(fake.notify))
	prime := h.PrimeWorktree()
	return &primeGuardFixture{t: t, hub: h, prime: prime, branch: gitkit.CurrentBranch(t, prime), fake: fake}
}

// commitRecords commits one records file on the prime as a transition of the guard run.
func (f *primeGuardFixture) commitRecords(name string) string {
	f.t.Helper()

	message := "transition " + name + "\n\n" + fabricengine.TransitionTrailerKey + ": " + guardRunSlug
	return gitkit.CommitFile(f.t, f.prime, "_lym/"+name+".txt", name+"\n", message)
}

// commitCode commits one code file on the prime, as the operator does.
func (f *primeGuardFixture) commitCode(name string) string {
	f.t.Helper()

	return gitkit.CommitFile(f.t, f.prime, name+".txt", name+"\n", "code "+name)
}

// originTip returns the tip origin holds of the prime's branch.
func (f *primeGuardFixture) originTip() string {
	f.t.Helper()

	return bareBranchTip(f.t, f.hub.CodeBare, f.branch)
}

// syncOptionsWithoutBackoff returns push options whose retries wait no time.
func syncOptionsWithoutBackoff() fabricengine.SyncOptions {
	return fabricengine.SyncOptions{BackoffSleep: func(time.Duration) {}}
}

// pushPair runs the prime's synchronous pair push.
func (f *primeGuardFixture) pushPair() error {
	f.t.Helper()

	_, err := fabricengine.PushPairAnchored(f.hub.Location, syncOptionsWithoutBackoff(), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
	return err
}

// requireHeld fails the test unless the guard left origin at originBefore and recorded one operator_unpushed record.
// The record names wantCommits and the guard run, and one notice was mailed.
func (f *primeGuardFixture) requireHeld(originBefore string, wantCommits []string) {
	f.t.Helper()

	if got := f.originTip(); got != originBefore {
		f.t.Errorf("origin branch = %s; want it left at %s", got, originBefore)
	}
	record := requireOneFailedPush(f.t, f.hub.Location.HubPath, "records", fabricengine.PushFailureOperatorUnpushed)
	if !slices.Equal(record.Commits, wantCommits) || !slices.Equal(record.Runs, []string{guardRunSlug}) {
		f.t.Errorf("record = commits %q, runs %q; want commits %q, runs %q", record.Commits, record.Runs, wantCommits, []string{guardRunSlug})
	}
	if len(f.fake.lines) != 1 {
		f.t.Errorf("mails = %q; want one", f.fake.lines)
	}
}

// TestPrimePushGuard covers the guard of a one-repository prime's records push.
// Not parallel: each row installs the process's push notifier.
func TestPrimePushGuard(t *testing.T) {
	tiergate.Require(t, 1)

	heldRows := []struct {
		name string
		// stage commits the operator's commit beneath the records commit and returns it.
		stage func(t *testing.T, f *primeGuardFixture) string
		// detached pushes through the detached push's coalescing loop, which returns nil for a held push.
		detached bool
	}{
		{
			name:  "OperatorCommitHoldsThePush",
			stage: func(t *testing.T, f *primeGuardFixture) string { return f.commitCode("operator") },
		},
		{
			name:     "DetachedPushOfThePrimeIsHeldToo",
			stage:    func(t *testing.T, f *primeGuardFixture) string { return f.commitCode("operator") },
			detached: true,
		},
		{
			name: "FastForwardedOperatorCommitHoldsThePush",
			stage: func(t *testing.T, f *primeGuardFixture) string {
				gitkit.Git(t, f.prime, "checkout", "-b", "side")
				side := gitkit.CommitFile(t, f.prime, "side.txt", "side\n", "side commit")
				gitkit.Git(t, f.prime, "push", "origin", "side")
				gitkit.Git(t, f.prime, "checkout", f.branch)
				gitkit.Git(t, f.prime, "merge", "--ff-only", "side")
				return side
			},
		},
		{
			name: "CodeCommitCarryingTheMergeTrailerHoldsThePush",
			stage: func(t *testing.T, f *primeGuardFixture) string {
				return gitkit.CommitFile(t, f.prime, "operator.txt", "operator\n", "code\n\n"+fabricengine.MergeTrailerKey+": side")
			},
		},
	}
	for _, tt := range heldRows {
		t.Run(tt.name, func(t *testing.T) {
			f := newPrimeGuardFixture(t)
			originBefore := f.originTip()
			operator := tt.stage(t, f)
			f.commitRecords("first")

			var err error
			if tt.detached {
				_, err = fabricengine.CoalescePushBothAt(f.prime, f.prime, syncOptionsWithoutBackoff())
				if err != nil {
					t.Fatalf("CoalescePushBothAt() error = %v; want nil", err)
				}
			} else if err = f.pushPair(); !fabricengine.IsPushOwed(err) {
				t.Fatalf("PushPairAnchored() error = %v; want an owed push", err)
			}

			f.requireHeld(originBefore, []string{operator})
		})
	}

	t.Run("RecordsOnlyAndFabricMergeCommitsPush", func(t *testing.T) {
		f := newPrimeGuardFixture(t)
		gitkit.Git(t, f.prime, "checkout", "-b", "side")
		gitkit.CommitFile(t, f.prime, "side.txt", "side\n", "side commit")
		gitkit.Git(t, f.prime, "push", "origin", "side")
		gitkit.Git(t, f.prime, "checkout", f.branch)
		f.commitRecords("first")
		gitkit.Git(t, f.prime, "merge", "--no-ff", "-m", "merge side\n\n"+fabricengine.MergeTrailerKey+": side", "side")
		// A later commit removes the records file the merge was made over, as an unwired prime does.
		gitkit.Git(t, f.prime, "rm", "-q", "_lym/first.txt")
		gitkit.Git(t, f.prime, "commit", "-m", "remove the records file")

		if err := f.pushPair(); err != nil {
			t.Fatalf("PushPairAnchored() error = %v; want nil", err)
		}

		if got, want := f.originTip(), gitkit.RevParse(t, f.prime, "HEAD"); got != want {
			t.Errorf("origin branch = %s; want the local tip %s", got, want)
		}
	})

	t.Run("ProtectedRefusalRecordsProtected", func(t *testing.T) {
		f := newPrimeGuardFixture(t)
		f.commitRecords("first")
		installPreReceive(t, f.hub.CodeBare, protectedRefusingHook(f.branch))

		err := f.pushPair()

		if !fabricengine.IsPushProtected(err) {
			t.Fatalf("PushPairAnchored() error = %v; want a protected push", err)
		}
		requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureProtected)
		operator := f.commitCode("operator")

		if err := f.pushPair(); !fabricengine.IsPushProtected(err) {
			t.Fatalf("PushPairAnchored() over an operator commit error = %v; want a protected push", err)
		}

		record := requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureProtected)
		if !slices.Equal(record.Commits, []string{operator}) {
			t.Errorf("record commits = %q; want %q", record.Commits, []string{operator})
		}
		wayForward := fabricengine.FailedPushWayForward(record)
		for _, want := range []string{"allow direct pushes to " + f.branch, "`git push` in " + f.prime} {
			if !strings.Contains(wayForward, want) {
				t.Errorf("way forward = %q; want it to name %q", wayForward, want)
			}
		}
	})

	t.Run("ProtectedRefusalOfATaskBranchKeepsTodaysClass", func(t *testing.T) {
		f := newPrimeGuardFixture(t)
		added := hubforge.AddPairWith(t, f.hub, "task-pair", fabricengine.AddOptions{})
		location, err := lymcwd.ResolveWorktree(added.Path)
		if err != nil {
			t.Fatalf("ResolveWorktree(%s): %v", added.Path, err)
		}
		gitkit.CommitFile(t, added.Path, "_lym/first.txt", "first\n", "transition first")
		installPreReceive(t, f.hub.CodeBare, protectedRefusingHook(added.Branch))

		_, err = fabricengine.PushPairAnchored(location, syncOptionsWithoutBackoff(), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})

		if err == nil || fabricengine.IsPushProtected(err) {
			t.Fatalf("PushPairAnchored() error = %v; want a failed push that is not protected", err)
		}
		records := failedPushesOf(t, f.hub.Location.HubPath)
		if len(records) != 1 || records[0].Class == fabricengine.PushFailureProtected {
			t.Errorf("failed-push records = %+v; want one record that is not protected", records)
		}
	})

	t.Run("TakeoverEntryPushOfThePrimeIsHeld", func(t *testing.T) {
		f := newPrimeGuardFixture(t)
		added := hubforge.AddPairWith(t, f.hub, takeoverRunSlug, fabricengine.AddOptions{})
		location, err := lymcwd.ResolveWorktree(added.Path)
		if err != nil {
			t.Fatalf("ResolveWorktree(%s): %v", added.Path, err)
		}
		_, statusRel, err := shedrun.TreeRunFileRels(takeoverRunSlug)
		if err != nil {
			t.Fatalf("TreeRunFileRels(%q): %v", takeoverRunSlug, err)
		}
		gitkit.CommitFile(t, added.Path, statusRel, statusJSON("Implementer"), "pair status")
		gitkit.Git(t, added.Path, "push", "origin", added.Branch)
		gitkit.CommitFile(t, f.prime, statusRel, statusJSON("Implementer"), "prime status")
		gitkit.Git(t, f.prime, "push", "origin", f.branch)
		operator := f.commitCode("operator")
		originBefore := f.originTip()

		_, err = fabricengine.TakeOverRun(f.hub.Location, fabricengine.TakeOverRequest{Slug: takeoverRunSlug, Subject: "beam", Machine: "oslo", Pair: location, At: takeoverAt})

		var failed *fabricengine.PushFailedError
		if !errors.As(err, &failed) || failed.Record.Class != fabricengine.PushFailureOperatorUnpushed {
			t.Fatalf("TakeOverRun() error = %v; want the guard's operator_unpushed failure", err)
		}
		requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureOperatorUnpushed)
		if got := f.originTip(); got != originBefore {
			t.Errorf("origin branch = %s; want it left at %s", got, originBefore)
		}
		if got := gitkit.RevParse(t, f.prime, "HEAD"); got != operator {
			t.Errorf("prime HEAD = %s; want %s, the entry commit dropped", got, operator)
		}
		status, err := os.ReadFile(filepath.Join(f.prime, statusRel))
		if err != nil || strings.Contains(string(status), "takeover") {
			t.Errorf("prime status file = %q, %v; want it with no takeover entry", status, err)
		}
	})
}
