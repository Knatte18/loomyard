// parentseatwiring.go builds the review segments' parent seats from the run's location and config.
// The Discussion-Review and Code-Review segments each get one.
// The closures read the key, the history and the routing on a call, never at wire time;
// wire() also runs for status and pause.
// Each chair's await cap is resolved when its seat is built.

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

// askParentModule is the module of the settings that turn a review segment's parent seat on;
// discussionAskParentKey and codeAskParentKey are those settings.
const (
	askParentModule        = "loom"
	discussionAskParentKey = "discussion_review_ask_parent"
	codeAskParentKey       = "code_review_ask_parent"
)

// parentSeatInputs holds what the review segments' parent seats are built from.
type parentSeatInputs struct {
	location *lymcwd.Location
	cfg      loomengine.Config
	// runTimeout is shuttle's run_timeout_min and startAllowance its launch allowance.
	runTimeout     time.Duration
	startAllowance time.Duration
	// awaitCap returns the keep-warm cap of a perch's review chair.
	awaitCap       func(perch loomengine.Perch) time.Duration
	setting        func(module, key string) (any, error)
	statusPath     string
	statusLockPath string
}

// parentSeats returns the parent seat of each review perch that may ask the run's parent orch.
// Those are the discussion and code perches.
func parentSeats(in parentSeatInputs) map[loomengine.Perch]*shedadapters.ParentReviewSeat {
	return map[loomengine.Perch]*shedadapters.ParentReviewSeat{
		loomengine.PerchDiscussion: discussionParentSeat(in),
		loomengine.PerchCode:       codeParentSeat(in),
	}
}

// discussionParentSeat returns the Discussion-Review segment's parent seat over the run at in.location.
// It asks the parent to review the decision record, again once a Discussion-Write call rewrote it.
func discussionParentSeat(in parentSeatInputs) *shedadapters.ParentReviewSeat {
	seat := baseParentSeat(in, loomengine.PerchDiscussion, discussionAskParentKey)
	seat.Tag = shedadapters.DiscussionParentAskLineTag
	seat.Subject = "the decision record " + loomengine.DiscussionDecisionRecord(in.location)
	seat.RecordWrittenAt = func() (time.Time, error) {
		return latestCallAt(in.statusPath, in.statusLockPath, loomshed.NameDiscussionWrite)
	}
	return seat
}

// codeParentSeat returns the Code-Review segment's parent seat over the run at in.location.
// Every round's Bouncer asks the parent to review the task's code change in the task worktree.
// It asks once the round's fixer report exists and before it judges the round.
// The ask names that round's own review and fixer report, so the parent sees each ruling of the perch.
func codeParentSeat(in parentSeatInputs) *shedadapters.ParentReviewSeat {
	seat := baseParentSeat(in, loomengine.PerchCode, codeAskParentKey)
	seat.Tag = shedadapters.CodeParentAskLineTag
	seat.Subject = "the task's code change in the task worktree " + in.location.WorktreePath()
	seat.AskBeforeJudge = true
	// The Bouncer's judge waits for the parent inside its own session, which shuttle bounds by run_timeout_min.
	// The review chair's keep-warm cap stands in for the judge's await cap.
	awaitCap := in.awaitCap(loomengine.PerchCode)
	seat.Wait = func() (time.Duration, string) {
		return loomengine.CapParentJudgeWait(in.cfg, loomengine.PerchCode, in.runTimeout, in.startAllowance, awaitCap)
	}
	return seat
}

// baseParentSeat returns perch's parent seat with what every seat shares:
// the ask record, the slug, the setting key read on each call, the perch's capped wait and the routing.
// Its asks go to the orch NotifyParent's routing resolves, signed as lym's loom program and tagged with the pair's run.
func baseParentSeat(in parentSeatInputs, perch loomengine.Perch, key string) *shedadapters.ParentReviewSeat {
	location := in.location
	awaitCap := in.awaitCap(perch)
	return &shedadapters.ParentReviewSeat{
		Files:      burlerengine.ParentReviewFilesOf(location.AnchorPath()),
		Slug:       seedSlug(location.WorktreeName),
		AskEnabled: func() bool { return askParentSetting(in.setting, key) },
		Wait: func() (time.Duration, string) {
			return loomengine.CapParentReviewWait(in.cfg, perch, in.runTimeout, in.startAllowance, awaitCap)
		},
		Resolve: func() (string, bool, error) { return orchcli.ParentStrand(location) },
		Post: func(strand, line string) error {
			return orchcli.PostAsProgram(location, strand, loomProgram, line, creelengine.PostTags{Run: location.WorktreeName})
		},
	}
}

// askParentSetting reads loom's boolean setting key through setting.
// An unreadable value, or one that is not a boolean, is logged and read as false.
func askParentSetting(setting func(module, key string) (any, error), key string) bool {
	value, err := setting(askParentModule, key)
	if err != nil {
		logger.Warn("loomcli: the parent-review setting is unreadable; this round asks no parent", "key", key, "error", err)
		return false
	}
	on, ok := value.(bool)
	if !ok {
		logger.Warn("loomcli: the parent-review setting is not a boolean; this round asks no parent", "key", key, "value", value)
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
