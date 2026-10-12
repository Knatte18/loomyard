// cache.go reads a shed run's status file for a status pane without the run's status lock, decoding it only when its modification time changed.

package statuspane

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// StatusPoll is one StatusCache.Poll's answer.
type StatusPoll struct {
	// Status is the decoded status, the last good one while a first failure is being tolerated.
	Status shedengine.Status
	// Found reports that Status holds a status; false for an absent file and for an unreadable one.
	Found bool
	// Unreadable reports that the file has failed to read or decode at two polls in a row, or at a later poll of the same failed modification time.
	Unreadable bool
}

// StatusCache polls one status file, keeping the last good decode so an unchanged file costs a stat.
// It is not safe for concurrent use.
type StatusCache struct {
	modTime    time.Time
	status     shedengine.Status
	haveStatus bool
	// failures counts the failed polls in a row; failedModTime is the modification time of the last failed read, when it got that far.
	failures       int
	failedModTime  time.Time
	failedModKnown bool
}

// NewStatusCache returns an empty cache.
func NewStatusCache() *StatusCache {
	return &StatusCache{}
}

// Poll reads the status at path without the run's status lock, which is safe because internal/state writes it atomically, so it never blocks or slows a run.
// It stats the file and decodes it only when its modification time differs from the last good decode's.
// An absent file is Found false and no failure.
// A failed stat, read or decode is a failure: the first in a row returns the last good status, Found false when there was none, and a second in a row, or a later poll of the same failed modification time, returns Unreadable; the next good decode clears the count.
func (c *StatusCache) Poll(path string) StatusPoll {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		*c = StatusCache{}
		return StatusPoll{}
	}
	if err != nil {
		return c.fail(time.Time{}, false)
	}

	modTime := info.ModTime()
	if c.failedModKnown && modTime.Equal(c.failedModTime) {
		return c.fail(modTime, true)
	}
	if c.haveStatus && modTime.Equal(c.modTime) {
		return StatusPoll{Status: c.status, Found: true}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return c.fail(modTime, true)
	}
	var status shedengine.Status
	if err := json.Unmarshal(data, &status); err != nil {
		return c.fail(modTime, true)
	}

	c.status, c.haveStatus, c.modTime = status, true, modTime
	c.failures, c.failedModKnown = 0, false
	return StatusPoll{Status: status, Found: true}
}

// fail records one failed poll, whose modification time is modTime when known, and answers it.
func (c *StatusCache) fail(modTime time.Time, known bool) StatusPoll {
	c.failures++
	c.failedModTime, c.failedModKnown = modTime, known
	if c.failures >= 2 {
		return StatusPoll{Unreadable: true}
	}
	return StatusPoll{Status: c.status, Found: c.haveStatus}
}
