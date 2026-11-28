// pushclasses_integration_test.go proves that every push a fabric runs classifies a failure.
// It recovers where it safely can; otherwise it writes the failed-push record and its one notice.
// The other machine is a second clone of the hub's bare repositories,
// and a refusing origin is a pre-receive hook or an unreachable URL.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// refusedOriginURL is an origin URL git refuses with `connection refused`, a phrase gitexec.IsTransportFailure matches.
const refusedOriginURL = "http://127.0.0.1:1/x"

// authRefusingHook is a pre-receive hook that declines every push the way GitHub declines one for want of permission.
const authRefusingHook = "#!/bin/sh\necho 'Permission to hub denied to tester.' >&2\nexit 1\n"

// protectedRefusingHook returns a pre-receive hook that declines every push to branch.
// It declines the way GitHub declines a push to a protected branch.
func protectedRefusingHook(branch string) string {
	return "#!/bin/sh\necho 'GH006: Protected branch update failed for refs/heads/" + branch + ".' >&2\nexit 1\n"
}

// newFailureFixture builds a hub and a notifier counting the mails of its own hub.
// A push reaches the notifier only through the options the notifier builds.
func newFailureFixture(t *testing.T) (*recordsFixture, *fakePushNotifier) {
	t.Helper()
	return newFailureFixtureAt(t, ".")
}

// newFailureFixtureAt builds the hub of newFailureFixture with the worktree-relative anchor anchor.
func newFailureFixtureAt(t *testing.T, anchor string) (*recordsFixture, *fakePushNotifier) {
	t.Helper()
	f := &recordsFixture{t: t, hub: hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})}
	return f, &fakePushNotifier{hub: f.hub.Location.HubPath, posted: true}
}

// failedPushesOf returns the hub's failed-push records.
func failedPushesOf(t *testing.T, hubPath string) []fabriclayout.FailedPush {
	t.Helper()
	records, err := fabricengine.ListFailedPushes(hubPath)
	if err != nil {
		t.Fatalf("ListFailedPushes(%s): %v", hubPath, err)
	}
	return records
}

// requireOneFailedPush fails the test unless the hub holds exactly one failed-push record, and returns it.
func requireOneFailedPush(t *testing.T, hubPath, repo string, class fabriclayout.PushFailureClass) fabriclayout.FailedPush {
	t.Helper()
	records := failedPushesOf(t, hubPath)
	if len(records) != 1 || records[0].Repo != repo || records[0].Class != class {
		t.Fatalf("failed-push records = %+v; want one %s record of class %s", records, repo, class)
	}
	return records[0]
}

// requireNoFailedPush fails the test when the hub holds a failed-push record.
func requireNoFailedPush(t *testing.T, hubPath string) {
	t.Helper()
	if records := failedPushesOf(t, hubPath); len(records) != 0 {
		t.Errorf("failed-push records = %+v; want none", records)
	}
}

// bareBranchTip returns the commit bare holds for branch.
func bareBranchTip(t *testing.T, bare, branch string) string {
	t.Helper()
	return gitkit.RevParse(t, bare, "refs/heads/"+branch)
}

// countingBackoff returns a BackoffSleep that counts its calls, and runs onSleep first when it is set.
func countingBackoff(sleeps *int, onSleep func(call int)) func(time.Duration) {
	return func(time.Duration) {
		*sleeps++
		if onSleep != nil {
			onSleep(*sleeps)
		}
	}
}

// holdWeftWriteLock takes the weft write lock of the weft worktree at weft until the test ends.
func holdWeftWriteLock(t *testing.T, weft string) *lock.FileLock {
	t.Helper()
	lockDir := fabriclayout.RecordsLockDirPath(weft)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", lockDir, err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(lockDir, "weft.write.lock"))
	if err != nil {
		t.Fatalf("take the weft write lock: %v", err)
	}
	return held
}

// pairFixture is a hub with a pair whose two sides have been pushed once, so each has an upstream.
type pairFixture struct {
	*recordsFixture
	fake       *fakePushNotifier
	location   *lymcwd.Location
	warpPath   string
	weftPath   string
	warpBranch string
	weftBranch string
}

func newPairFixture(t *testing.T, slug string) *pairFixture {
	t.Helper()
	return newPairFixtureAt(t, ".", slug)
}

// newPairFixtureAt builds the pair slug of newPairFixture in a hub with the worktree-relative anchor anchor.
func newPairFixtureAt(t *testing.T, anchor, slug string) *pairFixture {
	t.Helper()
	f, fake := newFailureFixtureAt(t, anchor)
	added := hubforge.AddPair(t, f.hub, slug)
	location, err := lymcwd.ResolveWorktree(f.hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	p := &pairFixture{
		recordsFixture: f,
		fake:           fake,
		location:       location,
		warpPath:       f.hub.PairCodeWorktree(slug),
		weftPath:       f.hub.PairRecordsSibling(slug),
		warpBranch:     added.Branch,
		weftBranch:     fabriclayout.RecordsBranchName(added.Branch),
	}
	if _, err := fabricengine.PushPairAnchored(location, fake.options(nil), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded}); err != nil {
		t.Fatalf("PushPairAnchored() priming push error = %v; want nil", err)
	}
	return p
}

// boardBranch returns the branch the hub's board is checked out on.
func (f *recordsFixture) boardBranch() string {
	f.t.Helper()
	return gitkit.CurrentBranch(f.t, f.hub.BoardDir())
}

// endEntryOnOrigin marks the board entry slug done on origin's Bolt branch, from the other machine.
func (f *recordsFixture) endEntryOnOrigin(slug string) {
	f.t.Helper()
	f.originCommit(f.boardBranch(), "board.json", boardFileOf(f.t, boardEntry(slug, withBoardStatus("done"))), slug+" done")
}

// requireRewoundHoldBack fails the test unless err holds back the push of p's code branch, which origin rewound.
// err must be a rewound *PushFailedError naming originHead and the dropped commit, and end in its way forward;
// origin's tip stays at originHead, and the hub holds one rewound record and one mail.
func requireRewoundHoldBack(t *testing.T, p *pairFixture, err error, originHead, dropped string) {
	t.Helper()

	var failed *fabricengine.PushFailedError
	if !errors.As(err, &failed) || failed.Record.Class != fabriclayout.PushFailureRewound {
		t.Fatalf("PushBranch() error = %v; want a rewound *PushFailedError", err)
	}
	if failed.Record.OriginHead != originHead || !slices.Contains(failed.Record.Commits, dropped) {
		t.Errorf("record = %+v; want origin's head %s and the dropped commit %s", failed.Record, originHead, dropped)
	}
	if !strings.HasSuffix(err.Error(), "way forward: "+fabricengine.FailedPushWayForward(failed.Record)) {
		t.Errorf("PushBranch() error = %q; want it to end in the rewound way forward", err)
	}
	if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != originHead {
		t.Errorf("origin tip = %s; want it left where the rewind put it, %s", got, originHead)
	}
	requireOneFailedPush(t, p.hub.Location.HubPath, "code", fabriclayout.PushFailureRewound)
	if len(p.fake.lines) != 1 {
		t.Errorf("mails = %q; want one", p.fake.lines)
	}
}

// TestPushFailureClasses is the enforcing test of the push classification:
// each row drives one push path against a refusing or moved origin.
// A row asserts what the push leaves in the failed-push records, the mail or origin.
func TestPushFailureClasses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("AWeftPushDefersUnderTheWriteLockAndTheNextCommitsPushLandsIt", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft, branch := f.primeWeft()
		f.originCommit(branch, "other.txt", "other\n", "other machine")
		gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
		before := bareBranchTip(t, f.hub.RecordsBare, branch)
		held := holdWeftWriteLock(t, weft)

		if _, err := fabricengine.CoalescePushRequest(fabriclayout.PushSpawn{WeftPath: weft}, fake.options(nil), fabriclayout.MachineNames{}); err != nil {
			t.Fatalf("CoalescePushRequest() with the write lock held error = %v; want nil", err)
		}

		if got := bareBranchTip(t, f.hub.RecordsBare, branch); got != before {
			t.Errorf("origin tip = %s; want it unchanged at %s", got, before)
		}
		requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabriclayout.PushFailureDeferred)
		if len(fake.lines) != 0 {
			t.Errorf("mails = %q; want none for a deferral", fake.lines)
		}
		if _, err := os.Stat(filepath.Join(fabriclayout.RecordsLockDirPath(weft), "pending.records")); !os.IsNotExist(err) {
			t.Errorf("pending flag stat error = %v; want the flag absent, since a deferral leaves the commits to the next push", err)
		}

		if err := held.Release(); err != nil {
			t.Fatalf("release the weft write lock: %v", err)
		}
		gitkit.CommitFile(t, weft, "second.txt", "second\n", "second local")
		if _, err := fabricengine.CoalescePushRequest(fabriclayout.PushSpawn{WeftPath: weft}, fake.options(nil), fabriclayout.MachineNames{}); err != nil {
			t.Fatalf("CoalescePushRequest() after the release error = %v; want nil", err)
		}
		if got, want := bareBranchTip(t, f.hub.RecordsBare, branch), gitkit.RevParse(t, weft, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the local HEAD %s", got, want)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})

	t.Run("ARejectionThatOutlastsTheBackoffWithOriginHoldingNothingTheBranchLacksIsNoDivergence", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		gitkit.CommitFile(t, f.hub.PrimeWorktree(), "local.txt", "local\n", "local")
		// Every push the hook sees is declined, and origin keeps no commit the branch lacks.
		installPreReceive(t, f.hub.CodeBare, "#!/bin/sh\necho 'declined' >&2\nexit 1\n")
		sleeps := 0

		_, err := hubforge.OpenFabric(t, f.hub).PushBranch(fake.options(countingBackoff(&sleeps, nil)))

		if !fabricengine.IsPushRejected(err) {
			t.Errorf("push error = %v; want IsPushRejected", err)
		}
		record := requireOneFailedPush(t, f.hub.Location.HubPath, "code", fabriclayout.PushFailureOther)
		if want := "(pre-receive hook declined)"; !strings.Contains(record.Error, want) {
			t.Errorf("record error = %q; want git's refusal line holding %q", record.Error, want)
		}
		if want := len(gitrepo.PushBackoffDelays()); sleeps < want {
			t.Errorf("backoff sleeps = %d; want at least %d, the whole backoff waited out before the record", sleeps, want)
		}
	})

	t.Run("ARejectionOfABranchWithNoUpstreamWhoseNameOriginAlreadyHoldsIsADivergence", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft := f.hub.PrimeRecords()
		branch := gitkit.CurrentBranch(t, weft)
		other := filepath.Join(t.TempDir(), "other")
		gitkit.Git(t, filepath.Dir(other), "clone", f.hub.RecordsBare, other)
		gitkit.Git(t, other, "checkout", "-B", branch)
		gitkit.CommitFile(t, other, "other.txt", "other\n", "other machine")
		gitkit.Git(t, other, "push", "origin", branch)
		local := gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")

		if _, err := fabricengine.CoalescePushRequest(fabriclayout.PushSpawn{WeftPath: weft}, fake.options(countingBackoff(new(int), nil)), fabriclayout.MachineNames{}); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
		}

		record := requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabriclayout.PushFailureNotFastForward)
		if !slices.Contains(record.Commits, local) {
			t.Errorf("record commits = %v; want the local commit %s the reset in its way forward would drop", record.Commits, local)
		}
		if len(fake.lines) != 1 {
			t.Errorf("mails = %q; want one", fake.lines)
		}
	})

	t.Run("APairPushFromARecordsStoreWithNoNamespacedFetchRefspecRecordsTheRestoreCommands", func(t *testing.T) {
		const slug = "ns-norefspec"
		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		fake := &fakePushNotifier{hub: h.Location.HubPath, posted: true}
		hubforge.AddPair(t, h, slug)
		sibling := h.PairRecordsSibling(slug)
		gitkit.CommitFile(t, sibling, "local.txt", "local\n", "local")
		gitkit.Git(t, sibling, "config", "--unset-all", "remote.origin.fetch")
		fabricengine.ForgetOriginNamespaceForTest(sibling)
		location, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}

		_, err = fabricengine.PushPairAnchored(location, fake.options(nil), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})

		var failed *fabricengine.PushFailedError
		if !errors.As(err, &failed) || failed.Record.Class != fabriclayout.PushFailureOther {
			t.Fatalf("PushPairAnchored() error = %v; want an other-class *PushFailedError", err)
		}
		want := "`git -C " + sibling + " config --replace-all remote.origin.fetch"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("PushPairAnchored() error = %q; want it to name %q", err, want)
		}
		record := requireOneFailedPush(t, h.Location.HubPath, "records", fabriclayout.PushFailureOther)
		if !strings.Contains(record.Error, want) {
			t.Errorf("record error = %q; want it to name %q", record.Error, want)
		}
		if len(fake.lines) != 1 {
			t.Errorf("mails = %q; want one", fake.lines)
		}
	})

	t.Run("AJudgmentFetchThatFailsAfterTheFinalRejectionIsRecordedAsOtherWithTheFetchError", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		gitkit.CommitFile(t, f.hub.PrimeWorktree(), "local.txt", "local\n", "local")
		// Every push is declined; the last one the backoff allows also breaks the bare repository,
		// so the fetch that judges the rejection fails.
		finalPush := len(gitrepo.PushBackoffDelays()) + 2
		bare := filepath.ToSlash(f.hub.CodeBare)
		marker := bare + "/declines"
		installPreReceive(t, f.hub.CodeBare, fmt.Sprintf("#!/bin/sh\nn=$(cat '%s' 2>/dev/null || echo 0)\nn=$((n + 1))\necho $n > '%s'\nif [ \"$n\" -ge %d ]; then\n  rm '%s/HEAD'\nfi\necho 'declined' >&2\nexit 1\n", marker, marker, finalPush, bare))

		_, err := hubforge.OpenFabric(t, f.hub).PushBranch(fake.options(countingBackoff(new(int), nil)))

		var failed *fabricengine.PushFailedError
		if !errors.As(err, &failed) {
			t.Fatalf("PushBranch() error = %v; want a *PushFailedError", err)
		}
		record := requireOneFailedPush(t, f.hub.Location.HubPath, "code", fabriclayout.PushFailureOther)
		for _, want := range []string{"(pre-receive hook declined)", "gitrepo: fetch in"} {
			if !strings.Contains(record.Error, want) {
				t.Errorf("record error = %q; want it to hold both the rejection line and the fetch error, missing %q", record.Error, want)
			}
		}
		if len(fake.lines) != 1 {
			t.Errorf("mails = %q; want one", fake.lines)
		}
	})

	// The hook's refusal text reaches fabric only through the `remote:` lines the rejection carries.
	t.Run("AProtectedBranchRefusalRecordsTheRejectionClassAndIsNotRetried", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		prime := f.hub.PrimeWorktree()
		gitkit.CommitFile(t, prime, "local.txt", "local\n", "local")
		branch := gitkit.CurrentBranch(t, prime)
		installPreReceive(t, f.hub.CodeBare, protectedRefusingHook(branch))
		sleeps := 0

		_, err := hubforge.OpenFabric(t, f.hub).PushBranch(fake.options(countingBackoff(&sleeps, nil)))

		if err == nil || !fabricengine.ProtectedBranchRefusal(err) {
			t.Fatalf("PushBranch() error = %v; want a protected-branch refusal", err)
		}
		requireOneFailedPush(t, f.hub.Location.HubPath, "code", fabriclayout.PushFailureOther)
		if sleeps != 0 {
			t.Errorf("backoff sleeps = %d; want none, a protected refusal is not retried", sleeps)
		}
	})

	t.Run("APushThatLostARaceIsRetriedUntilItLandsWithNoRecord", func(t *testing.T) {
		// The hook declines the first push and the first retry, the way a racing push of the same branch makes them lose;
		// origin keeps no commit the branch lacks.
		declineTwice := func(t *testing.T, bare string) {
			t.Helper()
			marker := filepath.ToSlash(filepath.Join(bare, "declines"))
			installPreReceive(t, bare, "#!/bin/sh\nn=$(cat '"+marker+"' 2>/dev/null || echo 0)\nif [ \"$n\" -lt 2 ]; then\n  echo $((n + 1)) > '"+marker+"'\n  echo 'declined' >&2\n  exit 1\nfi\nexit 0\n")
		}
		tests := []struct {
			name string
			push func(t *testing.T, f *recordsFixture, opts fabricengine.SyncOptions) (local, bare, branch string)
		}{
			{"Code", func(t *testing.T, f *recordsFixture, opts fabricengine.SyncOptions) (string, string, string) {
				prime := f.hub.PrimeWorktree()
				local := gitkit.CommitFile(t, prime, "local.txt", "local\n", "local")
				declineTwice(t, f.hub.CodeBare)
				if _, err := hubforge.OpenFabric(t, f.hub).PushBranch(opts); err != nil {
					t.Fatalf("PushBranch() error = %v; want nil", err)
				}
				return local, f.hub.CodeBare, gitkit.CurrentBranch(t, prime)
			}},
			{"Records", func(t *testing.T, f *recordsFixture, opts fabricengine.SyncOptions) (string, string, string) {
				weft, branch := f.primeWeft()
				local := gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
				declineTwice(t, f.hub.RecordsBare)
				if _, err := fabricengine.CoalescePushRequest(fabriclayout.PushSpawn{WeftPath: weft}, opts, fabriclayout.MachineNames{}); err != nil {
					t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
				}
				return local, f.hub.RecordsBare, branch
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				f, fake := newFailureFixture(t)
				sleeps := 0

				local, bare, branch := tt.push(t, f, fake.options(countingBackoff(&sleeps, nil)))

				if got := bareBranchTip(t, bare, branch); got != local {
					t.Errorf("origin tip = %s; want the local commit %s", got, local)
				}
				requireNoFailedPush(t, f.hub.Location.HubPath)
				if len(fake.lines) != 0 {
					t.Errorf("mails = %q; want none", fake.lines)
				}
				if sleeps != 1 {
					t.Errorf("backoff sleeps = %d; want 1, between the declined retry and the one that lands", sleeps)
				}
			})
		}
	})

	t.Run("AWeftPushThatMustReplayStopsOnAnAnchorChange", func(t *testing.T) {
		tests := []struct {
			name string
			// hubAnchor is the anchor of the hub the pair is made in.
			hubAnchor string
			// arrange makes the pair's weft push need a replay, and returns what the way forward must name.
			arrange func(t *testing.T, p *pairFixture) string
		}{
			{
				"OntoAnUpstreamCarryingAnotherMarkerSetsTheAnchorInTheAnchoredDir",
				"backend",
				func(t *testing.T, p *pairFixture) string {
					p.originCommit(p.weftBranch, lymcwd.AnchorFileName, "sub\n", "other machine re-anchored")
					gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
					return fmt.Sprintf("run `lym fabric anchor %s --set sub` in %s, which integrates the other marker",
						p.location.WorktreeName, filepath.Join(p.warpPath, "backend"))
				},
			},
			{
				"WhileTheOwnMarkerIsEmpty",
				".",
				func(t *testing.T, p *pairFixture) string {
					p.originCommit(p.weftBranch, "other.txt", "other\n", "other machine")
					gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
					if err := os.WriteFile(filepath.Join(p.weftPath, lymcwd.AnchorFileName), []byte("\n"), 0o644); err != nil {
						t.Fatalf("truncate the marker: %v", err)
					}
					return lymcwd.AnchorFileName
				},
			},
			{
				"WhileAMoveRecordStands",
				".",
				func(t *testing.T, p *pairFixture) string {
					p.originCommit(p.weftBranch, "other.txt", "other\n", "other machine")
					gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
					writeMoveRecord(t, p.weftPath, fabriclayout.MoveRecord{A: ".", B: "x", Kind: fabriclayout.MoveKindMove, PID: 4242})
					return "lym fabric anchor"
				},
			},
		}
		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				p := newPairFixtureAt(t, tt.hubAnchor, fmt.Sprintf("anchorstop%d", i))
				wantNamed := tt.arrange(t, p)
				weftHeadWas := gitkit.RevParse(t, p.weftPath, "HEAD")
				originTipWas := bareBranchTip(t, p.hub.RecordsBare, p.weftBranch)
				sleeps := 0

				_, err := fabricengine.PushPairAnchored(p.location, p.fake.options(countingBackoff(&sleeps, nil)), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})

				var failed *fabricengine.PushFailedError
				if !errors.As(err, &failed) || !errors.Is(err, fabricengine.ErrAnchorChange) {
					t.Fatalf("PushPairAnchored() error = %v; want a *PushFailedError wrapping ErrAnchorChange", err)
				}
				if sleeps != 0 {
					t.Errorf("backoff sleeps = %d; want none, an anchor stop is not retried", sleeps)
				}
				record := requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabriclayout.PushFailureAnchorChange)
				wayForward := fabricengine.FailedPushWayForward(record)
				if !strings.Contains(wayForward, wantNamed) || strings.Contains(wayForward, "git merge") {
					t.Errorf("way forward = %q; want it to name %q and no git merge", wayForward, wantNamed)
				}
				if got := gitkit.RevParse(t, p.weftPath, "HEAD"); got != weftHeadWas {
					t.Errorf("weft HEAD = %s; want unchanged %s", got, weftHeadWas)
				}
				if got := bareBranchTip(t, p.hub.RecordsBare, p.weftBranch); got != originTipWas {
					t.Errorf("origin tip = %s; want unchanged %s", got, originTipWas)
				}
				if len(p.fake.lines) != 1 || !strings.Contains(p.fake.lines[0], wayForward) {
					t.Errorf("mails = %q; want one carrying the record's way forward", p.fake.lines)
				}
			})
		}
	})

	t.Run("APushToAnUnreachableOriginWritesATransportRecordAfterTheBackoff", func(t *testing.T) {
		p := newPairFixture(t, "stuckpair")
		gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
		gitkit.Git(t, p.weftPath, "remote", "set-url", "origin", refusedOriginURL)
		sleeps := 0

		_, err := fabricengine.PushPairAnchored(p.location, p.fake.options(countingBackoff(&sleeps, nil)), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})

		var failed *fabricengine.PushFailedError
		if !errors.As(err, &failed) || failed.Record.Class != fabriclayout.PushFailureTransport {
			t.Fatalf("push error = %v; want a transport *PushFailedError", err)
		}
		requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabriclayout.PushFailureTransport)
		if want := len(gitrepo.PushBackoffDelays()); sleeps != want {
			t.Errorf("backoff sleeps = %d; want %d", sleeps, want)
		}
	})

	t.Run("ATaskBranchRewoundOnOriginIsHeldBackUntilTheRewindIsSettled", func(t *testing.T) {
		// Origin's task branch is reset past a pushed commit, as another machine's leased update would.
		p := newPairFixture(t, "resetpair")
		originHead := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
		dropped := gitkit.CommitFile(t, p.warpPath, "c1.txt", "c1\n", "c1")
		gitkit.Git(t, p.warpPath, "push", "origin", p.warpBranch)
		gitkit.Git(t, p.hub.CodeBare, "update-ref", "refs/heads/"+p.warpBranch, originHead)
		gitkit.CommitFile(t, p.warpPath, "c2.txt", "c2\n", "c2")
		f, err := fabricengine.Open(p.location)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}

		_, err = f.PushBranch(p.fake.options(nil))
		requireRewoundHoldBack(t, p, err, originHead, dropped)

		gitkit.Git(t, p.warpPath, "reset", "--keep", originHead)
		if _, err := f.PushBranch(p.fake.options(nil)); err != nil {
			t.Fatalf("PushBranch() after the reset error = %v; want nil", err)
		}
		requireNoFailedPush(t, p.hub.Location.HubPath)

		next := gitkit.CommitFile(t, p.warpPath, "c3.txt", "c3\n", "c3")
		if _, err := f.PushBranch(p.fake.options(nil)); err != nil {
			t.Fatalf("PushBranch() of a new commit error = %v; want nil, the branch no longer reading rewound", err)
		}
		if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != next {
			t.Errorf("origin tip = %s; want the new commit %s", got, next)
		}
	})

	t.Run("ThePrimesCodeBranchPushedAfterAnOriginRewindIsPushedAndRecordsNothing", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		prime := f.hub.PrimeWorktree()
		branch := gitkit.CurrentBranch(t, prime)
		originHead := bareBranchTip(t, f.hub.CodeBare, branch)
		gitkit.CommitFile(t, prime, "c1.txt", "c1\n", "c1")
		gitkit.Git(t, prime, "push", "origin", branch)
		gitkit.Git(t, f.hub.CodeBare, "update-ref", "refs/heads/"+branch, originHead)
		next := gitkit.CommitFile(t, prime, "c2.txt", "c2\n", "c2")

		if _, err := fabricengine.PushWarpRebaseFreeAt(prime, fake.options(nil)); err != nil {
			t.Fatalf("PushWarpRebaseFreeAt() error = %v; want nil", err)
		}

		if got := bareBranchTip(t, f.hub.CodeBare, branch); got != next {
			t.Errorf("origin tip = %s; want the prime's tip %s", got, next)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
		if len(fake.lines) != 0 {
			t.Errorf("mails = %q; want none", fake.lines)
		}
	})
}
