// overvieworch.go reads the orch session's state and thresholds for the batten overview pane and renders the orch line drawn above the runs in flight.

package battencli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

const (
	overviewOrchBroken   = "orch  state unreadable"
	overviewWatcherDown  = "watcher down"
	overviewOrchModule   = "orch"
	overviewTokensSuffix = "k"
)

// orchStatePoll is one orchStateCache.Poll's answer.
type orchStatePoll struct {
	// State is the decoded orch state, the last good one while a first failure is being tolerated.
	State orchengine.State
	// Found reports that State holds a state; false for an absent file and for an unreadable one.
	Found bool
	// Unreadable reports that the file has failed to read or decode at two polls in a row, or at a later poll of the same failed modification time.
	Unreadable bool
}

// orchStateCache polls the orch state file, keeping the last good decode so an unchanged file costs a stat.
// It takes no lock and creates nothing, and is not safe for concurrent use.
type orchStateCache struct {
	modTime   time.Time
	state     orchengine.State
	haveState bool
	// failures counts the failed polls in a row; failedModTime is the modification time of the last failed read, when it got that far.
	failures       int
	failedModTime  time.Time
	failedModKnown bool
}

// Poll reads the orch state at paths.StatePath, decoding it only when its modification time differs from the last good decode's.
// An absent file is Found false and no failure.
// A failed stat, read or decode is a failure: the first in a row returns the last good state, Found false when there was none, and a second in a row, or a later poll of the same failed modification time, returns Unreadable; the next good decode clears the count.
func (c *orchStateCache) Poll(paths orchengine.Paths) orchStatePoll {
	info, err := os.Stat(paths.StatePath)
	var modTime time.Time
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// PeekState's not-found result below is the one absent branch, which also covers a file removed after this stat.
	case err != nil:
		return c.fail(time.Time{}, false, err)
	default:
		modTime = info.ModTime()
		if c.failedModKnown && modTime.Equal(c.failedModTime) {
			return c.fail(modTime, true, errors.New("state file unchanged since its failed read"))
		}
		if c.haveState && modTime.Equal(c.modTime) {
			return orchStatePoll{State: c.state, Found: true}
		}
	}

	state, found, err := orchengine.PeekState(paths)
	if err != nil {
		return c.fail(modTime, !modTime.IsZero(), err)
	}
	if !found {
		*c = orchStateCache{}
		return orchStatePoll{}
	}

	c.state, c.haveState, c.modTime = state, true, modTime
	c.failures, c.failedModKnown = 0, false
	return orchStatePoll{State: state, Found: true}
}

// fail records one failed poll, whose modification time is modTime when known, and answers it.
func (c *orchStateCache) fail(modTime time.Time, known bool, err error) orchStatePoll {
	logger.Debug("batten: polling the orch state for the overview failed", "err", err)
	c.failures++
	c.failedModTime, c.failedModKnown = modTime, known
	if c.failures >= 2 {
		return orchStatePoll{Unreadable: true}
	}
	return orchStatePoll{State: c.state, Found: c.haveState}
}

// overviewThresholds is the orch config's context thresholds, as the overview draws them.
type overviewThresholds struct {
	// Soft is the token count at which the watcher may cycle at a natural break.
	Soft int
	// Hard is the token count that forces a cycle.
	Hard int
	// Known reports that Soft and Hard were loaded; false draws `-` for both.
	Known bool
}

// loadOverviewThresholds loads the soft and hard thresholds from the orch config of location's stack.
// A config that does not load logs a warning and returns the zero value, so a broken orch.yaml never refuses the overview.
func loadOverviewThresholds(location *lyxcwd.Location) overviewThresholds {
	cfg, err := orchengine.LoadConfig(fabricengine.ConfigDirs(location), overviewOrchModule)
	if err != nil {
		logger.Warn("batten: loading the orch thresholds for the overview failed", "err", err)
		return overviewThresholds{}
	}
	return overviewThresholds{Soft: cfg.SoftThreshold(), Hard: cfg.Threshold(), Known: true}
}

// overviewOrchLine renders the orch line for orch against thresholds; false when there is no orch state file to show.
// The line reads `orch  <tokens> / soft <soft> / hard <hard>  <phase>  cycles <n>`, with `  watcher down` appended once the watcher has exited.
func overviewOrchLine(orch orchStatePoll, thresholds overviewThresholds) (string, bool) {
	if orch.Unreadable {
		return overviewOrchBroken, true
	}
	if !orch.Found {
		return "", false
	}

	tokens, soft, hard := overviewUnknown, overviewUnknown, overviewUnknown
	if orch.State.LastContextKnown {
		tokens = overviewTokens(orch.State.LastContextTokens)
	}
	if thresholds.Known {
		soft, hard = overviewTokens(thresholds.Soft), overviewTokens(thresholds.Hard)
	}
	var line strings.Builder
	fmt.Fprintf(&line, "orch  %s / soft %s / hard %s  %s  cycles %d", tokens, soft, hard, orch.State.Phase, orch.State.CycleCount)
	if orch.State.WatcherExit != "" {
		line.WriteString(overviewColumnGap + overviewWatcherDown)
	}
	return line.String(), true
}

// overviewTokens renders a token count: the plain number under 1000, otherwise thousands rounded down with a `k` suffix.
func overviewTokens(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprint(n/1000) + overviewTokensSuffix
}
