// entries_batten.go implements the batten registry entries, which build the producers of the task-worktree lifecycle.
// They live in their own file.
// They share the Env.Slug, Env.ScratchDir and Env.PrimeLock validation shape that entries_simple.go does not have.

package shedrecipe

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// defaultInnerRunPollIntervalS is the poll_interval_s default, used when the configured value is zero.
// An absent key and an explicit zero both read as zero, so both resolve to this default.
// It matches the value the batten recipe sets explicitly, so an omitted key and the recipe's value never diverge.
// It is the interval at which the producer's in-call wait checks its child.
const defaultInnerRunPollIntervalS = 2

// defaultInnerRunNoticeProbeS is the notice_probe_s default, used when the configured value is zero.
// It matches the value the batten recipe sets explicitly.
// It is the longest the in-call wait goes between the reads that cost a process or a multiplexer round trip.
const defaultInnerRunNoticeProbeS = 30

// defaultInnerRunDriverExitGraceS is the driver_exit_grace_s default, used when the configured value is zero.
// It matches the value the batten recipe sets explicitly.
// It is the longest a done child's live driver strand is waited for before teardown ends it.
const defaultInnerRunDriverExitGraceS = 900

// worktreeCreateEntry is the constructor for the "WorktreeCreate" registry row.
// It rejects any Config key, and any Env value the producer needs that is unset.
func worktreeCreateEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
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
	return battenshed.NewWorktreeCreate(row.Name, env.Slug, env.CreateWorktree, env.PrimeLock, env.ScratchDir), nil
}

// worktreeTeardownEntry is the constructor for the "WorktreeTeardown" registry row.
// It rejects any Config key, and any Env value the producer needs that is unset.
func worktreeTeardownEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
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
	return battenshed.NewWorktreeTeardown(row.Name, env.Slug, env.Teardown, env.PrimeLock, env.ScratchDir), nil
}

// innerRunEntry is the constructor for the "InnerRun" registry row.
// It reads the optional int Config keys poll_interval_s, driver_exit_grace_s and notice_probe_s.
// For those three, a zero value takes the default and a negative value is an error naming the key.
// notice_quiet_min is validated as a non-negative int and passed to no producer:
// it is the quiet window of the run notices, which the hub wiring reads from this row.
// A poll_attempts key is rejected as unrecognized, since the wait lives in the producer's own call.
// It validates Env.Slug, Env.ScratchDir and the Env.InnerRun seams, except Env.InnerRun.Now.
// A nil Now is legitimate and selects the production clock.
// Apart from the interval, poll_interval_s's one effect is the sub-second floor warning of battenshed.NewInnerRun.
func innerRunEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
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

	driverExitGraceS, err := configInt(cfg, "driver_exit_grace_s", false)
	if err != nil {
		return nil, err
	}
	if driverExitGraceS < 0 {
		return nil, fmt.Errorf("shedrecipe: InnerRun: config key %q must not be negative, got %d", "driver_exit_grace_s", driverExitGraceS)
	}
	if driverExitGraceS == 0 {
		driverExitGraceS = defaultInnerRunDriverExitGraceS
	}

	noticeQuietMin, err := configInt(cfg, "notice_quiet_min", false)
	if err != nil {
		return nil, err
	}
	if noticeQuietMin < 0 {
		return nil, fmt.Errorf("shedrecipe: InnerRun: config key %q must not be negative, got %d", "notice_quiet_min", noticeQuietMin)
	}

	noticeProbeS, err := configInt(cfg, "notice_probe_s", false)
	if err != nil {
		return nil, err
	}
	if noticeProbeS < 0 {
		return nil, fmt.Errorf("shedrecipe: InnerRun: config key %q must not be negative, got %d", "notice_probe_s", noticeProbeS)
	}
	if noticeProbeS == 0 {
		noticeProbeS = defaultInnerRunNoticeProbeS
	}

	if err := configRejectUnknown(cfg, "poll_interval_s", "driver_exit_grace_s", "notice_quiet_min", "notice_probe_s"); err != nil {
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
	if err := requireSeam("InnerRun", "InnerRun.ReadDecision", env.InnerRun.ReadDecision); err != nil {
		return nil, err
	}
	if err := requireSeam("InnerRun", "InnerRun.DriverAlive", env.InnerRun.DriverAlive); err != nil {
		return nil, err
	}
	deps := env.InnerRun
	deps.NoticeProbe = time.Duration(noticeProbeS) * time.Second
	return battenshed.NewInnerRun(row.Name, env.Slug, deps, time.Duration(pollIntervalS)*time.Second, env.ScratchDir, time.Duration(driverExitGraceS)*time.Second), nil
}

// seedChildEntry is the constructor for the "SeedChild" registry row.
// It rejects any Config key, and any Env value the producer needs that is unset.
func seedChildEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("SeedChild", "Slug", env.Slug); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("SeedChild", "ScratchDir", env.ScratchDir); err != nil {
		return nil, err
	}
	if err := requireSeam("SeedChild", "SeedChild.ReadBoardType", env.SeedChild.ReadBoardType); err != nil {
		return nil, err
	}
	if err := requireSeam("SeedChild", "SeedChild.ChildDriver", env.SeedChild.ChildDriver); err != nil {
		return nil, err
	}
	if err := requireSeam("SeedChild", "SeedChild.WriteSeed", env.SeedChild.WriteSeed); err != nil {
		return nil, err
	}
	if err := requireSeam("SeedChild", "SeedChild.CommitSeed", env.SeedChild.CommitSeed); err != nil {
		return nil, err
	}
	if err := requireSeam("SeedChild", "SeedChild.PushSeed", env.SeedChild.PushSeed); err != nil {
		return nil, err
	}
	return battenshed.NewSeedChild(row.Name, env.Slug, env.SeedChild, env.ScratchDir), nil
}

// landEntry is the constructor for the "Land" registry row.
// It rejects any Config key, and any Env value the producer needs that is unset.
// It does not validate Env.Land.Sleep, Env.Land.Now or Env.Land.Notices, whose nil values are legitimate defaults.
func landEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireNonEmpty("Land", "Slug", env.Slug); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Land", "ScratchDir", env.ScratchDir); err != nil {
		return nil, err
	}
	if err := requireSeam("Land", "PrimeLock.Acquire", env.PrimeLock.Acquire); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Land", "PrimeLock.Path", env.PrimeLock.Path); err != nil {
		return nil, err
	}
	if err := requireSeam("Land", "Land.LandLock.Acquire", env.Land.LandLock.Acquire); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Land", "Land.LandLock.Path", env.Land.LandLock.Path); err != nil {
		return nil, err
	}
	for _, seam := range []struct {
		field string
		value any
	}{
		{"Land.ChildLanded", env.Land.ChildLanded},
		{"Land.ParentBranch", env.Land.ParentBranch},
		{"Land.PrimeBranch", env.Land.PrimeBranch},
		{"Land.PrimeDirty", env.Land.PrimeDirty},
		{"Land.Pull", env.Land.Pull},
		{"Land.Landing", env.Land.Landing},
		{"Land.RunDeploy", env.Land.RunDeploy},
		{"Land.PruneBoard", env.Land.PruneBoard},
		{"Land.PauseRequested", env.Land.PauseRequested},
	} {
		if err := requireSeam("Land", seam.field, seam.value); err != nil {
			return nil, err
		}
	}
	return battenshed.NewLand(row.Name, env.Slug, env.Land, env.PrimeLock, env.ScratchDir), nil
}
