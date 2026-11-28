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
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// holdRecordsCommit writes a standing landing of the run slug into the ledger of p's hub.
// The landing holds the records commit held.
func holdRecordsCommit(t *testing.T, p *pairFixture, slug, held string) {
	t.Helper()
	err := pushledger.Update(p.hub.Location.HubPath, pushledger.LandingKey, time.Unix(1, 0), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
		return pushledger.Entry{Landings: []pushledger.Landing{{Run: slug, Tip: held, RecordsCommit: held, ParentBranch: "main", At: time.Unix(1, 0)}}}, true, nil
	})
	if err != nil {
		t.Fatalf("pushledger.Update: %v", err)
	}
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
		{"RecordsBranchWithItsTrackingRefDeletedReadsLevel", func(t *testing.T) *lymcwd.Location {
			f, _ := newFailureFixture(t)
			weft, branch := f.primeWeft()
			gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")
			pruneTrackingRef(t, weft, branch)
			return f.hub.Location
		}, false},
		{"PairCodeBranchWithNoUpstreamReadsAhead", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "noupstreamcode")
			unsetUpstream(t, p.warpPath, p.warpBranch)
			return p.location
		}, true},
		{"PairRecordsAheadOnlyByTheHeldRefsCommitReadsLevel", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "heldref")
			held := gitkit.CommitFile(t, p.weftPath, "held.txt", "held\n", "held records")
			if err := fabricengine.KeepHeldRecords(p.location, "heldref", held); err != nil {
				t.Fatalf("KeepHeldRecords() = %v; want nil", err)
			}
			return p.location
		}, false},
		{"PairRecordsWithAnUnpushedCommitReadsAhead", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "unpushedrecords")
			gitkit.CommitFile(t, p.weftPath, "local.txt", "local\n", "local")
			return p.location
		}, true},
		{"PairDetachedPastOriginWithItsMarkerReadsTheBranchLevel", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "detachedlevel")
			gitkit.Git(t, p.warpPath, "checkout", "--detach")
			gitkit.CommitFile(t, p.warpPath, "chain.txt", "chain\n", "commit origin lacks")
			if err := fabricengine.WriteDetachMarker(p.location, p.warpBranch); err != nil {
				t.Fatalf("WriteDetachMarker() = %v; want nil", err)
			}
			return p.location
		}, false},
		{"PairDetachedWithItsMarkerAndNoCodeUpstreamReadsAhead", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "detachednoupstream")
			gitkit.Git(t, p.warpPath, "checkout", "--detach")
			unsetUpstream(t, p.warpPath, p.warpBranch)
			if err := fabricengine.WriteDetachMarker(p.location, p.warpBranch); err != nil {
				t.Fatalf("WriteDetachMarker() = %v; want nil", err)
			}
			return p.location
		}, true},
		{"PairDetachedWithItsMarkerAndItsTrackingRefDeletedReadsLevel", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "detachedpruned")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "unpushed code")
			gitkit.Git(t, p.warpPath, "checkout", "--detach")
			pruneTrackingRef(t, p.warpPath, p.warpBranch)
			if err := fabricengine.WriteDetachMarker(p.location, p.warpBranch); err != nil {
				t.Fatalf("WriteDetachMarker() = %v; want nil", err)
			}
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

// TestRefusedPushStands covers that the code branch read beside the records branch is the pair's own.
// A detached pair's own code branch is the one its detach marker names.
func TestRefusedPushStands(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		slug string
		// detach detaches the pair's code worktree under its marker once the push is refused.
		detach bool
	}{
		{"NotFastForwardRecordOfThePairsCodeBranch", "divergedcoderead", false},
		{"RecordOfTheCodeBranchOfADetachedPairStands", "divergeddetached", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newPairFixture(t, tt.slug)
			clone := cloneBareForTest(t, p.hub.CodeBare, p.warpBranch)
			gitkit.CommitFile(t, clone, "other.txt", "other\n", "other machine")
			gitkit.MustRun(t, clone, "git", "push")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "warp change")
			_, _ = fabricengine.PushPairAnchored(p.location, p.fake.options(func(time.Duration) {}), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})
			requireOneFailedPush(t, p.hub.Location.HubPath, "code", fabriclayout.PushFailureNotFastForward)
			if tt.detach {
				gitkit.Git(t, p.warpPath, "checkout", "--detach")
				if err := fabricengine.WriteDetachMarker(p.location, p.warpBranch); err != nil {
					t.Fatalf("WriteDetachMarker() = %v; want nil", err)
				}
			}

			got, err := fabricengine.RefusedPushStands(p.location)

			if err != nil || !got {
				t.Errorf("RefusedPushStands() = %v, %v; want true, nil", got, err)
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
		{"CodeBranchWithItsTrackingRefDeletedReadsHeld", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "codepruned")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "warp")
			pruneTrackingRef(t, p.warpPath, p.warpBranch)
			return p.location
		}, true},
		{"CodeBranchWithAnUnpushedCommitReadsNotOnOrigin", func(t *testing.T) *lymcwd.Location {
			p := newPairFixture(t, "unpushedcode")
			gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "warp")
			return p.location
		}, false},
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
