// entries_simple.go implements the constructors of the registry rows that take at most one Config key:
// preflightEntry, parentAlignEntry, finalizeEntry, frictionReflectEntry, stubEntry, and websterEntry.

package shedrecipe

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// preflightEntry is the Constructor for the "Preflight" registry row.
// It validates Env.Cwd and returns preflightshed.NewPreflightWithChecks(name, env.Cwd, checks);
// its checks are env.PreflightBatcher, env.PreflightStencils and env.PreflightDrift.
func preflightEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Preflight", "Cwd", env.Cwd); err != nil {
		return nil, err
	}
	return preflightshed.NewPreflightWithChecks(row.Name, env.Cwd, preflightshed.Checks{Batcher: env.PreflightBatcher, Stencils: env.PreflightStencils, Drift: env.PreflightDrift}), nil
}

// landingRowDeps returns a copy of env.Landing whose GateKey is the key the row's optional "gate_key" string names.
// When the row names no key, or an empty one, GateKey is the row name in lower case.
// The copy keeps Env run-wide: each landing row sets its own key on its own Deps.
// A non-string "gate_key" is refused naming the entry, the row, the key and the way forward.
// "gate_key" selects only which key of the hub's gate.yaml the row runs,
// and the key's tiers and scope stay hub-file only.
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

// parentAlignEntry is the Constructor for the "ParentAlign" registry row.
// It returns landingshed.NewParentAlign over a copy of env.Landing whose GateKey is the row's,
// and it wraps that constructor's own error with this package's error-text prefix rather than surfacing it raw.
//
// The optional "gate_key" string names the key of the hub's gate.yaml the row runs after its merge-in.
// Without it, or with an empty one, the key is the row name in lower case.
//
// It validates no Env field of its own:
// landingshed.NewParentAlign already rejects the nil seams and the empty merge record path it requires.
func parentAlignEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg, "gate_key"); err != nil {
		return nil, err
	}
	deps, err := landingRowDeps("ParentAlign", row, cfg, env)
	if err != nil {
		return nil, err
	}
	m, err := landingshed.NewParentAlign(deps)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: ParentAlign: %w", err)
	}
	return m, nil
}

// finalizeEntry is the Constructor for the "Finalize" registry row.
// It returns landingshed.NewFinalize over a copy of env.Landing whose GateKey is the row's,
// and it wraps that constructor's own error with this package's error-text prefix rather than surfacing it raw.
//
// The optional "gate_key" string names the key of the hub's gate.yaml the row runs after its merge-in.
// Without it, or with an empty one, the key is the row name in lower case.
// Finalize's own identity is the constant finalizeName in internal/landingshed.
// The coverage-guard test in internal/loomrecipe/coverage_guard_test.go pins the registry key of this row:
// the registry key "Finalize" must match that constant.
//
// It validates no Env field of its own:
// landingshed.NewFinalize already rejects the nil closures in landingshed.Deps.
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

// frictionReflectEntry is the Constructor for the "FrictionReflect" registry row.
// It returns loomshed.NewFrictionReflect(name, env.ReflectFriction),
// and it wraps that constructor's own error with this package's error-text prefix.
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

// stubEntry is the Constructor for the "Stub" registry row.
// It validates no Env field and returns loomshed.NewStub(name).
func stubEntry(row RowIdentity, cfg Config, _ Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	return loomshed.NewStub(row.Name), nil
}

// websterEntry is the Constructor of the webster implementer.
// The "Implementer" registry row builds it when its setting names webster.
// It resolves the row's "gates" Config key through resolveGateSpec onto the gate field of Env.WebsterDeps.
// It validates Env.ConfigDirs.Anchor, Env.WebsterRun and Env.CommitWebster;
// it also validates the Starter, Stopper, Engines and RefMatcher fields of Env.WebsterDeps.
// It returns loomshed.NewWebsterProducer(name, env.ConfigDirs, env.WebsterRun, deps, env.CommitWebster).
//
// The gate is resolved here for the same reason the other gated rows resolve theirs here:
// one key means one thing at every gated site,
// and a reader of the recipe can see which validator guards each row without opening Go.
// The shipped "Webster" row carries no "gates" key, so resolveGateSpec returns the zero GateSpec.
// `websterengine.Run` adds the plan-level `verify` entry to the Webster spec itself;
// it refuses a `deps.Gate` that already names `verify`, so a recipe row cannot add a second one beside it.
//
// It checks none of WebsterDeps' other nil-able fields, each for its own reason:
// Batcher is overwritten by loomshed's own wrapper on every Call,
// and that wrapper's own field doc says the caller leaves it nil;
// ShuttleCfg, Roles, Config, and Geom are value and map types;
// their validation belongs to websterengine.Run, not to this wiring layer.
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
