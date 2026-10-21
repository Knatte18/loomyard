// settledguard.go refuses a goto that would re-run a review segment whose verdict is settled.
// It is the loom and darn recipes' goto guard, told to every goto, re-launch and verdict check of a run.

package loomcli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// settledGuard is the run's goto guard over its settled review segments, reading the Publish failure record of the task worktree.
// It needs a known location.
func (c *loomCLI) settledGuard() shedengine.GotoGuard {
	location := c.location
	verifyPaths := verifytree.NewPaths(location.WorktreePath(), verifytree.Dir(location.AnchorPath()))
	return settledSegmentGuard(c.routing.Producers, loomengine.LoomReviewsDir(location), func() (verifytree.PublishFailure, bool, error) {
		return verifytree.ReadPublishFailure(verifyPaths)
	})
}

// settledSegmentGuard refuses a goto onto a review or fix row of a review segment whose Bouncer holds a live settle record, since such a goto would re-run a settled review.
// The one admission over a live record is a Publish failure record that names the target and was written after the record settled.
// Every other target is admitted: the Bouncer, a row outside every review segment, and a segment whose record is not live.
// A read error of the settle record or the failure record is returned, which Shed turns into a refusal.
// The guard only decides; it moves no run and writes no file.
func settledSegmentGuard(producers []shedengine.ProducerDef, reviewsDir string, publishFailure func() (verifytree.PublishFailure, bool, error)) shedengine.GotoGuard {
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

		settledAt, live, err := shedadapters.SettleLive(filepath.Join(reviewsDir, subdir))
		if err != nil || !live {
			return "", err
		}

		failure, failureFound, err := publishFailure()
		if err != nil {
			return "", err
		}
		if failureFound && failure.Target == target {
			// A record an older binary wrote carries no write time, and its goto is admitted as before.
			if failure.WrittenAt == "" {
				return "", nil
			}
			writtenAt, err := time.Parse(time.RFC3339, failure.WrittenAt)
			if err != nil {
				return "", fmt.Errorf("loom: publish failure record: written_at %q: %w", failure.WrittenAt, err)
			}
			if writtenAt.After(settledAt) {
				return "", nil
			}
		}

		writer := segmentWriter(producers, segment, bouncer)
		return fmt.Sprintf("the %s verdict is settled; way forward: \"lyx loom goto --to %s\" re-settles it at no review cost, then \"lyx loom resume\" in the task worktree runs it, or \"lyx loom goto --to %s\" re-runs the writer, and a changed subject then starts a new generation", segment, bouncer, writer), nil
	}
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
