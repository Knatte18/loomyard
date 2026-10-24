// entries_simple.go implements the registry entries that take an empty Config and validate only the
// Env fields they read: preflightEntry, mergeInEntry, finalizeEntry, frictionReflectEntry,
// loomPreflightEntry, stubEntry, and websterEntry, the webster implementer.

package shedrecipe

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// preflightEntry is the Constructor for the "Preflight" registry row: it validates Env.Cwd and
// returns preflightshed.NewPreflight(name, env.Cwd).
func preflightEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Preflight", "Cwd", env.Cwd); err != nil {
		return nil, err
	}
	return preflightshed.NewPreflight(row.Name, env.Cwd), nil
}

// landingRowDeps returns a copy of env.Landing whose GateKey is the key the row's optional "gate_key" string names, else the row name in lower case.
// The copy keeps Env run-wide: each landing row sets its own key on its own Deps.
// A non-string "gate_key" is refused naming the entry, the row, the key and the way forward.
// "gate_key" selects only which key of the hub's gate.yaml the row runs, and the key's tiers and scope stay hub-file only.
func landingRowDeps(entry string, row RowIdentity, cfg Config, env Env) (landingshed.Deps, error) {
	gateKey := strings.ToLower(row.Name)
	if raw, ok := cfg["gate_key"]; ok {
		named, isString := raw.(string)
		if !isString {
			return landingshed.Deps{}, fmt.Errorf("shedrecipe: %s: row %q config key %q must be a string, got %T; way forward: set %q to the name of a gate.yaml key, a string, or remove it to run the key named for the row", entry, row.Name, "gate_key", raw, "gate_key")
		}
		if named != "" {
			gateKey = named
		}
	}
	deps := env.Landing
	deps.GateKey = gateKey
	return deps, nil
}

// mergeInEntry is the Constructor for the "MergeIn" registry row: it returns landingshed.NewMergeIn over a copy of env.Landing whose GateKey is the row's, wrapping that constructor's own error with this package's error-text prefix rather than surfacing it raw.
//
// The optional "gate_key" string names the key of the hub's gate.yaml the row runs after its merge-in, else the row name in lower case.
//
// It validates no Env field of its own: landingshed.NewMergeIn already rejects the nil seams and the empty merge record path it requires.
func mergeInEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg, "gate_key"); err != nil {
		return nil, err
	}
	deps, err := landingRowDeps("MergeIn", row, cfg, env)
	if err != nil {
		return nil, err
	}
	m, err := landingshed.NewMergeIn(deps)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: MergeIn: %w", err)
	}
	return m, nil
}

// finalizeEntry is the Constructor for the "Finalize" registry row: it returns landingshed.NewFinalize over a copy of env.Landing whose GateKey is the row's, wrapping that constructor's own error with this package's error-text prefix rather than surfacing it raw.
//
// The optional "gate_key" string names the key of the hub's gate.yaml the row runs after its merge-in, else the row name in lower case.
// Finalize's own identity is the package constant finalizeName.
// The coverage-guard test in internal/loomrecipe/coverage_guard_test.go is what pins this row's registry key ("Finalize") to match that constant.
//
// It validates no Env field of its own: landingshed.NewFinalize already rejects the nil closures in landingshed.Deps.
func finalizeEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg, "gate_key"); err != nil {
		return nil, err
	}
	deps, err := landingRowDeps("Finalize", row, cfg, env)
	if err != nil {
		return nil, err
	}
	fz, err := landingshed.NewFinalize(deps)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: Finalize: %w", err)
	}
	return fz, nil
}

// frictionReflectEntry is the Constructor for the "FrictionReflect" registry row: it returns
// loomshed.NewFrictionReflect(name, env.ReflectFriction), wrapping that constructor's own error with
// this package's error-text prefix.
//
// It validates no Env field of its own and inherits loomshed.NewFrictionReflect's nil-closure refusal.
func frictionReflectEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	p, err := loomshed.NewFrictionReflect(row.Name, env.ReflectFriction)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: FrictionReflect: %w", err)
	}
	return p, nil
}

// loomPreflightEntry is the Constructor for the "LoomPreflight" registry row.
// It validates Env.StatusPath, Env.StatusLockPath and Env.ConfigDirs.Anchor and returns
// loomshed.NewLoomPreflight(name, env.StatusPath, env.StatusLockPath, env.ConfigDirs).
func loomPreflightEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("LoomPreflight", "StatusPath", env.StatusPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("LoomPreflight", "StatusLockPath", env.StatusLockPath); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("LoomPreflight", "ConfigDirs.Anchor", env.ConfigDirs.Anchor); err != nil {
		return nil, err
	}
	return loomshed.NewLoomPreflight(row.Name, env.StatusPath, env.StatusLockPath, env.ConfigDirs), nil
}

// stubEntry is the Constructor for the "Stub" registry row: it validates no Env field and returns
// loomshed.NewStub(name).
func stubEntry(row RowIdentity, cfg Config, _ Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	return loomshed.NewStub(row.Name), nil
}

// websterEntry is the Constructor of the webster implementer, which the "Implementer" registry row builds when its setting names webster: it resolves the row's "gates" Config key through resolveGateSpec onto RunDeps.Gate, validates Env.ConfigDirs.Anchor, Env.WebsterRun, Env.CommitWebster, and exactly four inner fields of Env.WebsterDeps -- Starter, Stopper, Engine, and RefMatcher -- and returns loomshed.NewWebsterProducer(name, env.ConfigDirs, env.WebsterRun, deps, env.CommitWebster).
//
// The gate is resolved here for the same reason the three already-gated rows resolve theirs here:
// one key means one thing at every gated site, and a reader of the recipe can see which validator
// guards each row without opening Go.
// The shipped "Webster" row carries no "gates" key, so resolveGateSpec returns the zero GateSpec.
// `websterengine.Run` adds the plan-level `verify` entry to Merriam's spec itself and refuses a `deps.Gate` that already names `verify`,
// so a recipe row cannot add a second one beside it.
//
// It checks none of WebsterDeps' other nil-able fields, each for its own reason: Batcher is
// overwritten by loomshed's own wrapper on every Call, and that wrapper's own field doc says the
// caller leaves it nil; and ShuttleCfg, Roles, Config, and Geom are value and map types whose validation belongs to
// websterengine.Run, not to this wiring layer.
func websterEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("Webster", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Webster", "ConfigDirs.Anchor", env.ConfigDirs.Anchor); err != nil {
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
	if err := requireSeam("Webster", "WebsterDeps.Stopper", env.WebsterDeps.Stopper); err != nil {
		return nil, err
	}
	if err := requireSeam("Webster", "WebsterDeps.Engines", env.WebsterDeps.Engines); err != nil {
		return nil, err
	}
	if err := requireSeam("Webster", "WebsterDeps.RefMatcher", env.WebsterDeps.RefMatcher); err != nil {
		return nil, err
	}
	deps := env.WebsterDeps
	deps.Gate = gate
	return loomshed.NewWebsterProducer(row.Name, env.ConfigDirs, env.WebsterRun, deps, env.CommitWebster), nil
}
