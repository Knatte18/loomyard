// holdreading.go reads, off the task worktree, the facts batten's eligible-stop hold decides on.

package arbiterstop

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// HoldReading reads the task worktree at location for the stop status describes.
// Eligible is whether Check admits the stop with no envelope kind, so an error stop is never eligible here.
// An awaiting stop that Check refuses stays eligible when its escalation reader reports nothing pending and an attempt at the stop's key has no applied mark:
// the arbiter records a circling decision before a resume, which makes the escalation non-pending while the verb's attempt still stands.
// A stop that is not eligible returns at once; any read error is returned.
func HoldReading(location *lymcwd.Location, status shedengine.Status) (battenshed.ChildArbiterStop, error) {
	readers := NewReaders(location)
	key := sheddriver.StopKeyOf(status)
	arbiterDir := shedrun.ArbiterDir(location, shedrun.SelfRunID)

	var attempts []loomengine.ArbiterAttempt
	listed := false
	listAttempts := func() error {
		if listed {
			return nil
		}
		var err error
		attempts, err = loomengine.ListArbiterAttempts(arbiterDir)
		listed = err == nil
		return err
	}

	_, checkErr := Check(status, "", "", readers)
	eligible := checkErr == nil
	if !eligible && status.State == shedengine.StateAwaiting {
		_, _, pending, err := readers.PendingEscalation(status.CurrentProducer)
		if err == nil && !pending {
			if err := listAttempts(); err != nil {
				return battenshed.ChildArbiterStop{}, err
			}
			newest, found := newestAttemptAt(attempts, key)
			if found {
				applied, err := loomengine.ArbiterApplied(newest.Dir)
				if err != nil {
					return battenshed.ChildArbiterStop{}, err
				}
				eligible = !applied
			}
		}
	}
	if !eligible {
		return battenshed.ChildArbiterStop{}, nil
	}

	seed, _, err := shedrun.ReadSeed(location, shedrun.SelfRunID)
	if err != nil {
		return battenshed.ChildArbiterStop{}, err
	}
	dirs := fabricengine.ConfigDirs(location)
	shedConfig, err := sheddriver.LoadConfig(dirs)
	if err != nil {
		return battenshed.ChildArbiterStop{}, err
	}
	override, err := loomengine.RecipeOverride(dirs, seed.Recipe)
	if err != nil {
		return battenshed.ChildArbiterStop{}, err
	}
	timeout, maxSpawns := sheddriver.ArbiterLimits(shedConfig, override)
	if err := listAttempts(); err != nil {
		return battenshed.ChildArbiterStop{}, err
	}
	reading := battenshed.ChildArbiterStop{
		Eligible:   true,
		On:         maxSpawns > 0,
		Timeout:    timeout,
		SpawnsLeft: len(attempts) < maxSpawns,
	}
	if newest, found := newestAttemptAt(attempts, key); found {
		reading.Attempt = true
		reading.AttemptDeadline = newest.Deadline
		if reading.HandOnAt, err = handOnTime(newest.Dir); err != nil {
			return battenshed.ChildArbiterStop{}, err
		}
		if reading.AppliedAt, err = modTimeIfPresent(loomengine.ArbiterAppliedPath(newest.Dir)); err != nil {
			return battenshed.ChildArbiterStop{}, err
		}
	}
	records, err := sheddriver.ReadSettleRecords(shedrun.SettleRecords(location, shedrun.SelfRunID))
	if err != nil {
		return battenshed.ChildArbiterStop{}, err
	}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].StopKey != key {
			continue
		}
		if records[i].Action == shedrun.SettleHandOver {
			reading.HandOverAt = records[i].At
		}
		break
	}
	return reading, nil
}

// newestAttemptAt returns the highest-numbered attempt of attempts, which are sorted by number, whose stop key is key.
func newestAttemptAt(attempts []loomengine.ArbiterAttempt, key sheddriver.StopKey) (loomengine.ArbiterAttempt, bool) {
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].StopKey == key {
			return attempts[i], true
		}
	}
	return loomengine.ArbiterAttempt{}, false
}

// handOnTime returns the write time of the attempt directory's envelope when it carries verdict hand-on, and zero otherwise.
func handOnTime(attemptDir string) (time.Time, error) {
	path := loomengine.ArbiterEnvelopePath(attemptDir)
	envelope, found, err := loomengine.ReadArbiterEnvelope(path)
	if err != nil || !found || envelope.Verdict != loomengine.ArbiterVerdictHandOn {
		return time.Time{}, err
	}
	return modTimeIfPresent(path)
}

// modTimeIfPresent returns path's modification time, and zero when path is absent.
func modTimeIfPresent(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("arbiterstop: stat %s: %w", path, err)
	}
	return info.ModTime(), nil
}
