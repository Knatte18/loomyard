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
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// refusedOriginURL is an origin URL git refuses with `connection refused`, a phrase gitexec.IsTransportFailure matches.
const refusedOriginURL = "http://127.0.0.1:1/x"

// authRefusingHook is a pre-receive hook that declines every push the way GitHub declines one for want of permission.
const authRefusingHook = "#!/bin/sh\necho 'Permission to hub denied to tester.' >&2\nexit 1\n"

// newFailureFixture builds a hub anchored at its root with the board replay installed,
// and a notifier counting the mails of its own hub.
// A push reaches the notifier only through the options the notifier builds.
func newFailureFixture(t *testing.T) (*recordsFixture, *fakePushNotifier) {
	t.Helper()
	return newFailureFixtureAt(t, ".")
}

// newFailureFixtureAt builds the failure fixture over a hub anchored at anchor.
func newFailureFixtureAt(t *testing.T, anchor string) (*recordsFixture, *fakePushNotifier) {
	t.Helper()
	boardkit.InstallReplay(t)
	f := &recordsFixture{t: t, hub: hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})}
	return f, &fakePushNotifier{hub: f.hub.Location.HubPath, posted: true}
}

// failedPushesOf returns the hub's failed-push records.
func failedPushesOf(t *testing.T, hubPath string) []fabricengine.FailedPush {
	t.Helper()
	records, err := fabricengine.ListFailedPushes(hubPath)
	if err != nil {
		t.Fatalf("ListFailedPushes(%s): %v", hubPath, err)
	}
	return records
}

// requireOneFailedPush fails the test unless the hub holds exactly one failed-push record, and returns it.
func requireOneFailedPush(t *testing.T, hubPath, repo string, class fabricengine.PushFailureClass) fabricengine.FailedPush {
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
	lockDir := fabricengine.RecordsLockDirPath(weft)
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
	return newPairFixtureAt(t, slug, ".")
}

// newPairFixtureAt builds the pair fixture over a hub anchored at anchor.
func newPairFixtureAt(t *testing.T, slug, anchor string) *pairFixture {
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
		weftBranch:     fabricengine.RecordsBranchName(added.Branch),
	}
	if _, err := fabricengine.PushPairAnchored(location, fake.options(nil), fabricengine.LockWaitUnbounded); err != nil {
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
	if !errors.As(err, &failed) || failed.Record.Class != fabricengine.PushFailureRewound {
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
	requireOneFailedPush(t, p.hub.Location.HubPath, "code", fabricengine.PushFailureRewound)
	if len(p.fake.lines) != 1 {
		t.Errorf("mails = %q; want one", p.fake.lines)
	}
}

// TestPushFailureClasses is the enforcing test of the push classification:
// each row drives one push path against a refusing or moved origin;
// it reads the failed-push record, the mail and the origin it leaves.
func TestPushFailureClasses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("ADetachedWeftPushRejectedAsNotFastForwardIsReplayedAndPushed", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft, branch := f.primeWeft()
		f.originCommit(branch, "other.txt", "other\n", "other machine")
		gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")

		if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(nil)); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
		}

		if got, want := bareBranchTip(t, f.hub.RecordsBare, branch), gitkit.RevParse(t, weft, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the replayed local HEAD %s", got, want)
		}
		if files := gitkit.LsFiles(t, weft, "."); !slices.Contains(files, "other.txt") || !slices.Contains(files, "local.txt") {
			t.Errorf("tracked files = %v; want other.txt and local.txt, the replay's two sides", files)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
		if len(fake.lines) != 0 {
			t.Errorf("mails = %q; want none", fake.lines)
		}
	})

	t.Run("AWeftPushDefersUnderTheWriteLockAndTheNextCommitsPushLandsIt", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft, branch := f.primeWeft()
		f.originCommit(branch, "other.txt", "other\n", "other machine")
		gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
		before := bareBranchTip(t, f.hub.RecordsBare, branch)
		held := holdWeftWriteLock(t, weft)

		if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(nil)); err != nil {
			t.Fatalf("CoalescePushRequest() with the write lock held error = %v; want nil", err)
		}

		if got := bareBranchTip(t, f.hub.RecordsBare, branch); got != before {
			t.Errorf("origin tip = %s; want it unchanged at %s", got, before)
		}
		requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureDeferred)
		if len(fake.lines) != 0 {
			t.Errorf("mails = %q; want none for a deferral", fake.lines)
		}
		if _, err := os.Stat(filepath.Join(fabricengine.RecordsLockDirPath(weft), "pending.records")); !os.IsNotExist(err) {
			t.Errorf("pending flag stat error = %v; want the flag absent, since a deferral leaves the commits to the next push", err)
		}

		if err := held.Release(); err != nil {
			t.Fatalf("release the weft write lock: %v", err)
		}
		gitkit.CommitFile(t, weft, "second.txt", "second\n", "second local")
		if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(nil)); err != nil {
			t.Fatalf("CoalescePushRequest() after the release error = %v; want nil", err)
		}
		if got, want := bareBranchTip(t, f.hub.RecordsBare, branch), gitkit.RevParse(t, weft, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the local HEAD %s", got, want)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})

	t.Run("AWeftReplayMeetingABothSidesStatusChangeWritesANotFastForwardRecordAndMails", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft, branch := f.primeWeft()
		f.originCommit(branch, "status.json", "from the other machine\n", "other status")
		local := gitkit.CommitFile(t, weft, "status.json", "from this machine\n", "local status")
		before := bareBranchTip(t, f.hub.RecordsBare, branch)
		sleeps := 0

		if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(countingBackoff(&sleeps, nil))); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil, a detached push never fails", err)
		}
		if sleeps != 0 {
			t.Errorf("backoff sleeps = %d; want none, a replay conflict is not retried", sleeps)
		}

		record := requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureNotFastForward)
		if !slices.Equal(record.Paths, []string{"status.json"}) || !slices.Contains(record.Commits, local) {
			t.Errorf("record = %+v; want the conflicting path status.json and the local commit %s", record, local)
		}
		if got := bareBranchTip(t, f.hub.RecordsBare, branch); got != before {
			t.Errorf("origin tip = %s; want it unchanged at %s", got, before)
		}
		if got := gitkit.RevParse(t, weft, "HEAD"); got != local {
			t.Errorf("local HEAD = %s; want the replay's stop to restore %s", got, local)
		}
		if len(fake.lines) != 1 || !strings.Contains(fake.lines[0], fabricengine.FailedPushWayForward(record)) {
			t.Errorf("mails = %q; want one carrying the record's way forward", fake.lines)
		}
	})

	t.Run("ARejectionThatOutlastsTheBackoffWithOriginHoldingNothingTheBranchLacksIsNoDivergence", func(t *testing.T) {
		// Every push the hook sees is declined, so the rejection outlasts the backoff;
		// origin keeps no commit the branch lacks.
		const declining = "#!/bin/sh\necho 'declined' >&2\nexit 1\n"
		tests := []struct {
			name string
			push func(t *testing.T, f *recordsFixture, opts fabricengine.SyncOptions) (side string, err error)
		}{
			{"Code", func(t *testing.T, f *recordsFixture, opts fabricengine.SyncOptions) (string, error) {
				gitkit.CommitFile(t, f.hub.PrimeWorktree(), "local.txt", "local\n", "local")
				installPreReceive(t, f.hub.CodeBare, declining)
				_, err := hubforge.OpenFabric(t, f.hub).PushBranch(opts)
				return "code", err
			}},
			{"Records", func(t *testing.T, f *recordsFixture, opts fabricengine.SyncOptions) (string, error) {
				weft, branch := f.primeWeft()
				f.originCommit(branch, "other.txt", "other\n", "other machine")
				gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
				// The first push never reaches the hook, since git refuses it against the other machine's tip;
				// the replay retry's pushes do.
				installPreReceive(t, f.hub.RecordsBare, declining)
				if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, opts); err != nil {
					t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
				}
				return "records", nil
			}},
			{"Bolt", func(t *testing.T, f *recordsFixture, opts fabricengine.SyncOptions) (string, error) {
				board := f.hub.BoardDir()
				gitkit.CommitFile(t, board, "notes/a.md", "a\n", "board change")
				installPreReceive(t, f.hub.RecordsBare, declining)
				return "bolt", fabricengine.NewBolt(board).Push(opts)
			}},
			// A branch never pushed has no upstream to compare with, so the rejection cannot be judged a divergence.
			{"RecordsWithNoUpstream", func(t *testing.T, f *recordsFixture, opts fabricengine.SyncOptions) (string, error) {
				weft := f.hub.PrimeRecords()
				gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
				installPreReceive(t, f.hub.RecordsBare, declining)
				if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, opts); err != nil {
					t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
				}
				return "records", nil
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				f, fake := newFailureFixture(t)
				sleeps := 0

				side, err := tt.push(t, f, fake.options(countingBackoff(&sleeps, nil)))

				if err != nil && !fabricengine.IsPushRejected(err) {
					t.Errorf("push error = %v; want nil or IsPushRejected", err)
				}
				record := requireOneFailedPush(t, f.hub.Location.HubPath, side, fabricengine.PushFailureOther)
				if want := "(pre-receive hook declined)"; !strings.Contains(record.Error, want) {
					t.Errorf("record error = %q; want git's refusal line holding %q", record.Error, want)
				}
				if len(fake.lines) != 1 {
					t.Fatalf("mails = %q; want one", fake.lines)
				}
				if mail := fake.lines[0]; strings.Contains(mail, "holds commits this branch lacks") || strings.Contains(mail, "git merge") {
					t.Errorf("mail = %q; want no divergence and no merge named", mail)
				}
				if wayForward := fabricengine.FailedPushWayForward(record); !strings.Contains(wayForward, "to retry the push") || !strings.Contains(fake.lines[0], wayForward) {
					t.Errorf("mail = %q; want it to carry the plain-retry way forward %q", fake.lines[0], wayForward)
				}
				if want := len(gitrepo.PushBackoffDelays()); sleeps < want {
					t.Errorf("backoff sleeps = %d; want at least %d, the whole backoff waited out before the record", sleeps, want)
				}
			})
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
		gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")

		if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(countingBackoff(new(int), nil))); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
		}

		requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureNotFastForward)
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
		record := requireOneFailedPush(t, f.hub.Location.HubPath, "code", fabricengine.PushFailureOther)
		for _, want := range []string{"(pre-receive hook declined)", "gitrepo: fetch in"} {
			if !strings.Contains(record.Error, want) {
				t.Errorf("record error = %q; want it to hold both the rejection line and the fetch error, missing %q", record.Error, want)
			}
		}
		if len(fake.lines) != 1 {
			t.Errorf("mails = %q; want one", fake.lines)
		}
	})

	t.Run("ACodeRejectionWhoseRetryIsRefusedForAuthIsNotRetriedAgain", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		prime := f.hub.PrimeWorktree()
		gitkit.CommitFile(t, prime, "local.txt", "local\n", "local")
		// The first push is a bare rejection; every later one is refused for permission.
		marker := filepath.ToSlash(filepath.Join(f.hub.CodeBare, "declines"))
		installPreReceive(t, f.hub.CodeBare, "#!/bin/sh\nn=$(cat '"+marker+"' 2>/dev/null || echo 0)\necho $((n + 1)) > '"+marker+"'\nif [ \"$n\" -lt 1 ]; then\n  echo 'declined' >&2\nelse\n  echo 'Permission to hub denied to tester.' >&2\nfi\nexit 1\n")
		sleeps := 0

		_, err := hubforge.OpenFabric(t, f.hub).PushBranch(fake.options(countingBackoff(&sleeps, nil)))

		if err == nil {
			t.Fatal("PushBranch() error = nil; want a failed push")
		}
		requireOneFailedPush(t, f.hub.Location.HubPath, "code", fabricengine.PushFailureAuth)
		if sleeps != 0 {
			t.Errorf("backoff sleeps = %d; want none, an auth refusal of the retry is not retried", sleeps)
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
				if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, opts); err != nil {
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
			// arrange makes the pair's weft push need a replay, and returns what the way forward must name.
			arrange func(t *testing.T, p *pairFixture) string
		}{
			{
				"OntoAnUpstreamCarryingAnotherMarker",
				func(t *testing.T, p *pairFixture) string {
					p.originCommit(p.weftBranch, lymcwd.AnchorFileName, "sub\n", "other machine re-anchored")
					gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
					return "lym fabric anchor"
				},
			},
			{
				"WhileTheOwnMarkerIsEmpty",
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
				func(t *testing.T, p *pairFixture) string {
					p.originCommit(p.weftBranch, "other.txt", "other\n", "other machine")
					gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
					writeMoveRecord(t, p.weftPath, fabricengine.MoveRecord{A: ".", B: "x", Kind: fabricengine.MoveKindMove, PID: 4242})
					return "lym fabric anchor"
				},
			},
		}
		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				p := newPairFixture(t, fmt.Sprintf("anchorstop%d", i))
				wantNamed := tt.arrange(t, p)
				weftHeadWas := gitkit.RevParse(t, p.weftPath, "HEAD")
				originTipWas := bareBranchTip(t, p.hub.RecordsBare, p.weftBranch)
				sleeps := 0

				_, err := fabricengine.PushPairAnchored(p.location, p.fake.options(countingBackoff(&sleeps, nil)), fabricengine.LockWaitUnbounded)

				var failed *fabricengine.PushFailedError
				if !errors.As(err, &failed) || !errors.Is(err, fabricengine.ErrAnchorChange) {
					t.Fatalf("PushPairAnchored() error = %v; want a *PushFailedError wrapping ErrAnchorChange", err)
				}
				if sleeps != 0 {
					t.Errorf("backoff sleeps = %d; want none, an anchor stop is not retried", sleeps)
				}
				record := requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabricengine.PushFailureAnchorChange)
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

	t.Run("RepeatedAuthRefusalsShareOneRecordAndOneMailAndALandedPushRemovesIt", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft, branch := f.primeWeft()
		gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
		installPreReceive(t, f.hub.RecordsBare, authRefusingHook)

		for range 2 {
			if _, err := fabricengine.CoalescePushBothAt("", weft, fake.options(nil)); err != nil {
				t.Fatalf("CoalescePushBothAt() error = %v; want nil, a detached push never fails", err)
			}
		}

		record := requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureAuth)
		if record.Count != 2 || !record.Notified {
			t.Errorf("record = %+v; want count 2 and notified", record)
		}
		if len(fake.lines) != 1 {
			t.Errorf("mails = %q; want exactly one for the episode", fake.lines)
		}

		if err := os.Remove(preReceiveHookPath(f.hub.RecordsBare)); err != nil {
			t.Fatalf("remove the hook: %v", err)
		}
		if _, err := fabricengine.CoalescePushBothAt("", weft, fake.options(nil)); err != nil {
			t.Fatalf("CoalescePushBothAt() after the hook is gone error = %v; want nil", err)
		}
		if got, want := bareBranchTip(t, f.hub.RecordsBare, branch), gitkit.RevParse(t, weft, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the local HEAD %s", got, want)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})

	t.Run("APushToAnUnreachableOriginWritesATransportRecord", func(t *testing.T) {
		t.Run("OfABranchNeverPushed", func(t *testing.T) {
			f, fake := newFailureFixture(t)
			weft := f.hub.PrimeRecords()
			gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
			gitkit.Git(t, weft, "remote", "set-url", "origin", refusedOriginURL)

			if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(nil)); err != nil {
				t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
			}

			requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureTransport)
			if len(fake.lines) != 1 {
				t.Errorf("mails = %q; want one", fake.lines)
			}
		})

		t.Run("OfABranchWithAnUpstreamAfterTheBackoff", func(t *testing.T) {
			tests := []struct {
				name string
				// push commits on a weft that has an upstream, points its origin at refusedOriginURL and pushes it.
				// It sleeps each backoff delay through backoff.
				// It returns the hub's path, the notifier and the push's error.
				push func(t *testing.T, backoff func(time.Duration)) (string, *fakePushNotifier, error)
				// wantFailed reports that the entry point returns the transport *PushFailedError; otherwise it returns nil.
				wantFailed bool
			}{
				{
					name: "ADetachedPrimeWeftPush",
					push: func(t *testing.T, backoff func(time.Duration)) (string, *fakePushNotifier, error) {
						f, fake := newFailureFixture(t)
						weft, _ := f.primeWeft()
						gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
						gitkit.Git(t, weft, "remote", "set-url", "origin", refusedOriginURL)
						_, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(backoff))
						return f.hub.Location.HubPath, fake, err
					},
				},
				{
					name: "AParkedPairPush",
					push: func(t *testing.T, backoff func(time.Duration)) (string, *fakePushNotifier, error) {
						p := newPairFixture(t, "stuckpair")
						gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
						gitkit.Git(t, p.weftPath, "remote", "set-url", "origin", refusedOriginURL)
						_, err := fabricengine.PushAnchored(p.location, p.fake.options(backoff), fabricengine.LockWaitUnbounded)
						return p.hub.Location.HubPath, p.fake, err
					},
					wantFailed: true,
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var sleeps int

					hubPath, fake, err := tt.push(t, countingBackoff(&sleeps, nil))

					if tt.wantFailed {
						var failed *fabricengine.PushFailedError
						if !errors.As(err, &failed) || failed.Record.Class != fabricengine.PushFailureTransport {
							t.Fatalf("push error = %v; want a transport *PushFailedError", err)
						}
					} else if err != nil {
						t.Fatalf("push error = %v; want nil", err)
					}
					requireOneFailedPush(t, hubPath, "records", fabricengine.PushFailureTransport)
					if want := len(gitrepo.PushBackoffDelays()); sleeps != want {
						t.Errorf("backoff sleeps = %d; want %d", sleeps, want)
					}
					if len(fake.lines) != 1 {
						t.Errorf("mails = %q; want one", fake.lines)
					}
				})
			}
		})
	})

	t.Run("APairPushFailingInASubpathAnchoredHubNamesTheAnchoredDirectory", func(t *testing.T) {
		const anchor = "backend"
		tests := []struct {
			name string
			side string
			// refused returns the worktree of the side whose push origin refuses.
			refused func(p *pairFixture) string
		}{
			{"Code", "code", func(p *pairFixture) string { return p.warpPath }},
			{"Records", "records", func(p *pairFixture) string { return p.weftPath }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				p := newPairFixtureAt(t, "anchoredpair", anchor)
				refused := tt.refused(p)
				gitkit.CommitFile(t, refused, "local.txt", "local\n", "local")
				gitkit.Git(t, refused, "remote", "set-url", "origin", refusedOriginURL)

				_, err := fabricengine.PushAnchored(p.location, p.fake.options(countingBackoff(new(int), nil)), fabricengine.LockWaitUnbounded)

				if err == nil {
					t.Fatal("PushAnchored() error = nil; want the refused push's failure")
				}
				record := requireOneFailedPush(t, p.hub.Location.HubPath, tt.side, fabricengine.PushFailureTransport)
				anchored := filepath.Join(p.warpPath, anchor)
				if record.FabricDir != anchored {
					t.Errorf("record FabricDir = %q; want the pair's anchored directory %q", record.FabricDir, anchored)
				}
				want := fmt.Sprintf("way forward: run `lym fabric push` in %s to retry the push", anchored)
				if len(p.fake.lines) != 1 || !strings.HasSuffix(p.fake.lines[0], want) {
					t.Errorf("mails = %q; want one ending in %q", p.fake.lines, want)
				}
			})
		}
	})

	t.Run("APushFailingForNoNamedReasonWritesAnOtherRecordAndMails", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft := f.hub.PrimeRecords()
		gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
		gitkit.Git(t, weft, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-repository"))

		if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(nil)); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
		}

		requireOneFailedPush(t, f.hub.Location.HubPath, "records", fabricengine.PushFailureOther)
		if len(fake.lines) != 1 {
			t.Errorf("mails = %q; want one", fake.lines)
		}
	})

	t.Run("AParkedPushOfABranchGoneOnOriginDoesNotRecreateIt", func(t *testing.T) {
		t.Run("AnEntryNotEndedWritesAGoneRecordAfterTheBackoff", func(t *testing.T) {
			p := newPairFixture(t, "gonepair")
			gitkit.Git(t, p.hub.RecordsBare, "branch", "-D", p.weftBranch)
			gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
			var sleeps int

			_, err := fabricengine.PushAnchored(p.location, p.fake.options(countingBackoff(&sleeps, nil)), fabricengine.LockWaitUnbounded)

			var failed *fabricengine.PushFailedError
			if !errors.As(err, &failed) || failed.Record.Class != fabricengine.PushFailureGone || !errors.Is(err, fabricengine.ErrBranchGone) {
				t.Fatalf("PushAnchored() error = %v; want a gone *PushFailedError wrapping ErrBranchGone", err)
			}
			if !strings.HasSuffix(err.Error(), "way forward: "+fabricengine.FailedPushWayForward(failed.Record)) {
				t.Errorf("PushAnchored() error = %q; want it to end in the gone way forward", err)
			}
			if originHasBranch(t, p.hub.RecordsBare, p.weftBranch) {
				t.Errorf("origin holds %s again; want a parked push never to recreate it", p.weftBranch)
			}
			if want := len(gitrepo.PushBackoffDelays()); sleeps != want {
				t.Errorf("backoff sleeps = %d; want %d", sleeps, want)
			}
			requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabricengine.PushFailureGone)
			if len(p.fake.lines) != 1 {
				t.Errorf("mails = %q; want one", p.fake.lines)
			}
		})

		t.Run("AnEntryEndedOnOriginSinceTheLastBoltFetchWritesNoRecord", func(t *testing.T) {
			p := newPairFixture(t, "endedpair")
			gitkit.Git(t, p.hub.RecordsBare, "branch", "-D", p.weftBranch)
			p.endEntryOnOrigin("endedpair")
			gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
			var sleeps int

			if _, err := fabricengine.PushAnchored(p.location, p.fake.options(countingBackoff(&sleeps, nil)), fabricengine.LockWaitUnbounded); err != nil {
				t.Fatalf("PushAnchored() error = %v; want nil, the ended entry's branch is left to its teardown", err)
			}

			if originHasBranch(t, p.hub.RecordsBare, p.weftBranch) {
				t.Errorf("origin holds %s again; want it left gone", p.weftBranch)
			}
			if sleeps != 0 {
				t.Errorf("backoff sleeps = %d; want none, since the entry reads ended on the first fetch", sleeps)
			}
			requireNoFailedPush(t, p.hub.Location.HubPath)
			if len(p.fake.lines) != 0 {
				t.Errorf("mails = %q; want none", p.fake.lines)
			}
		})

		t.Run("TheDoneMarkArrivingDuringTheBackoffEndsItWithNoRecord", func(t *testing.T) {
			p := newPairFixture(t, "latepair")
			gitkit.Git(t, p.hub.RecordsBare, "branch", "-D", p.weftBranch)
			gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
			var sleeps int
			backoff := countingBackoff(&sleeps, func(int) { p.endEntryOnOrigin("latepair") })

			if _, err := fabricengine.PushAnchored(p.location, p.fake.options(backoff), fabricengine.LockWaitUnbounded); err != nil {
				t.Fatalf("PushAnchored() error = %v; want nil", err)
			}

			if sleeps != 1 {
				t.Errorf("backoff sleeps = %d; want 1, the done mark ending the backoff", sleeps)
			}
			if originHasBranch(t, p.hub.RecordsBare, p.weftBranch) {
				t.Errorf("origin holds %s again; want it left gone", p.weftBranch)
			}
			requireNoFailedPush(t, p.hub.Location.HubPath)
		})

		t.Run("AnUnreachableOriginRestoredDuringTheBackoffRecordsNothingAndPushes", func(t *testing.T) {
			p := newPairFixture(t, "restorepair")
			realURL := gitkit.Git(t, p.weftPath, "remote", "get-url", "origin")
			gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
			gitkit.Git(t, p.weftPath, "remote", "set-url", "origin", refusedOriginURL)
			var sleeps int
			backoff := countingBackoff(&sleeps, func(int) { gitkit.Git(t, p.weftPath, "remote", "set-url", "origin", realURL) })

			if _, err := fabricengine.PushAnchored(p.location, p.fake.options(backoff), fabricengine.LockWaitUnbounded); err != nil {
				t.Fatalf("PushAnchored() error = %v; want nil", err)
			}

			if got, want := bareBranchTip(t, p.hub.RecordsBare, p.weftBranch), gitkit.RevParse(t, p.weftPath, "HEAD"); got != want {
				t.Errorf("origin tip = %s; want the pushed local HEAD %s", got, want)
			}
			if sleeps != 1 {
				t.Errorf("backoff sleeps = %d; want 1", sleeps)
			}
			requireNoFailedPush(t, p.hub.Location.HubPath)
		})
	})

	t.Run("APrimeWeftPushRejectedAfterThisMachinesFinalizeKeepsTheLandingsCommits", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft, branch := f.primeWeft()
		hubforge.AddPair(t, f.hub, "neve")
		pairStatus := filepath.Join(f.hub.PairCodeWorktree("neve"), "_lym", "shed", "neve", "status.json")
		if err := os.MkdirAll(filepath.Dir(pairStatus), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", pairStatus, err)
		}
		if err := os.WriteFile(pairStatus, []byte(statusJSON("Land", "Finalize")), 0o644); err != nil {
			t.Fatalf("write %s: %v", pairStatus, err)
		}
		const statusRel = "_lym/shed/neve/status.json"
		gitkit.CommitFile(t, weft, statusRel, statusJSON("Run-Shed"), "neve prime status")
		f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
		f.endEntryOnOrigin("neve")

		if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: weft}, fake.options(nil)); err != nil {
			t.Fatalf("CoalescePushRequest() error = %v; want nil", err)
		}

		if got := gitkit.Git(t, f.hub.RecordsBare, "ls-tree", "--name-only", bareBranchTip(t, f.hub.RecordsBare, branch), statusRel); strings.TrimSpace(got) != statusRel {
			t.Errorf("origin tip tracks %q; want %s, the landing's commit kept", got, statusRel)
		}
		if tags := tagsOf(t, weft, "set-aside/*"); len(tags) != 0 {
			t.Errorf("set-aside tags = %v; want none", tags)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})

	t.Run("ABoltPushRejectedOverAnUnrelatedOriginEntryIsReplayedAndLands", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		board := f.hub.BoardDir()
		branch := f.boardBranch()
		gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("a", withBoardTitle("A")), boardEntry("b", withBoardTitle("B"))), "base board")
		gitkit.Git(t, board, "push", "origin", branch)
		gitkit.CommitFile(t, board, "board.json", boardFileOf(t, boardEntry("a", withBoardTitle("A")), boardEntry("b", withBoardTitle("B2"))), "retitle b")
		f.originCommit(branch, "board.json", boardFileOf(t, boardEntry("a", withBoardTitle("A2")), boardEntry("b", withBoardTitle("B"))), "retitle a")

		if err := fabricengine.NewBolt(board).Push(fake.options(nil)); err != nil {
			t.Fatalf("Bolt.Push() error = %v; want nil", err)
		}

		if got, want := bareBranchTip(t, f.hub.RecordsBare, branch), gitkit.RevParse(t, board, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the replayed board tip %s", got, want)
		}
		if titles := boardTitles(t, board); titles["a"] != "A2" || titles["b"] != "B2" {
			t.Errorf("board titles = %v; want both machines' retitles", titles)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})

	t.Run("ABoltPushRejectedWhileTheBoardLockIsHeldDefersWithADeferredRecord", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		board := f.hub.BoardDir()
		branch := f.boardBranch()
		gitkit.CommitFile(t, board, "notes/a.md", "a\n", "board change")
		f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
		boardLockPath, err := fabricengine.BoltLockPath(board, fabricengine.BoardWriteLockFile)
		if err != nil {
			t.Fatalf("locate the board lock: %v", err)
		}
		boardLock, err := lock.AcquireWriteLock(boardLockPath)
		if err != nil {
			t.Fatalf("take the board lock: %v", err)
		}
		before := bareBranchTip(t, f.hub.RecordsBare, branch)

		if err := fabricengine.NewBolt(board).Push(fake.options(nil)); err != nil {
			t.Fatalf("Bolt.Push() with the board lock held error = %v; want nil", err)
		}

		requireOneFailedPush(t, f.hub.Location.HubPath, "bolt", fabricengine.PushFailureDeferred)
		if got := bareBranchTip(t, f.hub.RecordsBare, branch); got != before {
			t.Errorf("origin tip = %s; want it unchanged at %s", got, before)
		}
		if len(fake.lines) != 0 {
			t.Errorf("mails = %q; want none for a deferral", fake.lines)
		}
		if err := boardLock.Release(); err != nil {
			t.Fatalf("release the board lock: %v", err)
		}
		if err := fabricengine.NewBolt(board).Push(fake.options(nil)); err != nil {
			t.Fatalf("Bolt.Push() after the release error = %v; want nil", err)
		}
		if got, want := bareBranchTip(t, f.hub.RecordsBare, branch), gitkit.RevParse(t, board, "HEAD"); got != want {
			t.Errorf("origin tip = %s; want the board tip %s", got, want)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})

	t.Run("ATaskBranchRewoundOnOriginIsHeldBackUntilTheRewindIsSettled", func(t *testing.T) {
		// rewound pushes a pair's first commit and rewinds origin's task branch past it;
		// as another machine's leased update would, it commits again locally.
		rewound := func(t *testing.T, slug string) (p *pairFixture, f *fabricengine.Fabric, originHead, dropped string) {
			t.Helper()
			p = newPairFixture(t, slug)
			originHead = bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
			dropped = gitkit.CommitFile(t, p.warpPath, "c1.txt", "c1\n", "c1")
			gitkit.Git(t, p.warpPath, "push", "origin", p.warpBranch)
			gitkit.Git(t, p.hub.CodeBare, "update-ref", "refs/heads/"+p.warpBranch, originHead)
			gitkit.CommitFile(t, p.warpPath, "c2.txt", "c2\n", "c2")
			opened, err := fabricengine.Open(p.location)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			return p, opened, originHead, dropped
		}

		t.Run("SettledByResettingToOriginsHead", func(t *testing.T) {
			p, f, originHead, dropped := rewound(t, "resetpair")
			_, err := f.PushBranch(p.fake.options(nil))
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

		t.Run("SettledByAPlainPushThatRestoresTheDroppedCommits", func(t *testing.T) {
			p, f, originHead, dropped := rewound(t, "restorepair2")
			_, err := f.PushBranch(p.fake.options(nil))
			requireRewoundHoldBack(t, p, err, originHead, dropped)

			gitkit.Git(t, p.warpPath, "push", "origin", p.warpBranch)
			if _, err := f.PushBranch(p.fake.options(nil)); err != nil {
				t.Fatalf("PushBranch() after the plain push error = %v; want nil", err)
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

	t.Run("APrimeCodePushIsRecordedWithThePrimeFormOfItsWayForward", func(t *testing.T) {
		t.Run("RejectedBecauseTheOtherMachinePushedFirst", func(t *testing.T) {
			f, fake := newFailureFixture(t)
			prime := f.hub.PrimeWorktree()
			branch := gitkit.CurrentBranch(t, prime)
			other := filepath.Join(t.TempDir(), "other")
			gitkit.Git(t, filepath.Dir(other), "clone", "--branch", branch, f.hub.CodeBare, other)
			gitkit.CommitFile(t, other, "other.txt", "other\n", "other machine")
			gitkit.Git(t, other, "push", "origin", branch)
			gitkit.CommitFile(t, prime, "local.txt", "local\n", "local")
			opened := hubforge.OpenFabric(t, f.hub)
			sleeps := 0

			_, err := opened.PushBranch(fake.options(countingBackoff(&sleeps, nil)))

			var failed *fabricengine.PushFailedError
			if !errors.As(err, &failed) || failed.Record.Class != fabricengine.PushFailureNotFastForward || !failed.Record.Prime {
				t.Fatalf("PushBranch() error = %v; want a not-fast-forward *PushFailedError of the prime", err)
			}
			if sleeps != 0 {
				t.Errorf("backoff sleeps = %d; want 0, since origin holding commits the branch lacks is reported at once", sleeps)
			}
			if want := fmt.Sprintf("then run `git push` in %s", prime); !strings.HasSuffix(err.Error(), want) {
				t.Errorf("PushBranch() error = %q; want it to end in %q", err, want)
			}
			if strings.Contains(err.Error(), "lym fabric push") {
				t.Errorf("PushBranch() error = %q; want no `lym fabric push`, which leaves the prime's code branch unpushed", err)
			}
			requireOneFailedPush(t, f.hub.Location.HubPath, "code", fabricengine.PushFailureNotFastForward)
		})

		t.Run("RefusedForWantOfPermission", func(t *testing.T) {
			f, fake := newFailureFixture(t)
			prime := f.hub.PrimeWorktree()
			gitkit.CommitFile(t, prime, "local.txt", "local\n", "local")
			installPreReceive(t, f.hub.CodeBare, authRefusingHook)
			opened := hubforge.OpenFabric(t, f.hub)

			_, err := opened.PushBranch(fake.options(nil))

			var failed *fabricengine.PushFailedError
			if !errors.As(err, &failed) || failed.Record.Class != fabricengine.PushFailureAuth || !failed.Record.Prime || !errors.Is(err, gitrepo.ErrPushAuthRefused) {
				t.Fatalf("PushBranch() error = %v; want an auth *PushFailedError of the prime wrapping ErrPushAuthRefused", err)
			}
			if want := fmt.Sprintf("`git push` in %s", prime); !strings.HasSuffix(err.Error(), want) {
				t.Errorf("PushBranch() error = %q; want it to end naming %q", err, want)
			}
			requireOneFailedPush(t, f.hub.Location.HubPath, "code", fabricengine.PushFailureAuth)
		})
	})
}
