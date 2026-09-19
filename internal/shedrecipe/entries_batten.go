// entries_lifecycle.go implements the three lifecycle registry entries: worktreeCreateEntry,
// innerRunEntry, and worktreeTeardownEntry. They are grouped into their own file rather than folded
// into entries_simple.go because they share the Env.Slug/Env.ScratchDir/Env.PrimeLock validation
// shape that entries_simple.go's nine entries do not have.

package shedrecipe

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/lifecycleshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// defaultInnerRunPollIntervalS and defaultInnerRunPollAttempts are innerRunEntry's own defaults for
// the poll_interval_s and poll_attempts Config keys, used when the extracted value is zero --
// configInt reports an absent key and an explicit zero identically, so both resolve to these
// defaults. The twelve-hour default the attempt count expresses at the default interval is
// deliberately generous: a real task run spans hours.
const (
	defaultInnerRunPollIntervalS = 5
	defaultInnerRunPollAttempts  = 8640
)

// worktreeCreateEntry is the Constructor for the "WorktreeCreate" registry row: it validates
// Env.Slug, Env.ScratchDir, Env.CreateWorktree, Env.PrimeLock.Acquire, and Env.PrimeLock.Path, and
// returns lifecycleshed.NewWorktreeCreate(name, env.Slug, env.CreateWorktree, env.PrimeLock,
// env.ScratchDir).
func worktreeCreateEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("WorktreeCreate", "Slug", env.Slug); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("WorktreeCreate", "ScratchDir", env.ScratchDir); err != nil {
		return nil, err
	}
	if err := requireSeam("WorktreeCreate", "CreateWorktree", env.CreateWorktree); err != nil {
		return nil, err
	}
	if err := requireSeam("WorktreeCreate", "PrimeLock.Acquire", env.PrimeLock.Acquire); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("WorktreeCreate", "PrimeLock.Path", env.PrimeLock.Path); err != nil {
		return nil, err
	}
	return lifecycleshed.NewWorktreeCreate(name, env.Slug, env.CreateWorktree, env.PrimeLock, env.ScratchDir), nil
}

// worktreeTeardownEntry is the Constructor for the "WorktreeTeardown" registry row: worktreeCreateEntry's
// twin, validating the same Slug/ScratchDir/PrimeLock fields under the "WorktreeTeardown" entry name,
// plus Env.Teardown.Shutdown and Env.Teardown.Remove, and returns
// lifecycleshed.NewWorktreeTeardown(name, env.Slug, env.Teardown, env.PrimeLock, env.ScratchDir).
func worktreeTeardownEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("WorktreeTeardown", "Slug", env.Slug); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("WorktreeTeardown", "ScratchDir", env.ScratchDir); err != nil {
		return nil, err
	}
	if err := requireSeam("WorktreeTeardown", "Teardown.Shutdown", env.Teardown.Shutdown); err != nil {
		return nil, err
	}
	if err := requireSeam("WorktreeTeardown", "Teardown.Remove", env.Teardown.Remove); err != nil {
		return nil, err
	}
	if err := requireSeam("WorktreeTeardown", "PrimeLock.Acquire", env.PrimeLock.Acquire); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("WorktreeTeardown", "PrimeLock.Path", env.PrimeLock.Path); err != nil {
		return nil, err
	}
	return lifecycleshed.NewWorktreeTeardown(name, env.Slug, env.Teardown, env.PrimeLock, env.ScratchDir), nil
}

// innerRunEntry is the Constructor for the "InnerRun" registry row: it reads the optional int
// Config keys poll_interval_s and poll_attempts through configInt, defaulting to
// defaultInnerRunPollIntervalS and defaultInnerRunPollAttempts respectively when the extracted
// value is zero -- configInt reports an absent key and an explicit zero identically, so both
// resolve to the same default -- and rejects a negative value for either key with an error naming
// that key. It validates Env.Slug, Env.ScratchDir, and Env.InnerRun.Spawn/ResolveStatus/ReadStatus
// -- and neither Env.InnerRun.Now nor Env.InnerRun.Sleep, whose nil values are legitimate and
// select the production clock and sleep. It returns lifecycleshed.NewInnerRun(name, env.Slug,
// env.InnerRun, time.Duration(pollIntervalS)*time.Second, pollAttempts, env.ScratchDir).
func innerRunEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	pollIntervalS, err := configInt(cfg, "poll_interval_s", false)
	if err != nil {
		return nil, err
	}
	if pollIntervalS < 0 {
		return nil, fmt.Errorf("shedrecipe: InnerRun: config key %q must not be negative, got %d", "poll_interval_s", pollIntervalS)
	}
	if pollIntervalS == 0 {
		pollIntervalS = defaultInnerRunPollIntervalS
	}

	pollAttempts, err := configInt(cfg, "poll_attempts", false)
	if err != nil {
		return nil, err
	}
	if pollAttempts < 0 {
		return nil, fmt.Errorf("shedrecipe: InnerRun: config key %q must not be negative, got %d", "poll_attempts", pollAttempts)
	}
	if pollAttempts == 0 {
		pollAttempts = defaultInnerRunPollAttempts
	}

	if err := configRejectUnknown(cfg, "poll_interval_s", "poll_attempts"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("InnerRun", "Slug", env.Slug); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("InnerRun", "ScratchDir", env.ScratchDir); err != nil {
		return nil, err
	}
	if err := requireSeam("InnerRun", "InnerRun.Spawn", env.InnerRun.Spawn); err != nil {
		return nil, err
	}
	if err := requireSeam("InnerRun", "InnerRun.ResolveStatus", env.InnerRun.ResolveStatus); err != nil {
		return nil, err
	}
	if err := requireSeam("InnerRun", "InnerRun.ReadStatus", env.InnerRun.ReadStatus); err != nil {
		return nil, err
	}
	return lifecycleshed.NewInnerRun(name, env.Slug, env.InnerRun, time.Duration(pollIntervalS)*time.Second, pollAttempts, env.ScratchDir), nil
}
