// roles.go implements the role-resolution pre-flight: mapping builder.yaml's
// four role model-spec strings onto their resolved model-spec Resolved
// values, once, before any agent spawns. `run` and `spawn-batch` call
// ResolveRoles at entry so a typo'd alias in builder.yaml fails loud before
// any implementer or orchestrator session is started, never hours into a
// run when that role first spawns.

package builderengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/modelspec"
)

// Role names one of builder.yaml's four model-spec roles. The string value
// is both the role's builder.yaml key and its display name in error
// messages.
type Role string

// The four builder roles, per docs/reference/model-spec.md's "Roles that
// use this notation" section.
const (
	// RoleOrchestrator is the long-lived orchestrator session that drives
	// the batch loop.
	RoleOrchestrator Role = "orchestrator"
	// RoleImplementer is a normal-sized batch's implementer spawn.
	RoleImplementer Role = "implementer"
	// RoleImplementerOversized is an oversized-flagged batch's implementer
	// spawn.
	RoleImplementerOversized Role = "implementer_oversized"
	// RoleRecovery is the fresh escalated recovery spawn the orchestrator
	// triggers after a batch reports stuck.
	RoleRecovery Role = "recovery"
)

// ResolveRoles parses and resolves every one of cfg's four role model-spec
// strings against reg, returning the resolved value keyed by Role. This is
// the fail-pre-flight surface `run`/`spawn-batch` call at entry: a
// well-formed but unknown alias (a typo'd role spec) fails here, before any
// agent spawns, rather than surfacing only when that role's spawn site
// first reaches it. Any Parse or Resolve failure is wrapped naming the
// offending role, since cfg's four fields carry no name of their own once
// extracted.
//
// The Resolved→shuttleengine.Spec field mapping (spec.Model = resolved.Model;
// spec.Effort = resolved.Params["effort"]; spec.Version =
// resolved.Params["version"]) happens at each spawn site, not here — this
// function only resolves and returns, per modelspec's documented consumer
// mapping.
func ResolveRoles(cfg Config, reg modelspec.Registry) (map[Role]modelspec.Resolved, error) {
	specsByRole := map[Role]string{
		RoleOrchestrator:         cfg.Orchestrator,
		RoleImplementer:          cfg.Implementer,
		RoleImplementerOversized: cfg.ImplementerOversized,
		RoleRecovery:             cfg.Recovery,
	}

	resolved := make(map[Role]modelspec.Resolved, len(specsByRole))
	for role, specStr := range specsByRole {
		spec, err := modelspec.Parse(specStr)
		if err != nil {
			return nil, fmt.Errorf("builder: role %q: %w", role, err)
		}
		r, err := reg.Resolve(spec)
		if err != nil {
			return nil, fmt.Errorf("builder: role %q: %w", role, err)
		}
		resolved[role] = r
	}

	return resolved, nil
}
