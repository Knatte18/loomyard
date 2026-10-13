// innerrun_binaryrestep.go implements the halted arm's binary re-step:
// after a Go driver hands back and exits, a deploy of a new lyx re-steps the halted child once, through its bootstrap, so a fix lands without a parent turn.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	// binaryRestepFileSuffix is the fixed suffix of the marker naming the halt and the build already re-stepped, joined onto the producer's own name.
	binaryRestepFileSuffix = "-binary-restep"
	// binaryRestepCeiling caps how far the interval between two build checks backs off while nothing changes.
	binaryRestepCeiling = 30 * time.Minute
	// binaryRestepAttemptCap is how many refused or failed spawns one halt and build take before the re-step gives up on them.
	binaryRestepAttemptCap = 5
)

// binaryRestepFile returns the path of the binary re-step marker: the halt's stop key on its first line and the build label on its second.
func binaryRestepFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+binaryRestepFileSuffix)
}

// restepWatch is the binary re-step's state across the checks and Calls of one producer.
type restepWatch struct {
	// stop is the stop key of the halt last seen, and build the on-disk build label last read for it.
	stop  string
	build string
	// interval is the wait between two build checks, nextAt when the next one is due, and attempts the spawns refused or failed for this halt and build.
	interval time.Duration
	nextAt   time.Time
	attempts int
	// dirtyLogged is the uncommitted-path list last logged at Info for this halt and build, so an unchanged list logs at Debug.
	dirtyLogged []string
}

// restepStopKey identifies one halt of the child: its state, its row and the length of its history.
func restepStopKey(status shedengine.Status) string {
	return fmt.Sprintf("%s|%s|%d", status.State, status.CurrentProducer, len(status.History))
}

// binaryRestep re-steps a halted driverless go child once per halt and build, through the child's bootstrap.
// It acts only when every condition holds:
// the child is blocked or failed, and its reed state holds no driver strand;
// neither its run lock nor its driver lock is held, and the Call's publish retry takes no action;
// the on-disk build differs from the child's last step's, and the task pair has no uncommitted path outside the run's records;
// and the marker does not already name this halt and this build.
// The marker is written only after the spawn returns success, so a batten restart repeats nothing.
// A refused spawn writes no marker and is retried at the next due check, at most binaryRestepAttemptCap times, and a dirty tree is retried the same way; neither fails the row.
// The build and the dirty paths cost a process each, so the check runs once per interval, which starts at the notice probe, doubles up to binaryRestepCeiling while the halt and the build stay the same, and returns to the probe on a change.
func (p *innerRunProducer) binaryRestep(ctx context.Context, w *childWait) *waitEnd {
	status := w.status
	if !p.restepOn || (status.State != shedengine.StateBlocked && status.State != shedengine.StateFailed) || w.retry.action != publishRetryNone {
		return nil
	}
	stop := restepStopKey(status)
	if stop != p.restep.stop {
		p.restep = restepWatch{stop: stop, interval: p.noticeProbe}
	}
	now := p.deps.Now()
	if now.Before(p.restep.nextAt) {
		return nil
	}
	p.restep.nextAt = now.Add(p.restep.interval)
	p.restep.interval = min(2*p.restep.interval, binaryRestepCeiling)

	strand, err := p.deps.DriverStrand(ctx)
	if err != nil {
		return p.restepSkipped(ctx, failRestepDriverStrand, err)
	}
	p.seamRecovered(failRestepDriverStrand)
	if strand != ChildDriverNone {
		return nil
	}
	atWork, err := p.driverAtWork(ctx)
	if err != nil {
		return p.restepSkipped(ctx, failRestepLocks, err)
	}
	p.seamRecovered(failRestepLocks)
	if atWork {
		return nil
	}

	build, changed, err := p.deps.ChildBuildChanged()
	if err != nil {
		return p.restepSkipped(ctx, failRestepBuild, err)
	}
	p.seamRecovered(failRestepBuild)
	if p.restep.build != "" && build != p.restep.build {
		p.restep.interval = p.noticeProbe
		p.restep.nextAt = now.Add(p.restep.interval)
		p.restep.attempts = 0
		p.restep.dirtyLogged = nil
	}
	p.restep.build = build
	if !changed {
		return nil
	}
	if p.restepRecorded(stop, build) {
		return nil
	}

	dirty, err := p.deps.ChildDirtyPaths()
	if err != nil {
		return p.restepSkipped(ctx, failRestepDirtyPaths, err)
	}
	p.seamRecovered(failRestepDirtyPaths)
	if len(dirty) > 0 {
		logAttempt(!slices.Equal(dirty, p.restep.dirtyLogged), "battenshed: not re-stepping the halted child on the new build; the task pair has uncommitted paths", "producer", p.name, "slug", p.slug, "build", build, "paths", dirty)
		p.restep.dirtyLogged = dirty
		return nil
	}
	if p.restep.attempts >= binaryRestepAttemptCap {
		return nil
	}

	logger.Info("battenshed: re-stepping the halted child on the new build", "producer", p.name, "slug", p.slug, "build", build, "stop", stop)
	spawnErr := p.deps.Spawn(ctx)
	logger.Info("battenshed: inner shed run wait complete", "producer", p.name, "slug", p.slug)
	if spawnErr != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return hardEnd(cerr)
		}
		p.restep.attempts++
		logger.Warn("battenshed: the halted child's re-step was refused; retrying at the next due check", "producer", p.name, "slug", p.slug, "build", build, "attempt", p.restep.attempts, "cap", binaryRestepAttemptCap, "refused_as_not_parked", errors.Is(spawnErr, ErrChildNotParked), "refused_as_busy", errors.Is(spawnErr, ErrChildDriverBusy), "error", spawnErr)
		return nil
	}
	recordSpawnConfirmed(p.name, p.slug, p.scratchDir, SpawnConfirmedFile(p.scratchDir, p.name))
	if err := p.writeBinaryRestepMarker(stop, build); err != nil {
		return hardEnd(err)
	}
	return nil
}

// restepSkipped counts a seam failure of the re-step check, named by failure, and leaves the halt to the usual handling, ending the wait only when the context was cancelled.
func (p *innerRunProducer) restepSkipped(ctx context.Context, failure string, err error) *waitEnd {
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return hardEnd(cerr)
	}
	p.seamFailed(failure, err)
	return nil
}

// restepRecorded reports whether the marker already names this halt and this build; an unreadable marker reads as not recorded.
func (p *innerRunProducer) restepRecorded(stop, build string) bool {
	raw, err := os.ReadFile(binaryRestepFile(p.scratchDir, p.name))
	if err != nil {
		if !os.IsNotExist(err) {
			p.seamFailed(failRestepMarker, err)
			return false
		}
		p.seamRecovered(failRestepMarker)
		return false
	}
	p.seamRecovered(failRestepMarker)
	return string(raw) == stop+"\n"+build+"\n"
}

// writeBinaryRestepMarker records the halt and the build the child was re-stepped on.
func (p *innerRunProducer) writeBinaryRestepMarker(stop, build string) error {
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		return fmt.Errorf("battenshed: %s: create scratch directory for binary re-step marker: %w", p.name, err)
	}
	if err := os.WriteFile(binaryRestepFile(p.scratchDir, p.name), []byte(stop+"\n"+build+"\n"), 0o644); err != nil {
		return fmt.Errorf("battenshed: %s: write binary re-step marker: %w", p.name, err)
	}
	return nil
}
