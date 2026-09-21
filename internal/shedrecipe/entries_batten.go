// entries_batten.go implements the four batten registry entries: worktreeCreateEntry,
// innerRunEntry, seedChildEntry, and worktreeTeardownEntry. They are grouped into their own file
// rather than folded into entries_simple.go because they share the
// Env.Slug/Env.ScratchDir/Env.PrimeLock validation shape that entries_simple.go's nine entries do
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
// poll_interval_s so an omitted key and the recipe's own value never diverge; the wait budget
// itself now lives on the recipe row's own max_bounces, not on a Go constant, since InnerRun's
// bounce loop lives in shedengine's own on_stuck routing rather than inside this producer.
const defaultInnerRunPollIntervalS = 30

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

// innerRunEntry is the Constructor for the "InnerRun" registry row: it reads the optional int
// Config key poll_interval_s through configInt, defaulting to defaultInnerRunPollIntervalS when
// the extracted value is zero -- configInt reports an absent key and an explicit zero identically,
// so both resolve to the same default -- and rejects a negative value with an error naming the
// key. poll_attempts is retired: the wait budget now lives on the recipe row's own max_bounces,
// read by shedengine itself, not on a Config key this entry reads, so poll_attempts is rejected as
// an unrecognised key rather than silently read. It validates Env.Slug, Env.ScratchDir, and
// Env.InnerRun.Spawn/ResolveStatus/ReadStatus -- and not Env.InnerRun.Sleep, whose nil value is
// legitimate and selects the production sleep.
// It returns battenshed.NewInnerRun(name, env.Slug, env.InnerRun,
// time.Duration(pollIntervalS)*time.Second, env.ScratchDir).
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

	if err := configRejectUnknown(cfg, "poll_interval_s"); err != nil {
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
	return battenshed.NewInnerRun(name, env.Slug, env.InnerRun, time.Duration(pollIntervalS)*time.Second, env.ScratchDir), nil
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
