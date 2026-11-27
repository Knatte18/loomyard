// pusherreads_integration_test.go covers the local reads of the pusher.
// They are RunCommitsAhead, RefusedPushStands and TipsOnOrigin.
// Each runs over a hub fixture with a bare origin and reads local refs only.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// commitRecordsNaming commits on the pair's records branch a transition whose Warp-SHA trailer names codeTip.
func commitRecordsNaming(t *testing.T, p *pairFixture, codeTip string) {
	t.Helper()
	gitkit.CommitFile(t, p.weftPath, "transition.txt", codeTip+"\n", "transition\n\n"+fabriclayout.WarpSHATrailerKey+": "+codeTip)
}

// unsetUpstream removes the upstream setting of branch in the checkout at path.
func unsetUpstream(t *testing.T, path, branch string) {
	t.Helper()
	gitkit.Git(t, path, "config", "--unset", "branch."+branch+".remote")
	gitkit.Git(t, path, "config", "--unset", "branch."+branch+".merge")
}

// pruneTrackingRef deletes the tracking ref of branch in the checkout at path and keeps its upstream setting.
func pruneTrackingRef(t *testing.T, path, branch string) {
	t.Helper()
	gitkit.Git(t, path, "update-ref", "-d", "refs/remotes/origin/"+branch)
}

func TestRunCommitsAhead(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// build makes the fixture and returns the worktree to read.
		build func(t *testing.T) *lymcwd.Location
		want  bool
	}{
		{"PrimeRecordsCommitUnpushedReadsAhead", func(t *testing.T) *lymcwd.Location {
			f, _ := newFailureFixture(t)
			weft, _ := f.primeWeft()
			gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
			return f.hub.Location
		}, true},
		{"LevelPrimeReadsLevel", func(t *testing.T) *lymcwd.Location {
			f, _ := newFailureFixture(t)
			f.primeWeft()
			return f.hub.Location
		}, false},
		{"PrimeCodeCommitAloneReadsLevel", func(t *testing.T) *lymcwd.Location {
			f, _ := newFailureFixture(t)
			f.primeWeft()
			gitkit.CommitFile(t, f.hub.PrimeWorktree(), "code.txt", "code\n", "code")
			return f.hub.Location
		}, false},
		{"RecordsBranchWithNoUpstreamAndACommitReadsAhead", func(t *testing.T) *lymcwd.Location {
			f, _ := newFailureFixture(t)
			gitkit.CommitFile(t, f.hub.PrimeRecords(), "local.txt", "local\n", "local")
			return f.hub.Location
		}, true},
		{"RecordsBranchWithItsTrackingRefDeletedReadsLevel", func(t *testing.T) *lymcwd.Location {
			f, _ := newFailureFixture(t)
			weft, branch := f.primeWeft()
			gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
			pruneTrackingRef(t, weft, branch)
			return f.hub.Location
		}, false},
		{"PairCodeBranchHoldingTheNamedTipOriginLacksReadsAhead", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "namedlacked")
			lacked := gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "unpushed code")
			commitRecordsNaming(t, p, lacked)
			gitkit.Git(t, p.weftPath, "push")
			return p.location
		}, true},
		{"PairCodeBranchAheadOnlyPastTheNamedTipReadsLevel", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "pastnamed")
			named := gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "named code")
			gitkit.Git(t, p.warpPath, "push")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp 2\n", "code past the named tip")
			commitRecordsNaming(t, p, named)
			gitkit.Git(t, p.weftPath, "push")
			return p.location
		}, false},
		{"PairCodeBranchWithNoUpstreamHoldingTheNamedTipReadsAhead", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "noupstreamcode")
			unsetUpstream(t, p.warpPath, p.warpBranch)
			commitRecordsNaming(t, p, gitkit.RevParse(t, p.warpPath, "HEAD"))
			gitkit.Git(t, p.weftPath, "push")
			return p.location
		}, true},
		{"PairRecordsTipWithNoWarpSHATrailerCountsNoCodeSide", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "notrailer")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "unpushed code")
			gitkit.CommitFile(t, p.weftPath, "plain.txt", "plain\n", "plain records commit")
			gitkit.Git(t, p.weftPath, "push")
			return p.location
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := tt.build(t)

			got, err := fabricengine.RunCommitsAhead(l)

			if err != nil || got != tt.want {
				t.Errorf("RunCommitsAhead() = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}

func TestRefusedPushStands(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	noSleep := func(time.Duration) {}
	tests := []struct {
		name string
		// build makes the fixture, runs the push that leaves its records, and returns the worktree to read.
		build func(t *testing.T) *lymcwd.Location
		want  bool
	}{
		{"NoRecord", func(t *testing.T) *lymcwd.Location {
			return newPairFixture(t, "norecord").location
		}, false},
		{"NotFastForwardRecordOfTheRecordsBranch", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "conflictrecords")
			p.originCommit(p.weftBranch, "conflict.txt", "origin\n", "origin side")
			gitkit.CommitFile(t, p.weftPath, "conflict.txt", "local\n", "local side")
			_, _ = fabricengine.PushPairAnchored(p.location, p.fake.options(noSleep), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})
			requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabriclayout.PushFailureNotFastForward)
			return p.location
		}, true},
		{"NotFastForwardRecordOfThePairsCodeBranch", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "divergedcoderead")
			clone := cloneBareForTest(t, p.hub.CodeBare, p.warpBranch)
			gitkit.CommitFile(t, clone, "other.txt", "other\n", "other machine")
			gitkit.MustRun(t, clone, "git", "push")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "warp change")
			_, _ = fabricengine.PushPairAnchored(p.location, p.fake.options(noSleep), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})
			requireOneFailedPush(t, p.hub.Location.HubPath, "code", fabriclayout.PushFailureNotFastForward)
			return p.location
		}, true},
		{"OwedClassRecordOfTheRecordsBranchReadsFalse", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "owedrecord")
			gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
			gitkit.Git(t, p.weftPath, "remote", "set-url", "origin", refusedOriginURL)
			_, _ = fabricengine.PushPairAnchored(p.location, p.fake.options(noSleep), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})
			requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabriclayout.PushFailureTransport)
			return p.location
		}, false},
		{"RecordOfAnotherBranchReadsFalse", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "anotherbranch")
			p.originCommit(p.weftBranch, "conflict.txt", "origin\n", "origin side")
			gitkit.CommitFile(t, p.weftPath, "conflict.txt", "local\n", "local side")
			_, _ = fabricengine.PushPairAnchored(p.location, p.fake.options(noSleep), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})
			requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabriclayout.PushFailureNotFastForward)
			return p.hub.Location
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := tt.build(t)

			got, err := fabricengine.RefusedPushStands(l)

			if err != nil || got != tt.want {
				t.Errorf("RefusedPushStands() = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}

func TestTipsOnOrigin(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name  string
		build func(t *testing.T) *lymcwd.Location
		want  bool
	}{
		{"BothTipsPushed", func(t *testing.T) *lymcwd.Location {
			return newPairFixture(t, "bothpushed").location
		}, true},
		{"RecordsCommitUnpushed", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "recordsahead")
			gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
			return p.location
		}, false},
		{"CodeCommitUnpushed", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "codeahead")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "warp")
			return p.location
		}, false},
		{"CodeBranchWithNoUpstream", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "codenoupstream")
			unsetUpstream(t, p.warpPath, p.warpBranch)
			return p.location
		}, false},
		{"CodeBranchWithItsTrackingRefDeletedReadsHeld", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "codepruned")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "warp")
			pruneTrackingRef(t, p.warpPath, p.warpBranch)
			return p.location
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := tt.build(t)

			got, err := fabricengine.TipsOnOrigin(l)

			if err != nil || got != tt.want {
				t.Errorf("TipsOnOrigin() = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}
