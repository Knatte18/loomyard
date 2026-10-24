// settledguard.go refuses a goto that would re-run a review segment whose verdict is settled.
// It is the loom and darn recipes' goto guard, told to every goto, re-launch and verdict check of a run.

package loomcli

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// settledGuard is the run's whole goto guard: the Finalize arm first, then the settled-segment arm over its settled review segments.
// It needs a known location.
func (c *loomCLI) settledGuard() shedengine.GotoGuard {
	location := c.location
	return composeGotoGuards(c.finalizeGuard(), settledSegmentGuard(c.routing.Producers, loomengine.LoomReviewsDir(location), func() (loomshed.AmendmentRecord, bool, error) {
		return loomshed.ReadAmendment(loomengine.LoomAmendmentRecord(location))
	}))
}

// settledSegmentGuard refuses a goto onto a review or fix row of a review segment whose Bouncer holds a live settle record, since such a goto would re-run a settled review.
// One admission passes a live record: for Plan-Burler-Review alone, a plan amendment record written after the judged round started or over a settle with no known round start.
// Every other target is admitted: the Bouncer, a row outside every review segment, and a segment whose record is not live.
// A read error of the settle record or the amendment record is returned, which Shed turns into a refusal.
// The guard only decides; it moves no run and writes no file.
func settledSegmentGuard(producers []shedengine.ProducerDef, reviewsDir string, amendment func() (loomshed.AmendmentRecord, bool, error)) shedengine.GotoGuard {
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

		writer := segmentWriter(producers, segment, bouncer)
		return fmt.Sprintf("the %s verdict is settled; way forward: \"lyx loom goto --to %s\" re-settles it at no review cost, then \"lyx loom resume\" in the task worktree runs it, or \"lyx loom goto --to %s\" re-runs the writer, and a changed subject then starts a new generation", segment, bouncer, writer), nil
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
func segmentWriter(producers []shedengine.ProducerDef, segment, bouncer string) string {
	for _, producer := range producers {
		if producer.Segment != segment && producer.OnDone == bouncer {
			return producer.Name
		}
	}
	return ""
}
