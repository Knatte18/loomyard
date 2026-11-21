// burlerreview_halt_test.go pins that BurlerReviewProducer hands the plan executor's halt reason to its reviewer.
// That covers the reviewer it spawns, probes and resumes.

package shedadapters

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBurlerReviewProducer_Call_HaltReasonReachesTheReviewer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const reason = "card 14 misses the retry"
	conflictDir := t.TempDir()
	if err := runplan.WriteHaltReason(conflictDir, reason); err != nil {
		t.Fatalf("WriteHaltReason() error = %v; want nil", err)
	}

	t.Run("SpawnedReviewerGetsIt", func(t *testing.T) {
		t.Parallel()
		spec := newHalfSpec()
		spec.haltConflictDir = conflictDir
		runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}
		p := newReviewProducer(t, t.TempDir(), runner, spec)

		shedfake.CallOK(t, p)

		if got := runner.GotReviewProfiles[0].HaltReason; got != reason {
			t.Errorf("spawned reviewer's HaltReason = %q; want %q", got, reason)
		}
	})

	t.Run("ProbedAndResumedReviewerUnderADifferingFanGetsIt", func(t *testing.T) {
		t.Parallel()
		runDir := t.TempDir()
		writeRoundFile(t, roundReviewPath(runDir, 1))
		// The reviewer was spawned under a fan the lists no longer name, so the probe profile is built apart from the round's.
		if err := writeRoundUsage(runDir, 1, roundUsage{Fan: "fan-old", FansRecorded: true, Review: &halfUsage{}}); err != nil {
			t.Fatalf("writeRoundUsage: %v", err)
		}
		spec := newHalfSpec()
		spec.haltConflictDir = conflictDir
		spec.fans = &RoundFans{Review: []string{"fan-new"}, Fix: []string{""}}
		runner := &shedfake.BurlerHalfRunner{
			LiveReviews:         []burlerengine.LiveHalf{{State: burlerengine.HalfLive}},
			ResumeReviewResults: []burlerengine.ReviewResult{reviewDone("resumed")},
		}
		p := newReviewProducer(t, runDir, runner, spec)

		shedfake.CallOK(t, p)

		if len(runner.GotProbeReviewProfiles) != 1 || len(runner.GotResumeReviewProfiles) != 1 {
			t.Fatalf("probed %d and resumed %d reviewers; want 1 and 1", len(runner.GotProbeReviewProfiles), len(runner.GotResumeReviewProfiles))
		}
		probed := runner.GotProbeReviewProfiles[0]
		if probed.ClusterFan != "fan-old" {
			t.Errorf("probed reviewer's ClusterFan = %q; want the recorded %q", probed.ClusterFan, "fan-old")
		}
		if probed.HaltReason != reason || runner.GotResumeReviewProfiles[0].HaltReason != reason {
			t.Errorf("probed and resumed reviewers' HaltReason = %q and %q; want %q for both", probed.HaltReason, runner.GotResumeReviewProfiles[0].HaltReason, reason)
		}
	})

	t.Run("NoConflictDirectoryTellsNone", func(t *testing.T) {
		t.Parallel()
		runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}
		p := newReviewProducer(t, t.TempDir(), runner, newHalfSpec())

		shedfake.CallOK(t, p)

		if got := runner.GotReviewProfiles[0].HaltReason; got != "" {
			t.Errorf("HaltReason = %q; want none", got)
		}
	})
}
