// entries_simple.go implements the seven registry entries that take an empty Config and validate
// only the Env fields they read: preflightEntry, publishEntry, finalizeEntry, loomPreflightEntry,
// batchifierEntry, stubEntry, and websterEntry.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// preflightEntry is the Constructor for the "Preflight" registry row: it validates Env.Cwd and
// returns preflightshed.NewPreflight(name, env.Cwd).
func preflightEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Preflight", "Cwd", env.Cwd); err != nil {
		return nil, err
	}
	return preflightshed.NewPreflight(name, env.Cwd), nil
}

// publishEntry is the Constructor for the "Publish" registry row: it returns
// landingshed.NewPublish(env.Landing), wrapping that constructor's own error with this package's
// error-text prefix rather than surfacing it raw.
//
// name is deliberately discarded: landingshed.Deps carries no name field, and Publish's own
// identity is the package constant publishName. The coverage-guard test in
// internal/loomrecipe/coverage_guard_test.go is what pins this row's registry key ("Publish") to
// match that constant.
//
// It validates no Env field of its own: landingshed.NewPublish already rejects the nil closures in
// landingshed.Deps, so this entry inherits that check rather than duplicating it.
func publishEntry(_ string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	p, err := landingshed.NewPublish(env.Landing)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: Publish: %w", err)
	}
	return p, nil
}

// finalizeEntry is the Constructor for the "Finalize" registry row: publishEntry's twin over
// landingshed.NewFinalize(env.Landing).
//
// name is deliberately discarded, for the same reason as publishEntry: landingshed.Deps carries no
// name field, and Finalize's own identity is the package constant finalizeName. The coverage-guard
// test in internal/loomrecipe/coverage_guard_test.go is what pins this row's registry key
// ("Finalize") to match that constant.
//
// It validates no Env field of its own, for the same reason as publishEntry: landingshed.NewFinalize
// already rejects the nil closures in landingshed.Deps.
func finalizeEntry(_ string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	fz, err := landingshed.NewFinalize(env.Landing)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: Finalize: %w", err)
	}
	return fz, nil
}

// loomPreflightEntry is the Constructor for the "LoomPreflight" registry row: it validates
// Env.StatusPath and Env.StatusLockPath and returns
// loomshed.NewLoomPreflight(name, env.StatusPath, env.StatusLockPath).
func loomPreflightEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("LoomPreflight", "StatusPath", env.StatusPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("LoomPreflight", "StatusLockPath", env.StatusLockPath); err != nil {
		return nil, err
	}
	return loomshed.NewLoomPreflight(name, env.StatusPath, env.StatusLockPath), nil
}

// batchifierEntry is the Constructor for the "Batchifier" registry row: it validates
// Env.AnchorPath and returns loomshed.NewBatchifier(name, env.AnchorPath).
func batchifierEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Batchifier", "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	return loomshed.NewBatchifier(name, env.AnchorPath), nil
}

// stubEntry is the Constructor for the "Stub" registry row: it validates no Env field and returns
// loomshed.NewStub(name).
func stubEntry(name string, cfg Config, _ Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	return loomshed.NewStub(name), nil
}

// websterEntry is the Constructor for the "Webster" registry row: it resolves the row's
// "gate"/"gate_attempts" Config keys through resolveGateSpec onto RunDeps.Gate, validates
// Env.AnchorPath, Env.WebsterRun, Env.CommitWebster, and exactly four inner fields of
// Env.WebsterDeps -- Starter, Reed, Engine, and RefMatcher -- and returns
// loomshed.NewWebsterProducer(name, env.AnchorPath, env.WebsterRun, deps, env.CommitWebster).
//
// The gate is resolved here for the same reason the three already-gated rows resolve theirs here:
// one key means one thing at every gated site, and a reader of the recipe can see which validator
// guards each row without opening Go. The shipped "Webster" row carries no "gate" key, so
// resolveGateSpec returns the zero GateSpec and the row runs ungated exactly as before; naming one
// today fails loud through resolveGateSpec's own closed two-value vocabulary, since no Webster
// validator exists to name.
//
// It checks none of WebsterDeps' other nil-able fields, each for its own reason: Batcher is
// overwritten by loomshed's own wrapper on every Call, and that wrapper's own field doc says the
// caller leaves it nil; a nil Clock selects websterengine's production clock by design; a nil
// OpenBisector is a legitimate mode meaning "no fabric in this mode", not a missing value; and
// ShuttleCfg, Roles, Config, and Geom are value and map types whose validation belongs to
// websterengine.Run, not to this wiring layer.
func websterEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("Webster", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gate", "gate_attempts"); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Webster", "AnchorPath", env.AnchorPath); err != nil {
		return nil, err
	}
	if err := requireSeam("Webster", "WebsterRun", env.WebsterRun); err != nil {
		return nil, err
	}
	if err := requireSeam("Webster", "CommitWebster", env.CommitWebster); err != nil {
		return nil, err
	}
	if err := requireSeam("Webster", "WebsterDeps.Starter", env.WebsterDeps.Starter); err != nil {
		return nil, err
	}
	if err := requireSeam("Webster", "WebsterDeps.Reed", env.WebsterDeps.Reed); err != nil {
		return nil, err
	}
	if err := requireSeam("Webster", "WebsterDeps.Engine", env.WebsterDeps.Engine); err != nil {
		return nil, err
	}
	if err := requireSeam("Webster", "WebsterDeps.RefMatcher", env.WebsterDeps.RefMatcher); err != nil {
		return nil, err
	}
	deps := env.WebsterDeps
	deps.Gate = gate
	return loomshed.NewWebsterProducer(name, env.AnchorPath, env.WebsterRun, deps, env.CommitWebster), nil
}
