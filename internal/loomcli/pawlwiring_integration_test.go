// pawlwiring_integration_test.go drives the production closures of a beam's landing record.
// It runs over a real beam pair and real git.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const beamPullRequestURL = "https://github.com/o/r/pull/7"

// TestBeamLandingRecordSeams_DropRewritesTheRecordAndCommitsTheLandingDirectory asserts the seams:
// the read seam names the pull request the landing record holds,
// and the drop seam rewrites the record with none and commits the landing directory.
func TestBeamLandingRecordSeams_DropRewritesTheRecordAndCommitsTheLandingDirectory(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	beam, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("b"))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(b) error = %v; want nil", err)
	}
	path, lockPath := loomengine.BeamLandingPath(beam), loomengine.BeamLandingLockPath(beam)
	held := pawlengine.LandingPullRequest{Number: 7, URL: beamPullRequestURL, PushedHead: "abc"}
	err = pawlengine.UpdateLanding(path, lockPath, "b", func(rec *pawlengine.LandingRecord) error {
		rec.PullRequest = &held
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateLanding() error = %v; want nil", err)
	}
	read, drop := beamLandingRecordSeams(beam, "b")

	if got, found, err := read(); err != nil || !found || got != (landingshed.BeamPullRequest{Number: 7, URL: beamPullRequestURL}) {
		t.Fatalf("read() = %+v, %v, %v; want the recorded pull request", got, found, err)
	}
	if err := drop(); err != nil {
		t.Fatalf("drop() error = %v; want nil", err)
	}

	if got, found, err := read(); err != nil || found {
		t.Errorf("read() = %+v, %v, %v after the drop; want no pull request", got, found, err)
	}
	records := fabricengine.RecordsWorktree(beam)
	if got, want := gitkit.Git(t, records, "log", "-1", "--format=%s"), "loom: drop the landed pull request from the landing record of b"; got != want {
		t.Errorf("records commit subject = %q; want %q", got, want)
	}
	if got := gitkit.Git(t, records, "status", "--porcelain"); got != "" {
		t.Errorf("records status = %q; want the rewritten landing record committed", got)
	}
}
