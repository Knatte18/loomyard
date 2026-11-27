// parentpush_integration_test.go drives the parent step of the pusher over a hubforge hub with bare origins.
// It goes through the pusher verb's seams: fabric's parent steps, the board's done write and the refused-push records.

package loomcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// errPassWaits ends a pass at the point where the pusher would wait.
var errPassWaits = errors.New("the pusher waits")

// otherMachine is a machine name that is not this one.
const otherMachine = "another-machine"

// parentFixture is a hub whose prime code branch is on origin, with the runs that land on it.
type parentFixture struct {
	t      *testing.T
	h      *hubforge.Hub
	branch string
	// branches maps each run added to its pair's code branch.
	branches map[string]string
}

func newParentFixture(t *testing.T) *parentFixture {
	t.Helper()
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.Git(t, h.PrimeWorktree(), "push", "-q", "-u", "origin", "HEAD")
	return &parentFixture{t: t, h: h, branch: gitkit.CurrentBranch(t, h.PrimeWorktree()), branches: map[string]string{}}
}

// board returns the hub's board.
func (f *parentFixture) board() *boardengine.Board {
	f.t.Helper()
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: f.h.BoardDir()}, "board")
	if err != nil {
		f.t.Fatalf("load the board config: %v", err)
	}
	cfg.Path = f.h.BoardDir()
	return boardengine.New(cfg)
}

// addRun adds the pair of slug under parent, "" for the prime, with its board entry.
func (f *parentFixture) addRun(slug, parent string, beam bool) {
	f.t.Helper()
	added := hubforge.AddPairWith(f.t, f.h, slug, fabricengine.AddOptions{Parent: parent, Beam: beam})
	f.branches[slug] = added.Branch
	entry := map[string]any{"slug": slug, "kind": "task", "title": slug, "labels": []string{"bug"}}
	if beam {
		entry["kind"] = "project"
	}
	if parent != "" {
		entry["project"] = parent
	}
	board := f.board()
	if _, err := board.UpsertTask(entry); err != nil {
		f.t.Fatalf("seed the board entry %s: %v", slug, err)
	}
	if err := board.PusherSync(); err != nil {
		f.t.Fatalf("push the board: %v", err)
	}
	if beam {
		machine, err := machinename.Name()
		if err != nil {
			f.t.Fatalf("machine name: %v", err)
		}
		f.setOwners(boardengine.OwnerRequest{Machine: machine, Slugs: []string{slug}, Beam: true})
	}
}

func (f *parentFixture) setOwners(request boardengine.OwnerRequest) {
	f.t.Helper()
	if _, err := boardengine.SetOwners(f.h.BoardDir(), request); err != nil {
		f.t.Fatalf("SetOwners(%+v): %v", request, err)
	}
}

func (f *parentFixture) location(slug string) *lymcwd.Location {
	f.t.Helper()
	location, err := lymcwd.ResolveWorktree(f.h.PairCodeWorktree(slug))
	if err != nil {
		f.t.Fatalf("ResolveWorktree(%s): %v", slug, err)
	}
	return location
}

// merge commits the work of slug on its pair and lands it on the checkout at parentDir, returning the landing commit.
func (f *parentFixture) merge(slug, parentDir string) string {
	f.t.Helper()
	pairDir := f.h.PairCodeWorktree(slug)
	if _, err := os.Stat(filepath.Join(pairDir, slug+".txt")); err != nil {
		gitkit.CommitFile(f.t, pairDir, slug+".txt", slug+"\n", "the work of "+slug)
	}
	gitkit.Git(f.t, parentDir, "merge", "--no-ff", "-q", "-m", "land "+slug+"\n\n"+fabriclayout.LandingTrailerKey+": "+slug, f.branches[slug])
	return gitkit.RevParse(f.t, parentDir, "HEAD")
}

// land merges slug onto the parent at target and records the landing with its records commit, which it returns.
func (f *parentFixture) land(slug string, target runpusher.Target) (tip, records string) {
	f.t.Helper()
	tip = f.merge(slug, target.Location.WorktreePath())
	records = gitkit.CommitFile(f.t, f.h.PairRecordsSibling(slug), "finalize.txt", slug+"\n", "finalize records")
	landing := pushledger.Landing{Run: slug, Tip: tip, RecordsCommit: records, ParentBranch: f.branch, At: time.Now()}
	if err := runpusher.RecordLanding(target, landing, time.Now()); err != nil {
		f.t.Fatalf("RecordLanding(%s): %v", slug, err)
	}
	return tip, records
}

// clone returns a fresh clone of branch of the bare repository, ready to commit in.
func (f *parentFixture) clone(bare, branch string) string {
	f.t.Helper()
	clone := filepath.Join(f.t.TempDir(), "clone")
	gitkit.MustRun(f.t, filepath.Dir(clone), "git", "clone", "-q", "-b", branch, bare, clone)
	gitkit.Git(f.t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(f.t, clone, "config", "user.name", "Test")
	return clone
}

// landUnderAnotherCommit puts the work of slug on origin's branch through a commit of another clone.
func (f *parentFixture) landUnderAnotherCommit(slug, branch string) {
	f.t.Helper()
	clone := f.clone(f.h.CodeBare, branch)
	gitkit.CommitFile(f.t, clone, slug+".txt", slug+"\n", "the work of "+slug+" landed elsewhere")
	gitkit.Git(f.t, clone, "push", "-q")
}

// takeOverOnOriginOnly records another machine as the owner of the beam slug on origin's board, leaving the local board.
func (f *parentFixture) takeOverOnOriginOnly(slug string) {
	f.t.Helper()
	url := strings.TrimSpace(gitkit.Git(f.t, f.h.BoardDir(), "config", "--get", "remote.origin.url"))
	clone := f.clone(url, gitkit.CurrentBranch(f.t, f.h.BoardDir()))
	request := boardengine.OwnerRequest{Machine: otherMachine, Slugs: []string{slug}, Beam: true, Takeover: true}
	if _, err := boardengine.SetOwners(clone, request); err != nil {
		f.t.Fatalf("SetOwners(%+v) on another clone: %v", request, err)
	}
}

// requireHandedOver fails unless the pair of slug is gone and its landing is off the target.
// It also requires that no done write was made and that no origin branch holds the run's records commit.
func (f *parentFixture) requireHandedOver(slug, records string) {
	f.t.Helper()
	if _, err := os.Stat(f.h.PairCodeWorktree(slug)); !errors.Is(err, os.ErrNotExist) {
		f.t.Errorf("stat the pair of %s = %v; want it removed by the hand-over", slug, err)
	}
	entry := f.entry(pushledger.LandingKey)
	if f.done(slug) || len(entry.Landings) != 0 || len(entry.HandOvers) != 0 {
		f.t.Errorf("done = %v, landing entry = %+v; want no done write and nothing owed", f.done(slug), entry)
	}
	if refs := strings.TrimSpace(gitkit.Git(f.t, f.h.RecordsBare, "for-each-ref", "--contains", records, "refs/heads")); refs != "" {
		f.t.Errorf("origin branches holding the held records commit = %q; want none", refs)
	}
}

func (f *parentFixture) deps(target runpusher.Target) runpusher.Deps {
	f.t.Helper()
	check := prePushCheck{skip: func() (bool, error) { return true, nil }}
	deps := pusherDeps(f.h.Location, target, check, func(prime, l *lymcwd.Location, slug string, push pairteardown.RunPush) error {
		_, err := pairteardown.PushRun(prime, l, slug, push)
		return err
	})
	deps.Wait = func(context.Context, time.Duration, <-chan struct{}) error { return errPassWaits }
	return deps
}

// pass runs the pusher of target with the push-now mark until it would wait or stops.
func (f *parentFixture) pass(target runpusher.Target) {
	f.t.Helper()
	deps := f.deps(target)
	if err := markPushNow(deps, target); err != nil {
		f.t.Fatalf("mark the push now: %v", err)
	}
	if err := runpusher.Run(context.Background(), target, deps); err != nil && !errors.Is(err, errPassWaits) {
		f.t.Fatalf("Run() = %v; want nil or a wait", err)
	}
}

func (f *parentFixture) settleRefused(target runpusher.Target) {
	f.t.Helper()
	if err := runpusher.SettleRefused(context.Background(), target, f.deps(target)); err != nil {
		f.t.Fatalf("SettleRefused() = %v; want nil", err)
	}
}

func (f *parentFixture) done(slug string) bool {
	f.t.Helper()
	tasks, err := boardengine.ReadTasks(f.h.BoardDir())
	if err != nil {
		f.t.Fatalf("ReadTasks: %v", err)
	}
	return slices.ContainsFunc(tasks, func(task boardengine.Task) bool {
		return task.Slug == slug && task.Status != nil && *task.Status == boardengine.StatusDone
	})
}

func (f *parentFixture) entry(key pushledger.Key) pushledger.Entry {
	f.t.Helper()
	entry, _, err := pushledger.Read(f.h.Path, key)
	if err != nil {
		f.t.Fatalf("pushledger.Read(%s): %v", key, err)
	}
	return entry
}

func (f *parentFixture) pairEntry(slug string) pushledger.Entry {
	f.t.Helper()
	return f.entry(pushledger.RecordsKey(f.h.PairRecordsSibling(slug)))
}

func (f *parentFixture) originTip(branch string) string {
	f.t.Helper()
	return gitkit.RevParse(f.t, f.h.CodeBare, "refs/heads/"+branch)
}

// requireRefusal fails unless the hub holds one held-back record whose way forward holds each of wants.
// With no wants, it fails unless the hub holds no record.
func (f *parentFixture) requireRefusal(wants ...string) {
	f.t.Helper()
	records, err := fabricengine.ListFailedPushes(f.h.Path)
	if err != nil {
		f.t.Fatalf("ListFailedPushes: %v", err)
	}
	if len(wants) == 0 {
		if len(records) != 0 {
			f.t.Errorf("failed-push records = %+v; want none", records)
		}
		return
	}
	if len(records) != 1 || records[0].Class != fabriclayout.PushFailureHeldBack {
		f.t.Fatalf("failed-push records = %+v; want one held-back record", records)
	}
	for _, want := range wants {
		if got := fabricengine.FailedPushWayForward(records[0]); !strings.Contains(got, want) {
			f.t.Errorf("way forward = %q; want it to hold %q", got, want)
		}
	}
}

func TestParentPush(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("TwoLandingsSettleOnceTheNewestLands", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.addRun("two", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		_, oneRecords := f.land("one", target)
		tip, _ := f.land("two", target)
		originURL := strings.TrimSpace(gitkit.Git(t, f.h.PrimeWorktree(), "config", "--get", "remote.origin.url"))

		gitkit.Git(t, f.h.PrimeWorktree(), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing"))
		f.pass(target)
		f.requireRefusal()
		if held, _, err := pushledger.HeldRecordsCommit(f.h.Path, "one"); err != nil || held != oneRecords || f.done("one") || f.done("two") {
			t.Fatalf("with origin unreachable: held = %q, %v, done = %v, %v; want %s held and neither done", held, err, f.done("one"), f.done("two"), oneRecords)
		}
		gitkit.Git(t, f.h.PrimeWorktree(), "remote", "set-url", "origin", originURL)

		f.pass(target)
		if got := f.originTip(f.branch); got != tip {
			t.Errorf("origin tip = %s; want the newest landing %s", got, tip)
		}
		if !f.done("one") || !f.done("two") || len(f.entry(pushledger.LandingKey).Landings) != 0 {
			t.Errorf("done = %v, %v, landings = %+v; want both done and none standing", f.done("one"), f.done("two"), f.entry(pushledger.LandingKey).Landings)
		}
		if _, held, err := pushledger.HeldRecordsCommit(f.h.Path, "one"); err != nil || held || !f.pairEntry("one").PushNow {
			t.Errorf("after the settle: held = %v, %v, pair entry = %+v; want the hold released and the pair marked", held, err, f.pairEntry("one"))
		}
		f.requireRefusal()
	})

	t.Run("APrimeOwnedElsewhereRefusesUntilAdoptedAndPushed", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		before := f.originTip(f.branch)
		tip, _ := f.land("one", target)
		f.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Prime: true, Takeover: true})

		f.pass(target)
		f.requireRefusal("lym adopt root", "`git push` in "+f.h.PrimeWorktree())
		if got := f.originTip(f.branch); got != before || f.done("one") {
			t.Errorf("origin tip = %s, done = %v; want nothing pushed and no done write", got, f.done("one"))
		}

		machine, err := machinename.Name()
		if err != nil {
			t.Fatalf("machine name: %v", err)
		}
		f.setOwners(boardengine.OwnerRequest{Machine: machine, Prime: true, Takeover: true})
		gitkit.Git(t, f.h.PrimeWorktree(), "push", "-q")
		f.settleRefused(target)
		if got := f.originTip(f.branch); got != tip || !f.done("one") {
			t.Errorf("origin tip = %s, done = %v; want %s and the done write", got, f.done("one"), tip)
		}
		f.requireRefusal()
	})

	t.Run("ADroppedLandingAloneResetsTheParentAndKeepsItsRecordsHeld", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		_, records := f.land("one", target)
		f.landUnderAnotherCommit("one", f.branch)

		f.pass(target)
		entry := f.entry(pushledger.LandingKey)
		pair := f.pairEntry("one")
		if len(entry.Landings) != 0 || f.done("one") || pair.HeldRecords != records || pair.Slug != "one" || pair.Worktree != f.h.PairCodeWorktree("one") {
			t.Errorf("landing entry = %+v, pair entry = %+v, done = %v; want the landing dropped with its records held", entry, pair, f.done("one"))
		}
		if got, want := gitkit.RevParse(t, f.h.PrimeWorktree(), "HEAD"), gitkit.RevParse(t, f.h.PrimeWorktree(), "@{u}"); got != want {
			t.Errorf("prime head = %s; want origin's tip %s", got, want)
		}
		f.requireRefusal()

		if err := os.RemoveAll(filepath.Dir(pushledger.EntryPath(f.h.Path, pushledger.LandingKey))); err != nil {
			t.Fatalf("delete the ledger: %v", err)
		}
		recordsRef := "refs/heads/" + fabriclayout.RecordsBranchName(f.branches["one"])
		if _, err := fabricengine.PushPairAnchored(f.location("one"), fabricengine.SyncOptions{}, fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded}); err != nil {
			t.Fatalf("PushPairAnchored() = %v; want nil", err)
		}
		if got := gitkit.RevParse(t, f.h.RecordsBare, recordsRef); got == records {
			t.Errorf("origin records tip = %s; want the held commit kept off origin with the ledger gone", got)
		}
	})

	t.Run("ADroppedLandingUnderANewerOneRefusesUntilRebased", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.addRun("two", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		f.land("one", target)
		f.land("two", target)
		f.landUnderAnotherCommit("one", f.branch)
		other := f.originTip(f.branch)

		f.pass(target)
		f.requireRefusal("rebase", "`git push` in "+f.h.PrimeWorktree())
		if got := f.originTip(f.branch); got != other || f.done("two") {
			t.Errorf("origin tip = %s, two done = %v; want nothing pushed", got, f.done("two"))
		}

		gitkit.Git(t, f.h.PrimeWorktree(), "reset", "-q", "--keep", "@{u}")
		tip := f.merge("two", f.h.PrimeWorktree())
		f.settleRefused(target)
		f.requireRefusal()
		gitkit.Git(t, f.h.PrimeWorktree(), "push", "-q")
		f.settleRefused(target)
		if got := f.originTip(f.branch); got != tip || !f.done("two") || f.done("one") {
			t.Errorf("origin tip = %s, done = %v, %v; want %s with only two done", got, f.done("one"), f.done("two"), tip)
		}
	})

	t.Run("ALandingNoEntryRecordsIsRefusedAtFinalizeAndRecordedAgainPastIt", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		tip := f.merge("one", f.h.PrimeWorktree())

		f.pass(target)
		f.requireRefusal(tip, "lym loom resume")
		if err := runpusher.RecordLanding(target, pushledger.Landing{Run: "one", Tip: tip, ParentBranch: f.branch, At: time.Now()}, time.Now()); err != nil {
			t.Fatalf("RecordLanding: %v", err)
		}
		f.settleRefused(target)
		f.requireRefusal()

		if err := os.RemoveAll(filepath.Dir(pushledger.EntryPath(f.h.Path, pushledger.LandingKey))); err != nil {
			t.Fatalf("delete the ledger: %v", err)
		}
		pair := f.location("one")
		if err := state.WriteJSON(shedrun.StatusFile(pair, "one"), shedrun.StatusLock(pair, "one"), shedengine.Status{State: shedengine.StateDone}); err != nil {
			t.Fatalf("write the run status: %v", err)
		}
		if ahead, err := f.deps(target).Ahead(); err != nil || !ahead {
			t.Fatalf("Ahead() over a lost ledger = %v, %v; want true", ahead, err)
		}
		f.pass(target)
		if got := f.originTip(f.branch); got != tip || !f.done("one") {
			t.Errorf("origin tip = %s, done = %v; want %s pushed and the done write", got, f.done("one"), tip)
		}

		// A done run's commit past origin, as a landing with pushes off leaves it, starts no pusher.
		gitkit.Git(t, f.h.PrimeWorktree(), "commit", "-q", "--allow-empty", "-m", "again\n\n"+fabriclayout.LandingTrailerKey+": one")
		if ahead, err := f.deps(target).Ahead(); err != nil || ahead {
			t.Errorf("Ahead() over a done run's commit = %v, %v; want false", ahead, err)
		}
	})

	t.Run("ABeamRunOwnedElsewhereHoldsItsLandingOnThePrime", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("moved", "", true)
		target := runpusher.LandingTarget(f.h.Location)
		before := f.originTip(f.branch)
		f.land("moved", target)
		f.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Slugs: []string{"moved"}, Beam: true, Takeover: true})

		f.pass(target)
		f.requireRefusal("lym adopt moved")
		if got := f.originTip(f.branch); got != before || f.done("moved") {
			t.Errorf("origin tip = %s, done = %v; want nothing pushed and no done write", got, f.done("moved"))
		}
	})

	t.Run("ABeamParentTakesTheLandingOnItsOwnEntry", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("beam", "", true)
		f.addRun("member", "beam", false)
		beam := f.location("beam")
		target, err := runpusher.ParentTarget(f.h.Location, beam)
		if err != nil {
			t.Fatalf("ParentTarget: %v", err)
		}
		tip, _ := f.land("member", target)

		entry := f.pairEntry("beam")
		if len(entry.Landings) != 1 || entry.Slug != "beam" || entry.Worktree != beam.WorktreePath() || len(f.entry(pushledger.LandingKey).Landings) != 0 {
			t.Fatalf("beam entry = %+v; want the landing on the beam's key alone", entry)
		}
		f.pass(target)
		if got := f.originTip(f.branches["beam"]); got != tip || !f.done("member") {
			t.Errorf("origin beam tip = %s, member done = %v; want %s and the done write", got, f.done("member"), tip)
		}
		f.requireRefusal()
	})

	originOnly := []struct {
		name string
		act  func(f *parentFixture, target runpusher.Target)
	}{
		{"APassHandsOverALandedRunTakenOverOnOriginAlone", (*parentFixture).pass},
		{"SettleRefusedHandsOverALandedRunTakenOverOnOriginAlone", (*parentFixture).settleRefused},
	}
	for _, tt := range originOnly {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newParentFixture(t)
			f.addRun("moved", "", true)
			target := runpusher.LandingTarget(f.h.Location)
			_, records := f.land("moved", target)
			gitkit.Git(t, f.h.PrimeWorktree(), "push", "-q")
			f.takeOverOnOriginOnly("moved")

			tt.act(f, target)

			f.requireHandedOver("moved", records)
			f.requireRefusal()
		})
	}

	t.Run("ADroppedRunOwnedElsewhereIsResetAndHandedOver", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("moved", "", true)
		target := runpusher.LandingTarget(f.h.Location)
		_, records := f.land("moved", target)
		f.landUnderAnotherCommit("moved", f.branch)
		f.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Slugs: []string{"moved"}, Beam: true, Takeover: true})

		f.pass(target)

		f.requireHandedOver("moved", records)
		if got, want := gitkit.RevParse(t, f.h.PrimeWorktree(), "HEAD"), f.originTip(f.branch); got != want {
			t.Errorf("prime head = %s; want origin's tip %s", got, want)
		}
		f.requireRefusal()
	})

	t.Run("ARefusalOverARunOwnedElsewhereEndsOnceItsTipIsOnOrigin", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("moved", "", true)
		f.addRun("one", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		_, records := f.land("moved", target)
		f.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Slugs: []string{"moved"}, Beam: true, Takeover: true})
		f.pass(target)
		f.requireRefusal("lym adopt moved")

		gitkit.Git(t, f.h.PrimeWorktree(), "push", "-q")
		f.settleRefused(target)
		f.requireHandedOver("moved", records)
		f.requireRefusal()

		tip, _ := f.land("one", target)
		f.pass(target)
		if got := f.originTip(f.branch); got != tip || !f.done("one") {
			t.Errorf("origin tip = %s, done = %v; want %s pushed and the done write", got, f.done("one"), tip)
		}
	})

	t.Run("AnOlderTipIsPushedWhileANewerRunIsHeldBack", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.addRun("moved", "", true)
		target := runpusher.LandingTarget(f.h.Location)
		older, _ := f.land("one", target)
		f.land("moved", target)
		f.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Slugs: []string{"moved"}, Beam: true, Takeover: true})

		f.pass(target)

		f.requireRefusal("lym adopt moved")
		if got := f.originTip(f.branch); got != older || !f.done("one") || f.done("moved") {
			t.Errorf("origin tip = %s, done = %v, %v; want %s with only the older done", got, f.done("one"), f.done("moved"), older)
		}
	})

	t.Run("ABeamParentIsHeldBackWhole", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("beam", "", true)
		f.addRun("member", "beam", false)
		f.addRun("moved", "beam", true)
		beam := f.location("beam")
		gitkit.Git(t, beam.WorktreePath(), "push", "-q", "-u", "origin", "HEAD")
		before := f.originTip(f.branches["beam"])
		target, err := runpusher.ParentTarget(f.h.Location, beam)
		if err != nil {
			t.Fatalf("ParentTarget: %v", err)
		}
		f.land("member", target)
		f.land("moved", target)
		f.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Slugs: []string{"moved"}, Beam: true, Takeover: true})

		f.pass(target)

		f.requireRefusal("lym adopt moved")
		if got := f.originTip(f.branches["beam"]); got != before || f.done("member") || f.done("moved") {
			t.Errorf("origin beam tip = %s, done = %v, %v; want nothing pushed", got, f.done("member"), f.done("moved"))
		}
	})

	t.Run("ABeamParentsOnlyDroppedLandingIsUnrefusedOnceRebased", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("beam", "", true)
		f.addRun("member", "beam", false)
		beam := f.location("beam")
		gitkit.Git(t, beam.WorktreePath(), "push", "-q", "-u", "origin", "HEAD")
		target, err := runpusher.ParentTarget(f.h.Location, beam)
		if err != nil {
			t.Fatalf("ParentTarget: %v", err)
		}
		f.land("member", target)
		gitkit.CommitFile(t, beam.WorktreePath(), "plain.txt", "plain\n", "a plain commit")
		f.landUnderAnotherCommit("member", f.branches["beam"])
		f.pass(target)
		f.requireRefusal("rebase", "`lym fabric push` in ")

		gitkit.Git(t, beam.WorktreePath(), "reset", "-q", "--keep", "@{u}")
		f.settleRefused(target)
		f.requireRefusal()

		f.addRun("later", "beam", false)
		tip, _ := f.land("later", target)
		f.pass(target)
		if got := f.originTip(f.branches["beam"]); got != tip || !f.done("later") {
			t.Errorf("origin beam tip = %s, later done = %v; want %s and the done write", got, f.done("later"), tip)
		}
	})

	t.Run("APairRemovedAfterADropIsNeitherRecordedNorPushedAgain", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.addRun("two", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		f.land("one", target)
		f.land("two", target)
		f.landUnderAnotherCommit("one", f.branch)
		other := f.originTip(f.branch)
		f.pass(target)

		teardown, err := pairteardown.New(f.h.Location)
		if err != nil {
			t.Fatalf("pairteardown.New: %v", err)
		}
		if _, err := teardown.RemovePair(pairteardown.Request{Slug: "one", Local: true, Force: true}); err != nil {
			t.Fatalf("RemovePair(one): %v", err)
		}
		f.pass(target)

		f.requireRefusal("rebase")
		landings := f.entry(pushledger.LandingKey).Landings
		if got := f.originTip(f.branch); got != other || len(landings) != 1 || landings[0].Run != "two" {
			t.Errorf("origin tip = %s, landings = %+v; want %s and the landing of two alone", got, landings, other)
		}
	})

	t.Run("ARefusedResetIsRecordedAndMadeAgain", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		gitkit.CommitFile(t, f.h.PrimeWorktree(), "shared.txt", "base\n", "a shared file")
		gitkit.Git(t, f.h.PrimeWorktree(), "push", "-q")
		tip, _ := f.land("one", target)
		// Origin changes the shared file too, so the reset to origin has to rewrite it.
		clone := f.clone(f.h.CodeBare, f.branch)
		gitkit.CommitFile(t, clone, "shared.txt", "changed on origin\n", "change the shared file")
		gitkit.CommitFile(t, clone, "one.txt", "one\n", "the work of one landed elsewhere")
		gitkit.Git(t, clone, "push", "-q")
		if err := os.WriteFile(filepath.Join(f.h.PrimeWorktree(), "shared.txt"), []byte("edited\n"), 0o644); err != nil {
			t.Fatalf("edit shared.txt: %v", err)
		}

		f.pass(target)
		f.requireRefusal(tip)

		gitkit.Git(t, f.h.PrimeWorktree(), "checkout", "--", "shared.txt")
		f.settleRefused(target)
		f.requireRefusal()
		if got, want := gitkit.RevParse(t, f.h.PrimeWorktree(), "HEAD"), f.originTip(f.branch); got != want {
			t.Errorf("prime head = %s; want origin's tip %s", got, want)
		}
	})

	t.Run("AFailedHandOverIsRecordedAndMadeAgain", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("moved", "", true)
		target := runpusher.LandingTarget(f.h.Location)
		f.land("moved", target)
		gitkit.Git(t, f.h.PrimeWorktree(), "push", "-q")
		f.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Slugs: []string{"moved"}, Beam: true, Takeover: true})
		var handedOver []string
		handOverErr := errors.New("the pair is busy")
		deps := f.deps(target)
		deps.Parent.HandOver = func(run, owner string) error {
			handedOver = append(handedOver, run+" to "+owner)
			return handOverErr
		}

		if err := runpusher.SettleRefused(context.Background(), target, deps); err != nil {
			t.Fatalf("SettleRefused() = %v; want nil", err)
		}
		f.requireRefusal("hand-over of moved")
		// The owed hand-over alone makes the target ahead, so a pusher runs it once no record stands.
		if ahead, err := deps.Ahead(); err != nil || !ahead {
			t.Errorf("Ahead() with a hand-over owed = %v, %v; want true", ahead, err)
		}

		handOverErr = nil
		if err := runpusher.SettleRefused(context.Background(), target, deps); err != nil {
			t.Fatalf("SettleRefused() = %v; want nil", err)
		}
		f.requireRefusal()
		want := []string{"moved to " + otherMachine, "moved to " + otherMachine}
		if owed := f.entry(pushledger.LandingKey).HandOvers; !slices.Equal(handedOver, want) || len(owed) != 0 {
			t.Errorf("hand-overs = %q, still owed %v; want %q and none owed", handedOver, owed, want)
		}
	})

	t.Run("ALandingCommitRemovedFromTheParentIsNotPushed", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		target := runpusher.LandingTarget(f.h.Location)
		before := f.originTip(f.branch)
		tip, _ := f.land("one", target)
		gitkit.Git(t, f.h.PrimeWorktree(), "reset", "-q", "--keep", "HEAD~1")

		f.pass(target)

		f.requireRefusal("git merge --no-ff " + tip)
		if got := f.originTip(f.branch); got != before || f.done("one") {
			t.Errorf("origin tip = %s, done = %v; want nothing pushed and no done write", got, f.done("one"))
		}
	})
}
