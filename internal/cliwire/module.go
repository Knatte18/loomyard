// module.go declares Module and PlanRules, the descriptor types that carry each CLI's own variance,
// and the message builders that read them.

package cliwire

import (
	"fmt"
	"os"
)

// Module carries the variance of one standalone-capable CLI.
// Its name and the noun phrases distinguish its refusal messages from those of the other CLIs.
// Each CLI declares its own Module value in its own package, so this package declares none.
type Module struct {
	// Name is the CLI's own name, used as the "<name>: " prefix on every error this package returns.
	Name string

	// StateArtifacts is the nested-geometry refusal's noun phrase for what standalone keeps outside the target.
	// It fills the clause that names what the state directory holds.
	StateArtifacts string

	// TargetRole is the nested-geometry refusal's noun phrase for the target.
	// It fills the clause that names what the target is to this CLI.
	TargetRole string

	// TargetRecourse opens the recourse clause of the reverse-nesting refusal.
	// It fills the sentence that tells the operator how to move the target clear of the state directory.
	TargetRecourse string

	// HubTargetSubject is the subject clause of the hub --target-dir refusal.
	// It fills the sentence that explains what is already the target in hub mode.
	HubTargetSubject string

	// Plan carries this CLI's plan-directory rules, or nil when the CLI parses no plan.
	// A nil Plan makes ResolveStandalone skip plan-dir resolution entirely.
	Plan *PlanRules
}

// PlanRules carries a CLI's plan-directory behaviour.
// It gives the default plan directory for a told base path,
// and the refusal text for a plan directory that does not exist or holds no plan files.
type PlanRules struct {
	// DefaultPlanDir returns the default plan directory for a told base path.
	// It is a function because the default depends on the state directory ResolveStandalone derives,
	// so a caller cannot hand over a finished string.
	// The field is not an import of internal/planparser:
	// that would put one CLI's plan layout inside a module that other CLIs share.
	//
	// The hub wiring does not call this field.
	// Hub mode checks its override against the geometry's own PlanDir, the value internal/hubgeom builds.
	// The two are the same string today.
	// Comparing against the geometry's field keeps the check correct if internal/hubgeom changes its computation.
	// Do not route that check through this field.
	DefaultPlanDir func(base string) string

	// MissingPlanRefusal produces the whole refusal text for a plan directory that is missing or holds no plan files.
	//
	// It is a function rather than a string because the refusal interpolates two distinct paths in a fixed order;
	// a function makes that order a compile-time fact rather than a comment.
	//
	// The second parameter, recourse, is the location the refusal tells the operator to place the plan at.
	// That is the mode's default only when --plan-dir moved the plan off it;
	// otherwise it is the resolved plan directory itself.
	//
	// ResolveStandalone's plan step computes it.
	MissingPlanRefusal func(planDir, recourse string) string
}

// RefuseTargetDirInHubMode returns nil when flag is empty.
// Otherwise it returns the hub refusal built from m.Name and m.HubTargetSubject.
//
// The refusal fires on the flag alone, before any config is loaded.
// Hub mode's target is fixed by the worktree itself, so the flag cannot choose one.
func (m Module) RefuseTargetDirInHubMode(flag string) error {
	if flag == "" {
		return nil
	}
	return fmt.Errorf("%s: --target-dir is not honoured in hub mode: %s, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec", m.Name, m.HubTargetSubject)
}

// RefuseUnreadableStencilsDir returns nil for an empty stencilsDir, which means no override was told.
// Each mode derives or seeds its own default in that case.
// Otherwise it requires the already-resolved override to exist as a directory.
// It refuses with a message built from m.Name.
//
// Without this check, a mistyped stencils directory surfaces only at the first stencil read:
// after the run lock is taken and a substrate is booted.
// A refusal is free only before a substrate is booted, so the check runs at the wiring boundary.
//
// Every mode's wiring and ResolveStandalone call this method,
// so the modes cannot disagree on what a told stencils directory must be.
// The derived standalone default is exempt by the empty-string contract: it is seeded where it is derived.
func (m Module) RefuseUnreadableStencilsDir(stencilsDir string) error {
	if stencilsDir == "" {
		return nil
	}
	info, err := os.Stat(stencilsDir)
	if err != nil {
		return fmt.Errorf("%s: --stencils-dir %s cannot be read: %w -- a curated stencil set that is not there is not an empty one: every prompt render reads from it, and unchecked the failure surfaces only after the run lock is taken and substrate is booted", m.Name, stencilsDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: --stencils-dir %s is not a directory -- the stencils directory is a curated stencil set read at call time", m.Name, stencilsDir)
	}
	return nil
}

// refuseNestedStandaloneGeometry refuses a standalone target and derived state directory that are
// not disjoint, naming m.Name in every message so an operator reading a live error knows which CLI refused.
//
// Standalone mode keeps its m.StateArtifacts in a state directory outside the repository being driven.
// A state home nested under the target is an environment fact the operator can fix:
// a dotfiles repository rooted at the home directory, or an XDG_STATE_HOME set inside the checkout.
// The refusal fires before any substrate is booted, the only point where it costs nothing to recover from.
//
// The reverse nesting, a target inside the state directory, is refused by the same guard,
// and its lever is m.TargetRecourse rather than the state home.
func (m Module) refuseNestedStandaloneGeometry(target, stateDir string) error {
	target, stateDir = NormalizeForContainment(target), NormalizeForContainment(stateDir)
	if pathContains(target, stateDir) {
		return fmt.Errorf("%s: the derived state directory %s lies inside the standalone target %s: standalone mode keeps its %s strictly outside %s, so the two must be disjoint. The state home is nested under the target -- a repository rooted at your home directory is the usual cause. Point XDG_STATE_HOME (LOCALAPPDATA on Windows) at a directory outside %s and re-run", m.Name, stateDir, target, m.StateArtifacts, m.TargetRole, target)
	}
	if pathContains(stateDir, target) {
		return fmt.Errorf("%s: the standalone target %s lies inside the derived state directory %s: standalone mode keeps its %s strictly outside %s, so the two must be disjoint. %s outside the state home, or point XDG_STATE_HOME (LOCALAPPDATA on Windows) elsewhere, and re-run", m.Name, target, stateDir, m.StateArtifacts, m.TargetRole, m.TargetRecourse)
	}
	return nil
}
