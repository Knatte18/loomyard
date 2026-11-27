// catchuppush_integration_test.go drives the catch-up's push step over real hubs with a pair and bare origins.
// No test here calls t.Parallel: a push reads the per-user machine file the whole test process shares.

package battencli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// catchUpPushFixture is one hub with the pair "alpha", whose prime and pair pusher locks the test holds.
type catchUpPushFixture struct {
	h    *hubforge.Hub
	pair *lymcwd.Location
	// branch is the pair's records branch.
	branch  string
	started []runpusher.Target
	// onStart runs at each start, before the real start.
	onStart func(target runpusher.Target)
}

func newCatchUpPushFixture(t *testing.T) *catchUpPushFixture {
	t.Helper()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	added := hubforge.AddPair(t, h, "alpha")
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("alpha"))
	if err != nil {
		t.Fatalf("ResolveWorktree() = %v; want nil", err)
	}
	for _, target := range []runpusher.Target{{Location: h.Location}, {Location: pair, Slug: "alpha", Pair: true}} {
		path := runpusher.LockPath(target)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) = %v; want nil", filepath.Dir(path), err)
		}
		held, err := lock.AcquireWriteLock(path)
		if err != nil {
			t.Fatalf("AcquireWriteLock(%s) = %v; want nil", path, err)
		}
		t.Cleanup(func() { _ = held.Release() })
	}
	return &catchUpPushFixture{h: h, pair: pair, branch: fabriclayout.RecordsBranchName(added.Branch)}
}

// push runs the catch-up's push step for slug through the production seams.
// It uses a short lock wait, no backoff delay and a start that records its target.
func (f *catchUpPushFixture) push(slug string) error {
	return pushOwedForCatchUp(f.h.Location, slug, catchUpPushSeams{
		Pair: catchUpPair,
		Held: pushledger.HeldRecordsCommit,
		Push: pairteardown.PushRun,
		Start: func(target runpusher.Target) (bool, error) {
			f.started = append(f.started, target)
			if f.onStart != nil {
				f.onStart(target)
			}
			return runpusher.Start(target)
		},
		LockWait:     5 * time.Millisecond,
		BackoffSleep: func(time.Duration) {},
	})
}

// startedSlugs returns the slug of each target started, the prime's being empty.
func (f *catchUpPushFixture) startedSlugs() []string {
	slugs := []string{}
	for _, target := range f.started {
		slugs = append(slugs, target.Slug)
	}
	return slugs
}

// TestPushOwedForCatchUp asserts an owed push and a held records commit go to the pusher and never fail the catch-up.
// A refusal does fail it.
func TestPushOwedForCatchUp(t *testing.T) {
	tiergate.Require(t, 1)

	t.Run("A_held_pair_push_lock_hands_the_pair_to_the_pusher", func(t *testing.T) {
		f := newCatchUpPushFixture(t)
		gitkit.CommitFile(t, f.h.PairRecordsSibling("alpha"), "alpha.txt", "alpha\n", "alpha transition")
		release, err := fabricengine.HoldPairPushLock(f.pair)
		if err != nil {
			t.Fatalf("HoldPairPushLock() = %v; want nil", err)
		}
		defer release()

		if err := f.push("alpha"); err != nil {
			t.Fatalf("pushOwedForCatchUp() = %v; want nil", err)
		}
		if got := f.startedSlugs(); !slices.Equal(got, []string{"alpha"}) {
			t.Errorf("started = %q; want the pair alone", got)
		}
	})

	t.Run("An_unreachable_prime_origin_hands_the_prime_to_the_pusher_and_still_pushes_the_pair", func(t *testing.T) {
		f := newCatchUpPushFixture(t)
		records := f.h.PrimeRecords()
		gitkit.CommitFile(t, records, "prime.txt", "prime\n", "prime transition")
		head := gitkit.CommitFile(t, f.h.PairRecordsSibling("alpha"), "alpha.txt", "alpha\n", "alpha transition")
		url := gitkit.Git(t, records, "remote", "get-url", "origin")
		gitkit.Git(t, records, "remote", "set-url", "origin", "http://127.0.0.1:1/x")
		// The pair's records worktree shares the prime's remote config, so origin returns once the prime is handed over.
		f.onStart = func(target runpusher.Target) {
			if !target.Pair {
				gitkit.Git(t, records, "remote", "set-url", "origin", url)
			}
		}

		if err := f.push("alpha"); err != nil {
			t.Fatalf("pushOwedForCatchUp() = %v; want nil", err)
		}
		if got := f.startedSlugs(); !slices.Equal(got, []string{""}) {
			t.Errorf("started = %q; want the prime alone", got)
		}
		if got := recordsTip(t, f.h, f.branch); got != head {
			t.Errorf("origin's records tip of alpha = %s; want %s", got, head)
		}
	})

	t.Run("A_diverged_origin_is_a_refusal_and_starts_nothing", func(t *testing.T) {
		f := newCatchUpPushFixture(t)
		records := f.h.PrimeRecords()
		branch := gitkit.CurrentBranch(t, records)
		gitkit.Git(t, records, "push", "-u", "origin", branch)
		other := filepath.Join(t.TempDir(), "other")
		gitkit.Git(t, t.TempDir(), "clone", "--branch", branch, f.h.RecordsBare, other)
		gitkit.CommitFile(t, other, "clash.txt", "origin\n", "origin's side")
		gitkit.Git(t, other, "push")
		gitkit.CommitFile(t, records, "clash.txt", "local\n", "this side")

		if err := f.push("alpha"); err == nil {
			t.Error("pushOwedForCatchUp() = nil; want the refusal")
		}
		if got := f.startedSlugs(); len(got) != 0 {
			t.Errorf("started = %q; want none", got)
		}
	})

	t.Run("A_held_run_hands_the_pair_to_the_pusher_short_of_the_held_commit", func(t *testing.T) {
		f := newCatchUpPushFixture(t)
		sibling := f.h.PairRecordsSibling("alpha")
		below := gitkit.CommitFile(t, sibling, "below.txt", "below\n", "records below the hold")
		held := gitkit.CommitFile(t, sibling, "held.txt", "held\n", "held records")
		err := pushledger.Update(f.h.Location.HubPath, pushledger.LandingKey, time.Unix(1, 0), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
			landing := pushledger.Landing{Run: "alpha", Tip: held, RecordsCommit: held, ParentBranch: "main", At: time.Unix(1, 0)}
			return pushledger.Entry{Landings: []pushledger.Landing{landing}}, true, nil
		})
		if err != nil {
			t.Fatalf("pushledger.Update() = %v; want nil", err)
		}

		if err := f.push("alpha"); err != nil {
			t.Fatalf("pushOwedForCatchUp() = %v; want nil", err)
		}
		if got := f.startedSlugs(); !slices.Equal(got, []string{"alpha"}) {
			t.Errorf("started = %q; want the pair", got)
		}
		if got := recordsTip(t, f.h, f.branch); got != below {
			t.Errorf("origin's records tip of alpha = %s; want it short of the held commit at %s", got, below)
		}
	})

	t.Run("A_run_with_no_pair_here_pushes_the_primes_records_alone", func(t *testing.T) {
		f := newCatchUpPushFixture(t)
		records := f.h.PrimeRecords()
		head := gitkit.CommitFile(t, records, "prime.txt", "prime\n", "prime transition")
		pairTip := recordsTip(t, f.h, f.branch)
		gitkit.CommitFile(t, f.h.PairRecordsSibling("alpha"), "alpha.txt", "alpha\n", "alpha transition")

		if err := f.push("ghost"); err != nil {
			t.Fatalf("pushOwedForCatchUp() = %v; want nil", err)
		}
		if got := recordsTip(t, f.h, gitkit.CurrentBranch(t, records)); got != head {
			t.Errorf("origin's records tip of the prime = %s; want %s", got, head)
		}
		if got := recordsTip(t, f.h, f.branch); got != pairTip {
			t.Errorf("origin's records tip of alpha = %s; want %s unchanged", got, pairTip)
		}
	})
}
