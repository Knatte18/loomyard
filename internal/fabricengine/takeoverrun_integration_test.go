// takeoverrun_integration_test.go covers the takeover writer.
// TakeOverRun moves a run's pair onto origin and pushes the takeover entry on the pair's records branch and the parent's.
// OriginParentRunStatus reads a run's record at a parent's upstream tip.
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
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const takeoverRunSlug = "moved-run"

var takeoverAt = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

// takeoverFixture is a pair whose run records sit on its own records branch and on the prime's, both pushed.
type takeoverFixture struct {
	*pairFixture
	statusRel   string
	primeWeft   string
	primeBranch string
}

func newTakeoverFixture(t *testing.T) *takeoverFixture {
	t.Helper()
	p := newPairFixture(t, takeoverRunSlug)
	_, statusRel, err := shedrun.TreeRunFileRels(takeoverRunSlug)
	if err != nil {
		t.Fatalf("TreeRunFileRels(%q): %v", takeoverRunSlug, err)
	}
	status := statusJSON("Implementer")
	gitkit.CommitFile(t, p.weftPath, statusRel, status, "pair status")
	gitkit.Git(t, p.weftPath, "push", "origin", p.weftBranch)
	primeWeft, primeBranch := p.primeWeft()
	gitkit.CommitFile(t, primeWeft, statusRel, status, "prime status")
	gitkit.Git(t, primeWeft, "push", "origin", primeBranch)
	return &takeoverFixture{pairFixture: p, statusRel: statusRel, primeWeft: primeWeft, primeBranch: primeBranch}
}

func (f *takeoverFixture) request() fabricengine.TakeOverRequest {
	return fabricengine.TakeOverRequest{Slug: takeoverRunSlug, Subject: "beam", Machine: "oslo", Pair: f.location, At: takeoverAt}
}

func (f *takeoverFixture) wantEntry() shedengine.Takeover {
	return shedengine.Takeover{Machine: "oslo", Subject: "beam", At: "2026-11-17T10:00:00Z"}
}

// originTakeover fetches the records worktree and reads the takeover entry at its upstream tip.
func (f *takeoverFixture) originTakeover(t *testing.T, worktree string) (shedengine.Takeover, bool) {
	t.Helper()
	gitkit.Git(t, worktree, "fetch", "origin")
	entry, found, err := fabricengine.ReadUpstreamTakeover(f.hub.Location, worktree, takeoverRunSlug)
	if err != nil {
		t.Fatalf("ReadUpstreamTakeover(%s) error = %v; want nil", worktree, err)
	}
	return entry, found
}

// requireNoEntryOnOrigin fails the test when origin's tip of either records branch carries a takeover entry.
func (f *takeoverFixture) requireNoEntryOnOrigin(t *testing.T, worktrees ...string) {
	t.Helper()
	for _, worktree := range worktrees {
		if entry, found := f.originTakeover(t, worktree); found {
			t.Errorf("origin's records branch of %s holds the takeover entry %+v; want none", worktree, entry)
		}
	}
}

// TestTakeOverRun drives the takeover over a pair and the prime.
// Not parallel: the rows set takeoverBeforePushHook, a package variable.
// No parallel test runs while a serial one does.
func TestTakeOverRun(t *testing.T) {
	tiergate.Require(t, 1)

	t.Run("WritesTheEntryOnBothRecordsBranchesAndLands", func(t *testing.T) {
		f := newTakeoverFixture(t)
		writeHandOverMark(t, f.hub.Location.HubPath, takeoverRunSlug, fabricengine.HandOverMark{Machine: "paris", At: "2026-11-16T10:00:00Z", Written: time.Now()})

		res, err := fabricengine.TakeOverRun(f.hub.Location, f.request())
		if err != nil {
			t.Fatalf("TakeOverRun() error = %v; want nil", err)
		}

		for name, worktree := range map[string]string{"pair": f.weftPath, "prime": f.primeWeft} {
			entry, found := f.originTakeover(t, worktree)
			if !found || entry != f.wantEntry() {
				t.Errorf("origin's %s records branch entry = %+v, found %v; want %+v", name, entry, found, f.wantEntry())
			}
		}
		if res.Retries != 0 {
			t.Errorf("TakeOverRun() Retries = %d; want 0", res.Retries)
		}
		if _, found, err := fabricengine.ReadHandOverMark(f.hub.Location.HubPath, takeoverRunSlug); err != nil || found {
			t.Errorf("ReadHandOverMark() = found %v, %v; want the earlier mark dropped", found, err)
		}
	})

	t.Run("APauseHoldsTheTakeoverWithoutAnyTeardownOrPushLockAndTheEntryStillLands", func(t *testing.T) {
		f := newTakeoverFixture(t)
		lockPaths := []string{
			fabricengine.TeardownLockPath(f.hub.Location.HubPath, takeoverRunSlug),
			// The takeover's parent is the prime, whose teardown lock is named root.
			fabricengine.TeardownLockPath(f.hub.Location.HubPath, "root"),
		}
		for _, weft := range []string{f.weftPath, f.primeWeft} {
			lockPaths = append(lockPaths, filepath.Join(fabricengine.RecordsLockDirPath(weft), fabricengine.WeftPushLockFileForTest))
		}
		waits := fabricengine.PassingPauseGateForTest(t, func() {
			for _, path := range lockPaths {
				if !fabricengine.LockFreeForTest(t, path) {
					t.Errorf("%s was held while the pause held the takeover; want it free", path)
				}
			}
		})

		if _, err := fabricengine.TakeOverRun(f.hub.Location, f.request()); err != nil {
			t.Fatalf("TakeOverRun() error = %v; want nil", err)
		}

		if *waits == 0 {
			t.Errorf("the pause held no call of the takeover; want at least one")
		}
		if entry, found := f.originTakeover(t, f.weftPath); !found || entry != f.wantEntry() {
			t.Errorf("origin's pair records branch entry = %+v, found %v; want %+v", entry, found, f.wantEntry())
		}
	})

	t.Run("ATakeoverPushRejectedOverTheOldOwnersLaterTransitionWritesOntoOriginsTipAndLands", func(t *testing.T) {
		f := newTakeoverFixture(t)
		planted := false
		fabricengine.SetTakeoverBeforePushHookForTest(t, func(attempt int) {
			if attempt == 1 && !planted {
				planted = true
				f.originCommit(f.weftBranch, "old-owner.md", "old\n", "old owner transition")
			}
		})

		res, err := fabricengine.TakeOverRun(f.hub.Location, f.request())
		if err != nil {
			t.Fatalf("TakeOverRun() error = %v; want nil", err)
		}

		if res.Retries != 1 {
			t.Errorf("TakeOverRun() Retries = %d; want 1", res.Retries)
		}
		subjects := strings.Split(gitkit.Git(t, f.weftPath, "log", "--format=%s", "-2"), "\n")
		if len(subjects) != 2 || subjects[0] != "takeover of moved-run by oslo" || subjects[1] != "old owner transition" {
			t.Errorf("local records log = %q; want the entry on top of the old owner's transition", subjects)
		}
		if got, want := bareBranchTip(t, f.hub.RecordsBare, f.weftBranch), gitkit.RevParse(t, f.weftPath, "HEAD"); got != want {
			t.Errorf("origin records tip = %s; want the local tip %s", got, want)
		}
	})

	t.Run("ARetryOriginKeepsRejectingReturnsThePushError", func(t *testing.T) {
		f := newTakeoverFixture(t)
		attempts := 0
		fabricengine.SetTakeoverBeforePushHookForTest(t, func(int) {
			attempts++
			f.originCommit(f.weftBranch, "old-owner-"+strings.Repeat("x", attempts)+".md", "old\n", "old owner transition")
		})
		req := f.request()
		req.Delays = []time.Duration{0, 0}

		res, err := fabricengine.TakeOverRun(f.hub.Location, req)

		if !errors.Is(err, gitrepo.ErrPushRejected) || !strings.Contains(err.Error(), takeoverRunSlug) || !strings.Contains(err.Error(), "kept moving") {
			t.Fatalf("TakeOverRun() error = %v; want the push rejection naming the run and that origin kept moving", err)
		}
		if attempts != 3 || res.Retries != 2 {
			t.Errorf("TakeOverRun() made %d attempts and %d retries; want 3 and 2", attempts, res.Retries)
		}
		f.requireNoEntryOnOrigin(t, f.weftPath)
		if subject := gitkit.Git(t, f.weftPath, "log", "--format=%s", "-1"); strings.HasPrefix(subject, "takeover of") {
			t.Errorf("local records tip subject = %q; want no takeover entry commit left", subject)
		}
	})

	t.Run("ALocalTipOriginLacksIsArchivedAndPushedBeforeTheReset", func(t *testing.T) {
		f := newTakeoverFixture(t)
		codeTip := gitkit.CommitFile(t, f.warpPath, "local-code.txt", "code\n", "local code")
		recordsTip := gitkit.CommitFile(t, f.weftPath, "notes/local.md", "records\n", "local records")

		res, err := fabricengine.TakeOverRun(f.hub.Location, f.request())
		if err != nil {
			t.Fatalf("TakeOverRun() error = %v; want nil", err)
		}

		for _, tip := range []struct{ tag, bare string }{
			{fabricengine.OwnerArchiveTagName(takeoverRunSlug, codeTip), f.hub.CodeBare},
			{fabricengine.OwnerArchiveTagName(takeoverRunSlug, recordsTip), f.hub.RecordsBare},
		} {
			if !strings.Contains(strings.Join(res.Tags, " "), tip.tag) || gitkit.Git(t, tip.bare, "tag", "--list", tip.tag) == "" {
				t.Errorf("tag %s: Tags %v, on origin %q; want it returned and pushed", tip.tag, res.Tags, gitkit.Git(t, tip.bare, "tag", "--list", tip.tag))
			}
		}
		if got, want := gitkit.RevParse(t, f.warpPath, "HEAD"), bareBranchTip(t, f.hub.CodeBare, f.warpBranch); got != want {
			t.Errorf("code HEAD = %s; want origin's tip %s, the local commit moved off", got, want)
		}
	})

	t.Run("ARebaseThatConflictsIsAbortedLeavesTheBranchAsItWasAndNamesTheRun", func(t *testing.T) {
		f := newTakeoverFixture(t)
		localCommit := gitkit.CommitFile(t, f.primeWeft, f.statusRel, statusJSON("Local"), "local status")
		firstAttempts := 0
		fabricengine.SetTakeoverBeforePushHookForTest(t, func(attempt int) {
			if attempt != 1 {
				return
			}
			firstAttempts++
			if firstAttempts == 2 {
				f.originCommit(f.primeBranch, f.statusRel, statusJSON("Old"), "old owner later transition")
			}
		})

		_, err := fabricengine.TakeOverRun(f.hub.Location, f.request())

		if !errors.Is(err, fabricengine.ErrReplayConflict) || !strings.Contains(err.Error(), takeoverRunSlug) {
			t.Fatalf("TakeOverRun() error = %v; want a replay conflict naming the run", err)
		}
		if got := gitkit.RevParse(t, f.primeWeft, "HEAD"); got != localCommit {
			t.Errorf("prime records HEAD = %s; want the local commit %s, the entry commit dropped", got, localCommit)
		}
		if status := gitkit.Git(t, f.primeWeft, "status"); strings.Contains(status, "cherry-pick") || strings.Contains(status, "rebase") {
			t.Errorf("prime records status = %q; want no replay in progress", status)
		}
		if ahead := gitkit.Git(t, f.primeWeft, "log", "--format=%s", "origin/"+f.primeBranch+"..HEAD"); ahead != "local status" {
			t.Errorf("prime records commits ahead of origin = %q; want only the local commit, with no takeover entry", ahead)
		}
		f.requireNoEntryOnOrigin(t, f.primeWeft)
	})

	t.Run("AParentRecordsCommitOriginLacksIsArchivedAndKeptAcrossTheRetry", func(t *testing.T) {
		f := newTakeoverFixture(t)
		otherRun := gitkit.CommitFile(t, f.primeWeft, "notes/other-run.md", "other\n", "other run commit")
		firstAttempts := 0
		fabricengine.SetTakeoverBeforePushHookForTest(t, func(attempt int) {
			if attempt != 1 {
				return
			}
			firstAttempts++
			if firstAttempts == 2 {
				f.originCommit(f.primeBranch, "old-owner.md", "old\n", "old owner transition")
			}
		})

		res, err := fabricengine.TakeOverRun(f.hub.Location, f.request())
		if err != nil {
			t.Fatalf("TakeOverRun() error = %v; want nil", err)
		}

		tag := fabricengine.OwnerArchiveTagName("root", otherRun)
		if !strings.Contains(strings.Join(res.Tags, " "), tag) || gitkit.Git(t, f.hub.RecordsBare, "tag", "--list", tag) == "" {
			t.Errorf("tag %s: Tags %v; want the other run's commit archived and pushed", tag, res.Tags)
		}
		history := gitkit.Git(t, f.hub.RecordsBare, "log", "--format=%s", f.primeBranch)
		for _, subject := range []string{"takeover of moved-run by oslo", "other run commit", "old owner transition"} {
			if !strings.Contains(history, subject) {
				t.Errorf("origin's prime records history = %q; want it to hold %q", history, subject)
			}
		}
	})

	t.Run("AnAnchorChangeOnTheWeftRefusesBeforeAnyEntryIsWritten", func(t *testing.T) {
		f := newTakeoverFixture(t)
		gitkit.CommitFile(t, f.weftPath, lymcwd.AnchorFileName, "sub\n", "re-anchor")
		before := bareBranchTip(t, f.hub.RecordsBare, f.weftBranch)

		_, err := fabricengine.TakeOverRun(f.hub.Location, f.request())

		if !errors.Is(err, fabricengine.ErrAnchorChange) {
			t.Fatalf("TakeOverRun() error = %v; want it to wrap ErrAnchorChange", err)
		}
		if after := bareBranchTip(t, f.hub.RecordsBare, f.weftBranch); after != before {
			t.Errorf("origin records tip = %s; want it unchanged at %s", after, before)
		}
		f.requireNoEntryOnOrigin(t, f.weftPath, f.primeWeft)
	})

	t.Run("APairOrParentTeardownLockHeldElsewhereRefusesBeforeAnyEntryIsWritten", func(t *testing.T) {
		for _, held := range []string{takeoverRunSlug, "root"} {
			f := newTakeoverFixture(t)
			holdLock(t, fabricengine.TeardownLockPath(f.hub.Location.HubPath, held))
			pairTip := bareBranchTip(t, f.hub.RecordsBare, f.weftBranch)
			primeTip := bareBranchTip(t, f.hub.RecordsBare, f.primeBranch)
			localTips := []string{gitkit.RevParse(t, f.weftPath, "HEAD"), gitkit.RevParse(t, f.primeWeft, "HEAD")}
			req := f.request()
			req.LockWait = 20 * time.Millisecond

			_, err := fabricengine.TakeOverRun(f.hub.Location, req)

			if !errors.Is(err, fabricengine.ErrPairPushRunning) || !strings.Contains(err.Error(), takeoverRunSlug) {
				t.Errorf("TakeOverRun() with %q held: error = %v; want a refusal wrapping ErrPairPushRunning that names the run", held, err)
			}
			if bareBranchTip(t, f.hub.RecordsBare, f.weftBranch) != pairTip || bareBranchTip(t, f.hub.RecordsBare, f.primeBranch) != primeTip {
				t.Errorf("TakeOverRun() with %q held moved an origin records tip; want both unchanged", held)
			}
			if gitkit.RevParse(t, f.weftPath, "HEAD") != localTips[0] || gitkit.RevParse(t, f.primeWeft, "HEAD") != localTips[1] {
				t.Errorf("TakeOverRun() with %q held moved a local records tip; want both unchanged", held)
			}
		}
	})

	t.Run("TheLocalBranchesEndAtOriginsTips", func(t *testing.T) {
		f := newTakeoverFixture(t)
		originCommitFile(t, f.hub.CodeBare, f.warpBranch, "moved.txt", "moved\n")

		if _, err := fabricengine.TakeOverRun(f.hub.Location, f.request()); err != nil {
			t.Fatalf("TakeOverRun() error = %v; want nil", err)
		}

		for name, tip := range map[string][2]string{
			"pair code":     {gitkit.RevParse(t, f.warpPath, "HEAD"), bareBranchTip(t, f.hub.CodeBare, f.warpBranch)},
			"pair records":  {gitkit.RevParse(t, f.weftPath, "HEAD"), bareBranchTip(t, f.hub.RecordsBare, f.weftBranch)},
			"prime records": {gitkit.RevParse(t, f.primeWeft, "HEAD"), bareBranchTip(t, f.hub.RecordsBare, f.primeBranch)},
		} {
			if tip[0] != tip[1] {
				t.Errorf("%s HEAD = %s; want origin's tip %s", name, tip[0], tip[1])
			}
		}
	})
}

// TestDropHandOverMark covers the hand-over mark's drop: an earlier mark goes, and none is no error.
func TestDropHandOverMark(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("AHandOverMarkOfTheSlugIsDropped", func(t *testing.T) {
		t.Parallel()
		hub := newOriginHub(t).Location.HubPath
		writeHandOverMark(t, hub, takeoverRunSlug, fabricengine.HandOverMark{Machine: "paris", At: "2026-11-16T10:00:00Z", Written: time.Now()})

		if err := fabricengine.DropHandOverMark(hub, takeoverRunSlug); err != nil {
			t.Fatalf("DropHandOverMark() error = %v; want nil", err)
		}
		if _, found, err := fabricengine.ReadHandOverMark(hub, takeoverRunSlug); err != nil || found {
			t.Errorf("ReadHandOverMark() = found %v, %v; want the mark gone", found, err)
		}
	})

	t.Run("NoHandOverMarkIsNoError", func(t *testing.T) {
		t.Parallel()
		if err := fabricengine.DropHandOverMark(t.TempDir(), takeoverRunSlug); err != nil {
			t.Errorf("DropHandOverMark() error = %v; want nil", err)
		}
	})
}

// TestOriginRunStatus covers OriginParentRunStatus over the prime's records branch.
func TestOriginRunStatus(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	entry := shedengine.Takeover{Machine: "oslo", Subject: "sub", At: "2026-11-17T10:00:00Z"}

	t.Run("ReadsTheRunRecordAtTheParentsUpstreamTipWithItsTakeover", func(t *testing.T) {
		t.Parallel()
		f, _ := newFailureFixture(t)
		_, branch := f.primeWeft()
		plantTakeover(t, f, branch, takeoverRunSlug, entry)

		status, found, err := fabricengine.OriginParentRunStatus(f.hub.Location, takeoverRunSlug, "")

		if err != nil || !found || status.Takeover == nil || *status.Takeover != entry {
			t.Errorf("OriginParentRunStatus() = %+v, found %v, %v; want the record with the takeover %+v", status, found, err, entry)
		}
	})

	t.Run("ABranchWithNoUpstreamReadsNotFound", func(t *testing.T) {
		t.Parallel()
		f, _ := newFailureFixture(t)

		if _, found, err := fabricengine.OriginParentRunStatus(f.hub.Location, takeoverRunSlug, ""); err != nil || found {
			t.Errorf("OriginParentRunStatus() = found %v, %v; want not found and no error", found, err)
		}
	})

	t.Run("ATipWithNoRecordReadsNotFound", func(t *testing.T) {
		t.Parallel()
		f, _ := newFailureFixture(t)
		_, branch := f.primeWeft()
		f.originCommit(branch, "other.txt", "other\n", "another run's commit")

		if _, found, err := fabricengine.OriginParentRunStatus(f.hub.Location, takeoverRunSlug, ""); err != nil || found {
			t.Errorf("OriginParentRunStatus() = found %v, %v; want not found and no error", found, err)
		}
	})
}
