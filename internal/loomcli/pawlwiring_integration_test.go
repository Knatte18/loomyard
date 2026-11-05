// pawlwiring_integration_test.go drives the production closures of a beam's landing record and of the move off a retired row over a real beam pair and real git.

package loomcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const beamPullRequestURL = "https://github.com/o/r/pull/7"

// TestBeamLandingRecordSeams_DropRewritesTheRecordAndCommitsTheLandingDirectory asserts the read seam names the pull request the landing record holds, and the drop seam rewrites the record with none and commits the landing directory.
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
	held := pawlengine.LandingPullRequest{Number: 7, URL: beamPullRequestURL, PushedHead: "abc", AnnounceHead: "abc"}
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

// TestPawlRetiredMove_CarriesTheScratchPullRequestIntoTheLandingRecord asserts the production move over a beam holding a scratch pull-request record and a ready record writes the landing record carrying the pull request with the beam's head, removes both records, makes one commit of the landing directory, and writes the status and the notice through the pawl seams.
// The hub's origin is a local path, so the GitHub read of the pull request's state fails and the pull request is carried, as for any state that does not read.
func TestPawlRetiredMove_CarriesTheScratchPullRequestIntoTheLandingRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	worktree := h.PairCodeWorktree("b")
	beam, err := lymcwd.ResolveWorktree(worktree)
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(b) error = %v; want nil", err)
	}
	scratch, scratchLock := shedrun.PullRequestRecord(beam, shedrun.SelfRunID), shedrun.PullRequestRecordLock(beam, shedrun.SelfRunID)
	for _, dir := range []string{filepath.Dir(scratch), filepath.Dir(scratchLock)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	if err := pullrequest.WriteRecord(scratch, scratchLock, pullrequest.Record{Number: 7, URL: beamPullRequestURL}); err != nil {
		t.Fatalf("WriteRecord() error = %v; want nil", err)
	}
	readyPath, readyLock := loomengine.BeamReadyPath(beam), loomengine.BeamReadyLockPath(beam)
	if err := pawlengine.WriteReady(readyPath, readyLock, pawlengine.ReadyRecord{Slug: "b", SetAt: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("WriteReady() error = %v; want nil", err)
	}
	var statuses, notices []string
	pawl := pawlengine.Deps{
		SetStatus: func(producer string) error {
			statuses = append(statuses, producer)
			return nil
		},
		Notify: func(line string) error {
			notices = append(notices, line)
			return nil
		},
	}
	records := fabricengine.RecordsWorktree(beam)
	headBefore := gitkit.Git(t, records, "rev-parse", "HEAD")

	if err := pawlRetiredMove(beam, "b", pawl)("PR-Review", pawlengine.NameBeamAwait, false); err != nil {
		t.Fatalf("the retired move error = %v; want nil", err)
	}

	beamHead := gitkit.Git(t, worktree, "rev-parse", "HEAD")
	record, found, err := pawlengine.ReadLanding(loomengine.BeamLandingPath(beam), loomengine.BeamLandingLockPath(beam), "b")
	want := pawlengine.LandingPullRequest{Number: 7, URL: beamPullRequestURL, PushedHead: beamHead, AnnounceHead: beamHead}
	if err != nil || !found || record.PullRequest == nil || *record.PullRequest != want {
		t.Errorf("ReadLanding() = %+v, %v, %v; want the pull request carried as %+v", record, found, err, want)
	}
	for _, removed := range []string{scratch, readyPath} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Errorf("%s stands after the move (stat error %v); want it removed", filepath.Base(removed), err)
		}
	}
	if got := gitkit.Git(t, records, "rev-parse", "HEAD~1"); got != headBefore {
		t.Errorf("records HEAD~1 = %s; want the move to make exactly one commit on %s", got, headBefore)
	}
	if got, want := gitkit.Git(t, records, "log", "-1", "--format=%s"), "loom: move the landing of b off a retired row"; got != want {
		t.Errorf("records commit subject = %q; want %q", got, want)
	}
	if !slices.Equal(statuses, []string{pawlengine.NameBeamAwait}) {
		t.Errorf("statuses = %q; want the one status naming %s", statuses, pawlengine.NameBeamAwait)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], beamPullRequestURL) {
		t.Errorf("notices = %q; want one naming the pull request %s", notices, beamPullRequestURL)
	}
}
