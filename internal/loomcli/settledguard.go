// settledguard.go refuses a goto that would re-run a review segment whose verdict is settled.
// It is the tapestry, darn and sampler recipes' goto guard, told to every goto, re-launch and verdict check of a run.

package loomcli

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// settledGuard is the run's whole goto guard: the Finalize arm first, then the settled-segment arm over its settled review segments.
// It needs a known location.
func (c *loomCLI) settledGuard() shedengine.GotoGuard {
	location := c.location
	readStatus := circlingStatusReader(shedrun.StatusFile(location, c.runID), shedrun.StatusLock(location, c.runID))
	return composeGotoGuards(c.finalizeGuard(), settledSegmentGuard(c.routing.Producers, loomengine.LoomReviewsDir(location), func() (loomshed.AmendmentRecord, bool, error) {
		return loomshed.ReadAmendment(loomengine.LoomAmendmentRecord(location))
	}, func() ([]shedengine.HistoryEntry, error) {
		status, _, err := readStatus()
		return status.History, err
	}))
}

// settledSegmentGuard refuses a goto onto a review or fix row of a review segment whose Bouncer holds a live settle record, since such a goto would re-run a settled review.
// One admission passes a live record: for Plan-Burler-Review alone, a plan amendment record written after the judged round started or over a settle with no known round start.
// Every other target is admitted: the Bouncer, a row outside every review segment, and a segment whose record is not live.
// The refusal names the plan writer the run ran, read from history once per refusal.
// A read error of the settle record, the amendment record or the history is returned, which Shed turns into a refusal.
// The guard only decides; it moves no run and writes no file.
func settledSegmentGuard(producers []shedengine.ProducerDef, reviewsDir string, amendment func() (loomshed.AmendmentRecord, bool, error), history func() ([]shedengine.HistoryEntry, error)) shedengine.GotoGuard {
	return func(target string) (string, error) {
		var segment string
		for _, producer := range producers {
			if producer.Name == target {
				segment = producer.Segment
			}
		}
		if segment == "" {
			return "", nil
		}

		bouncer, subdir, found, err := segmentBouncer(producers, segment)
		if err != nil || !found || target == bouncer {
			return "", err
		}

		_, live, err := shedadapters.SettleLive(filepath.Join(reviewsDir, subdir))
		if err != nil || !live {
			return "", err
		}

		if target == loomshed.NamePlanBurlerReview {
			admitted, err := amendmentAdmits(filepath.Join(reviewsDir, subdir), amendment)
			if err != nil || admitted {
				return "", err
			}
		}

		runHistory, err := history()
		if err != nil {
			return "", err
		}
		writer := segmentWriter(producers, segment, bouncer, runHistory)
		return fmt.Sprintf("the %s verdict is settled; way forward: \"lym loom goto --to %s\" re-settles it at no review cost, then \"lym loom resume\" in the task worktree runs it, or \"lym loom goto --to %s\" re-runs the writer, and a changed subject then starts a new generation", segment, bouncer, writer), nil
	}
}

// amendmentAdmits reports whether a plan amendment record exists that the settle record in runDir has not judged:
// its write time is not before the judged round's start, or the settle records no start.
func amendmentAdmits(runDir string, amendment func() (loomshed.AmendmentRecord, bool, error)) (bool, error) {
	record, found, err := amendment()
	if err != nil || !found {
		return false, err
	}
	roundStart, _, err := shedadapters.SettleRoundStart(runDir)
	if err != nil {
		return false, err
	}
	return !loomshed.AmendmentSettledBy(record, roundStart), nil
}

// segmentBouncer returns the Bouncer row of segment and its run subdirectory, and false when the segment holds none.
func segmentBouncer(producers []shedengine.ProducerDef, segment string) (name, subdir string, found bool, err error) {
	for _, producer := range producers {
		if producer.Segment != segment {
			continue
		}
		subdir, isBouncer, err := loomrecipe.BouncerRunSubdir(producer.Name)
		if err != nil {
			return "", "", false, err
		}
		if isBouncer {
			return producer.Name, subdir, true, nil
		}
	}
	return "", "", false, nil
}

// segmentWriter returns the first row in list order outside segment whose OnDone names bouncer.
// When that row's latest history entry is Skipped and a row outside the segment names it as its OnDone, it returns that row instead, and never steps further.
func segmentWriter(producers []shedengine.ProducerDef, segment, bouncer string, history []shedengine.HistoryEntry) string {
	writer := ""
	for _, producer := range producers {
		if producer.Segment != segment && producer.OnDone == bouncer {
			writer = producer.Name
			break
		}
	}
	if writer == "" || !latestSkipped(history, writer) {
		return writer
	}
	for _, producer := range producers {
		if producer.Segment != segment && producer.OnDone == writer {
			return producer.Name
		}
	}
	return writer
}

// latestSkipped reports whether the latest history entry of the row named producer is a Skipped one.
func latestSkipped(history []shedengine.HistoryEntry, producer string) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Producer == producer {
			return history[i].Outcome == shedengine.OutcomeSkipped
		}
	}
	return false
}
