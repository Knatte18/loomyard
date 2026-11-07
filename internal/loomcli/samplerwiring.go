// samplerwiring.go wires the Sampler-Session row's seams, entry stamp and record watch, which only a sampler run needs.
// A recipe without a Sampler-Session row leaves all of it unwired.

package loomcli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// samplerConfigWayForward is the trailing clause for a failed load of the hub-wide sampler config.
// A value the hub tuned survives `lym config reconcile`, so the fix is an edit of the key rather than a reconcile.
const samplerConfigWayForward = "way forward: fix the key with lym config sampler (the editor) or lym config sampler --set <key>=<value> from the prime, then run lym loom resume"

// samplerStamp returns a reader of the entry stamp of row in the run runID.
// The stamp is computed from the rows of recipe that route into row.
// It reads the status file under its lock at each call and returns 0 when the file is absent.
func samplerStamp(l *lymcwd.Location, runID, recipe, row string) func() (int, error) {
	return func() (int, error) {
		rows, err := loomrecipe.Rows(recipe)
		if err != nil {
			return 0, err
		}
		var predecessors []string
		for _, candidate := range rows {
			if candidate.OnDone == row {
				predecessors = append(predecessors, candidate.Name)
			}
		}

		status, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(l, runID), shedrun.StatusLock(l, runID))
		// A run directory that does not exist yet fails the lock open with not-exist; that counts as an absent status file too.
		if errors.Is(err, fs.ErrNotExist) || (err == nil && !found) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		return samplerengine.EntryStamp(status.History, row, predecessors), nil
	}
}

// samplerDeps assembles the Sampler-Session row's seams for location l and the run addressed by runID.
// recipe is the run's recipe, row is the session row, and spec builds the session's spec at each start.
// Every closure reads or writes on demand and opens nothing at build time.
// This matters because the wiring also runs for status and pause.
// The seams that need the strand table are left to the landing wiring.
func samplerDeps(l *lymcwd.Location, runID, recipe, row string, spec func() (shuttleengine.Spec, error)) loomshed.SamplerDeps {
	slug := seedSlug(l.WorktreeName)
	return loomshed.SamplerDeps{
		RecordPath:     shedrun.SamplerRecord(l, runID),
		RecordLockPath: shedrun.SamplerRecordLock(l, runID),
		Spec:           spec,
		Stamp:          samplerStamp(l, runID, recipe, row),
		DirtyPaths:     func() ([]string, error) { return verifytree.DirtyPaths(l.WorktreePath()) },
		CheckRecord:    discussionRecordCheck(l, recipe),
		MailAgent: func(strandName, line string) error {
			client, err := creelcli.NewProgramClient(l, loomProgram)
			if err != nil {
				return err
			}
			_, err = client.Post(strandName, line)
			return err
		},
		// PauseCovered reports whether the pause record covers the slug; an unreadable record covers nothing.
		PauseCovered: func() (bool, error) { return pauseCovered(l)(slug), nil },
		Now:          time.Now,
		Sleep:        sleepContext,
	}
}

// openRecordWatch returns a channel that delivers a value whenever the file at path may have changed.
// It returns nil when no watch can be opened, which leaves the Sampler-Session row on its timer.
// The record's directory is created when absent, since the row may start before anything has written there.
// The watcher lives until the process ends.
func openRecordWatch(path string) <-chan struct{} {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("loom: the sampler record directory could not be created, so Sampler-Session polls", "dir", dir, "error", err)
		return nil
	}
	changed, _ := watchFileChanges(path, "Sampler-Session")
	return changed
}
