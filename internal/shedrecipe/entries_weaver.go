// entries_weaver.go implements weaverEntry, the Constructor of the weaver implementer.
// The "Implementer" registry row builds it when its setting names weaver.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// weaverConfigWayForward closes every refusal about weaver.yaml with the command that edits it.
const weaverConfigWayForward = "way forward: fix weaver.yaml with `lym config weaver`"

// weaverEntry is the Constructor of the weaver implementer.
// It resolves the row's "gates" Config key through resolveGateSpec onto the gate field of Env.WeaverDeps.
// It validates Env.ConfigDirs.Anchor and Env.WeaverRun.
// It also validates the Runner, Fixer, Reed, CommitRecords and RecordsHead seams of Env.WeaverDeps.
// It loads weaver.yaml under Env.ConfigDirs and resolves its ladder against Env.Models.
// An empty ladder, a spec that does not resolve and a config that does not load each refuse when the row builds.
// Each refusal names `lym config weaver`.
// The returned producer wraps the row's name, Env.ConfigDirs, Env.WeaverRun, the filled deps and Env.ImplementerSeam.
//
// It checks none of WeaverDeps' other nil-able fields.
// Batcher is overwritten by the producer on every Call.
// Verify, Clock, Deliver and ParentBranch have documented nil defaults.
// Geom is a value type whose validation belongs to weaverengine.Run, not to this wiring layer.
func weaverEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	gate, err := resolveGateSpec("Weaver", cfg, env)
	if err != nil {
		return nil, err
	}
	if err := configRejectUnknown(cfg, "gates"); err != nil {
		return nil, err
	}
	if err := requireAbsRoot("Weaver", "ConfigDirs.Anchor", env.ConfigDirs.Anchor); err != nil {
		return nil, err
	}
	required := []struct {
		field string
		seam  any
	}{
		{"WeaverRun", env.WeaverRun},
		{"WeaverDeps.Runner", env.WeaverDeps.Runner},
		{"WeaverDeps.Fixer", env.WeaverDeps.Fixer},
		{"WeaverDeps.Reed", env.WeaverDeps.Reed},
		{"WeaverDeps.CommitRecords", env.WeaverDeps.CommitRecords},
		{"WeaverDeps.RecordsHead", env.WeaverDeps.RecordsHead},
	}
	for _, r := range required {
		if err := requireSeam("Weaver", r.field, r.seam); err != nil {
			return nil, err
		}
	}

	weaverCfg, err := weaverengine.LoadConfig(env.ConfigDirs, "weaver")
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: Weaver: %w; %s", err, weaverConfigWayForward)
	}
	ladder, err := weaverengine.ResolveLadder(weaverCfg, env.Models)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: Weaver: %w", err)
	}

	deps := env.WeaverDeps
	deps.Gate = gate
	deps.Config = weaverCfg
	deps.Ladder = ladder
	return loomshed.NewWeaverProducer(row.Name, env.ConfigDirs, env.WeaverRun, deps, env.ImplementerSeam), nil
}
