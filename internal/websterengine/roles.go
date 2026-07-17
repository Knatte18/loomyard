// roles.go implements the role-resolution pre-flight: mapping webster.yaml's
// three role model-spec strings onto their resolved model-spec Resolved
// values, once, before any agent spawns. `run` calls ResolveRoles at entry
// so a typo'd alias in webster.yaml fails loud before Master ever starts,
// never mid-run when a role first spawns.

package websterengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/modelspec"
)

// Role names one of webster.yaml's three model-spec roles. The string value
// is both the role's webster.yaml key and its display name in error
// messages.
//
// Unlike builder, webster has no per-fork implementer roles: an in-session
// Agent-tool fork always inherits Master's current model — there is no
// mechanism to select a model for an individual fork. Batch-level model
// choice is therefore expressed by switching Master itself (RoleMaster vs.
// RoleMasterOversized), and only the cold recovery strand — a genuinely
// separate process, not a fork — carries its own role.
type Role string

// The three webster roles, per docs/reference/model-spec.md's "Roles that
// use this notation" section.
const (
	// RoleMaster is the long-lived Master session that reads the plan once
	// and forks one implementer per batch in-session.
	RoleMaster Role = "master"
	// RoleMasterOversized is the model begin-batch injects into Master's
	// pane for a batch flagged oversized: true; forks inherit whichever
	// model Master is currently running, so this is what an oversized
	// batch's fork actually runs at.
	RoleMasterOversized Role = "master_oversized"
	// RoleRecovery is the cold, fresh recovery strand recover-batch spawns
	// when a fork reports stuck or writes no report.
	RoleRecovery Role = "recovery"
)

// ResolveRoles parses and resolves every one of cfg's three role model-spec
// strings against reg, returning the resolved value keyed by Role. This is
// the fail-pre-flight surface `run` calls at entry: a well-formed but
// unknown alias (a typo'd role spec) fails here, before Master ever spawns,
// rather than surfacing only when that role's spawn or injection site first
// reaches it. Any Parse or Resolve failure is wrapped naming the offending
// role, since cfg's three fields carry no name of their own once extracted.
//
// The Resolved→shuttleengine.Spec field mapping (spec.Model = resolved.Model;
// spec.Effort = resolved.Params["effort"]; spec.Version =
// resolved.Params["version"]) happens at each spawn/inject site, not here —
// this function only resolves and returns, per modelspec's documented
// consumer mapping.
func ResolveRoles(cfg Config, reg modelspec.Registry) (map[Role]modelspec.Resolved, error) {
	specsByRole := map[Role]string{
		RoleMaster:          cfg.Master,
		RoleMasterOversized: cfg.MasterOversized,
		RoleRecovery:        cfg.Recovery,
	}

	resolved := make(map[Role]modelspec.Resolved, len(specsByRole))
	for role, specStr := range specsByRole {
		spec, err := modelspec.Parse(specStr)
		if err != nil {
			return nil, fmt.Errorf("webster: role %q: %w", role, err)
		}
		r, err := reg.Resolve(spec)
		if err != nil {
			return nil, fmt.Errorf("webster: role %q: %w", role, err)
		}
		resolved[role] = r
	}

	return resolved, nil
}
