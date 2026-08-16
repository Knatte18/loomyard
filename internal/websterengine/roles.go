// roles.go implements the role-resolution pre-flight: mapping webster.yaml's two role model-spec
// strings onto their resolved model-spec Resolved values, once, before any agent spawns. `run`
// calls ResolveRoles at entry so a typo'd alias in webster.yaml fails loud before Master ever
// starts, never mid-run when a role first spawns.

package websterengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/modelspec"
)

// Role names one of webster.yaml's two model-spec roles.
// Only the cold recovery strand carries its own role;
// in-session forks always inherit Master's current model.
type Role string

// The two webster roles, per contracts/specs/llm-model-spec.md's "Roles that use this notation" section.
const (
	// RoleMaster is the long-lived Master session that reads the plan once and forks one implementer
	// per batch in-session.
	RoleMaster Role = "master"
	// RoleRecovery is the cold, fresh recovery strand recover-batch spawns when a fork reports stuck
	// or writes no report.
	RoleRecovery Role = "recovery"
)

// ResolveRoles parses and resolves cfg's two role model-spec strings against reg, failing before
// Master spawns on any unknown alias.
// It returns the resolved values keyed by Role, with each violation wrapped naming the offending
// role.
func ResolveRoles(cfg Config, reg modelspec.Registry) (map[Role]modelspec.Resolved, error) {
	specsByRole := map[Role]string{
		RoleMaster:   cfg.Master,
		RoleRecovery: cfg.Recovery,
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
