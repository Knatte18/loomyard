// roles.go resolves webster.yaml's two role model-spec strings to shuttle model choices, once, before any agent spawns.
// `run` calls ResolveRoles at entry, so a typo'd alias in webster.yaml fails loud before Master starts;
// it never fails mid-run, when a role first spawns.

package websterengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
	// Its spec names the webster segment.
	RoleRecovery Role = "recovery"
)

// MerriamStrandRole is the reed strand role the Master session runs under,
// so reed names the strand `<shortname>:<slug>:webster`.
// The webster.yaml model key of the Master session is `master`, a separate name.
// Its spec names the webster segment.
const MerriamStrandRole = "webster"

// ResolveRoles parses and resolves cfg's two role model-spec strings against reg, failing before
// Master spawns on any unknown alias.
// It returns each role's model choice, engine included, keyed by Role.
// Each violation is wrapped naming the offending role.
func ResolveRoles(cfg Config, reg modelspec.Registry) (map[Role]shuttleengine.ModelChoice, error) {
	specsByRole := map[Role]string{
		RoleMaster:   cfg.Master,
		RoleRecovery: cfg.Recovery,
	}

	choices := make(map[Role]shuttleengine.ModelChoice, len(specsByRole))
	for role, specStr := range specsByRole {
		spec, err := modelspec.Parse(specStr)
		if err != nil {
			return nil, fmt.Errorf("webster: role %q: %w", role, err)
		}
		r, err := reg.Resolve(spec)
		if err != nil {
			return nil, fmt.Errorf("webster: role %q: %w", role, err)
		}
		choices[role] = shuttleengine.NewModelChoice(r)
	}

	return choices, nil
}
