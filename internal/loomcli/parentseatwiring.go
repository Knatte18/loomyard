// parentseatwiring.go builds the Discussion-Review segment's parent seat from the run's location and config.
// The closures read the key, the history and the routing on a call, never at wire time;
// wire() also runs for status and pause.
// The chair's await cap is resolved when the seat is built.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// askParentModule and askParentKey name the setting that turns the Discussion-Review parent seat on.
const (
	askParentModule = "loom"
	askParentKey    = "discussion_review_ask_parent"
)

// parentSeatInputs holds what the Discussion-Review parent seat is built from.
type parentSeatInputs struct {
	location *lymcwd.Location
	cfg      loomengine.Config
	// runTimeout is shuttle's run_timeout_min and startAllowance its launch allowance; awaitCap is the chair's keep-warm cap.
	runTimeout     time.Duration
	startAllowance time.Duration
	awaitCap       time.Duration
	setting        func(module, key string) (any, error)
	statusPath     string
	statusLockPath string
}

// discussionParentSeat returns the Discussion-Review segment's parent seat over the run at in.location.
// Its asks go to the orch NotifyParent's routing resolves, signed as lym's loom program and tagged with the pair's run.
func discussionParentSeat(in parentSeatInputs) *shedadapters.ParentReviewSeat {
	location := in.location
	return &shedadapters.ParentReviewSeat{
		Files:      burlerengine.ParentReviewFilesOf(location.AnchorPath()),
		Slug:       seedSlug(location.WorktreeName),
		RecordPath: loomengine.DiscussionDecisionRecord(location),
		AskEnabled: func() bool { return askParentSetting(in.setting) },
		Wait: func() (time.Duration, string) {
			return loomengine.CapParentReviewWait(in.cfg, in.runTimeout, in.startAllowance, in.awaitCap)
		},
		RecordWrittenAt: func() (time.Time, error) {
			return latestCallAt(in.statusPath, in.statusLockPath, loomshed.NameDiscussionWrite)
		},
		Resolve: func() (string, bool, error) { return orchcli.ParentStrand(location) },
		Post: func(strand, line string) error {
			return orchcli.PostAsProgram(location, strand, loomProgram, line, creelengine.PostTags{Run: location.WorktreeName})
		},
	}
}

// askParentSetting reads discussion_review_ask_parent through setting.
// An unreadable value, or one that is not a boolean, is logged and read as false.
func askParentSetting(setting func(module, key string) (any, error)) bool {
	value, err := setting(askParentModule, askParentKey)
	if err != nil {
		logger.Warn("loomcli: the parent-review setting is unreadable; this round asks no parent", "key", askParentKey, "error", err)
		return false
	}
	on, ok := value.(bool)
	if !ok {
		logger.Warn("loomcli: the parent-review setting is not a boolean; this round asks no parent", "key", askParentKey, "value", value)
		return false
	}
	return on
}

// latestCallAt returns the latest time the status history at statusPath records a call of producer.
// It reads each entry's At and LastAt; the zero time answers a history with none or an absent status file.
func latestCallAt(statusPath, statusLockPath, producer string) (time.Time, error) {
	status, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	var latest time.Time
	if !found {
		return latest, nil
	}
	for _, entry := range status.History {
		if entry.Producer != producer {
			continue
		}
		for _, stamp := range []string{entry.At, entry.LastAt} {
			if stamp == "" {
				continue
			}
			at, err := time.Parse(time.RFC3339, stamp)
			if err != nil {
				return time.Time{}, fmt.Errorf("loomcli: the status history's %s entry holds the time %q: %w", producer, stamp, err)
			}
			if at.After(latest) {
				latest = at
			}
		}
	}
	return latest, nil
}
