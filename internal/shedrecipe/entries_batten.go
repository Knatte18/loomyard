// entries_batten.go implements the batten registry entries, the ones that build a producer of the task-worktree lifecycle.
// They are grouped into their own file
// rather than folded into entries_simple.go because they share the
// Env.Slug/Env.ScratchDir/Env.PrimeLock validation shape that entries_simple.go's entries do
// not have.

package shedrecipe

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// defaultInnerRunPollIntervalS is innerRunEntry's own default for the poll_interval_s Config key,
// used when the extracted value is zero -- configInt reports an absent key and an explicit zero
// identically, so both resolve to this default. It agrees with the batten recipe's own explicit
// poll_interval_s so an omitted key and the recipe's own value never diverge.
// It is the interval at which the producer's in-call wait checks its child.
const defaultInnerRunPollIntervalS = 2

// defaultInnerRunNoticeProbeS is innerRunEntry's own default for the notice_probe_s Config key, used when the extracted value is zero, exactly as defaultInnerRunPollIntervalS is.
// It agrees with the batten recipe's own explicit notice_probe_s: the longest the in-call wait goes between the reads that cost a process or a multiplexer round trip.
const defaultInnerRunNoticeProbeS = 30

// defaultInnerRunDriverExitGraceS is innerRunEntry's own default for the driver_exit_grace_s Config key, used when the extracted value is zero, exactly as defaultInnerRunPollIntervalS is.
// It agrees with the batten recipe's own explicit driver_exit_grace_s: 900 seconds, the longest a done child's live driver strand is waited for before teardown ends it.
const defaultInnerRunDriverExitGraceS = 900

// defaultInnerRunNoticeQuietMin is innerRunEntry's own default for the notice_quiet_min Config key, in minutes, used when the extracted value is zero, exactly as defaultInnerRunPollIntervalS is.
// It agrees with the batten recipe's own explicit notice_quiet_min.
const defaultInnerRunNoticeQuietMin = 30

// worktreeCreateEntry is the Constructor for the "WorktreeCreate" registry row: it validates
// Env.Slug, Env.ScratchDir, Env.CreateWorktree, Env.PrimeLock.Acquire, and Env.PrimeLock.Path, and
// returns battenshed.NewWorktreeCreate(name, env.Slug, env.CreateWorktree, env.PrimeLock,
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
	return battenshed.NewWorktreeCreate(name, env.Slug, env.CreateWorktree, env.PrimeLock, env.ScratchDir), nil
}

// worktreeTeardownEntry is the Constructor for the "WorktreeTeardown" registry row: worktreeCreateEntry's
// twin, validating the same Slug/ScratchDir/PrimeLock fields under the "WorktreeTeardown" entry name,
// plus Env.Teardown.Shutdown and Env.Teardown.Remove, and returns
// battenshed.NewWorktreeTeardown(name, env.Slug, env.Teardown, env.PrimeLock, env.ScratchDir).
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
	return battenshed.NewWorktreeTeardown(name, env.Slug, env.Teardown, env.PrimeLock, env.ScratchDir), nil
}

// innerRunEntry is the Constructor for the "InnerRun" registry row: it reads the optional int Config key poll_interval_s through configInt, defaulting to defaultInnerRunPollIntervalS when the extracted value is zero -- configInt reports an absent key and an explicit zero identically, so both resolve to the same default -- and rejects a negative value with an error naming the key.
// It reads driver_exit_grace_s the same way, defaulting to defaultInnerRunDriverExitGraceS, and notice_quiet_min likewise, defaulting to defaultInnerRunNoticeQuietMin and set on the deps as InnerRunDeps.NoticeQuiet.
// It reads notice_probe_s the same way, defaulting to defaultInnerRunNoticeProbeS and set on the deps as InnerRunDeps.NoticeProbe.
// poll_attempts is retired: the wait lives in the producer's own call, not on a Config key this entry reads, so poll_attempts is rejected as an unrecognised key rather than silently read.
// It validates Env.Slug, Env.ScratchDir, and Env.InnerRun.Spawn/ResolveStatus/ReadStatus/ReadDecision/DriverAlive -- and not Env.InnerRun.Sleep or Env.InnerRun.Now, whose nil values are legitimate and select the production sleep and clock.
// It returns battenshed.NewInnerRun(name, env.Slug, env.InnerRun, time.Duration(pollIntervalS)*time.Second, env.ScratchDir, time.Duration(driverExitGraceS)*time.Second).
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
	if noticeQuietMin == 0 {
		noticeQuietMin = defaultInnerRunNoticeQuietMin
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
	deps.NoticeQuiet = time.Duration(noticeQuietMin) * time.Minute
	deps.NoticeProbe = time.Duration(noticeProbeS) * time.Second
	return battenshed.NewInnerRun(name, env.Slug, deps, time.Duration(pollIntervalS)*time.Second, env.ScratchDir, time.Duration(driverExitGraceS)*time.Second), nil
}

// seedChildEntry is the Constructor for the "SeedChild" registry row: it validates Env.Slug,
// Env.ScratchDir, and all five Env.SeedChild closures, and returns
// battenshed.NewSeedChild(name, env.Slug, env.SeedChild, env.ScratchDir).
func seedChildEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
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
	return battenshed.NewSeedChild(name, env.Slug, env.SeedChild, env.ScratchDir), nil
}

// landEntry is the Constructor for the "Land" registry row: it rejects any Config key.
// It validates Env.Slug, Env.ScratchDir, Env.PrimeLock.Acquire, Env.PrimeLock.Path, Env.Land.LandLock.Acquire, Env.Land.LandLock.Path and the Env.Land seams ParentBranch, PrimeBranch, PrimeDirty, Pull, Landing, RunDeploy, ReadyTasks, PruneBoard and PauseRequested.
// It returns battenshed.NewLand(name, env.Slug, env.Land, env.PrimeLock, env.ScratchDir).
// It does not validate Env.Land.Sleep, Env.Land.Now or Env.Land.Notices, whose nil values are legitimate defaults.
func landEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
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
		{"Land.ParentBranch", env.Land.ParentBranch},
		{"Land.PrimeBranch", env.Land.PrimeBranch},
		{"Land.PrimeDirty", env.Land.PrimeDirty},
		{"Land.Pull", env.Land.Pull},
		{"Land.Landing", env.Land.Landing},
		{"Land.RunDeploy", env.Land.RunDeploy},
		{"Land.ReadyTasks", env.Land.ReadyTasks},
		{"Land.PruneBoard", env.Land.PruneBoard},
		{"Land.PauseRequested", env.Land.PauseRequested},
	} {
		if err := requireSeam("Land", seam.field, seam.value); err != nil {
			return nil, err
		}
	}
	return battenshed.NewLand(name, env.Slug, env.Land, env.PrimeLock, env.ScratchDir), nil
}
