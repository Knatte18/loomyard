// module.go declares Module and PlanRules, the descriptor types that carry each caller's own
// per-CLI variance, plus the message-producing pieces that read them.

package cliwire

import "fmt"

// Module carries one standalone-capable CLI's own variance: its name and the noun phrases that
// distinguish its refusal messages from its sibling's. Each CLI declares its own Module value in
// its own package; no production file in cliwire declares one for webster or for burler.
type Module struct {
	// Name is the CLI's own name, used as the "<name>: " prefix on every error this package
	// returns. Its doc comment names no CLI and lists no example value, under the same rule the
	// four fields below follow; each caller's own wireModule declaration is where the concrete
	// name lives.
	Name string

	// StateArtifacts is the nested-geometry refusal's noun phrase for what standalone keeps
	// outside the target -- it occupies the clause naming what the state directory holds.
	StateArtifacts string

	// TargetRole is the nested-geometry refusal's noun phrase for the target -- it occupies the
	// clause naming what the target is to this CLI.
	TargetRole string

	// TargetRecourse is the reverse-nesting refusal's recourse clause opener -- it occupies the
	// sentence that tells the operator how to move the target clear of the state directory.
	TargetRecourse string

	// HubTargetSubject is the hub --target-dir refusal's subject clause -- it occupies the
	// sentence explaining what is already the target in hub mode.
	HubTargetSubject string

	// Plan carries this CLI's plan-directory rules, or nil when the CLI parses no plan (burler's
	// case). A nil Plan is what makes ResolveStandalone skip plan-dir resolution entirely.
	Plan *PlanRules
}

// PlanRules carries a CLI's plan-directory behaviour: how to compute the default plan directory for
// a told base path, and how to produce the refusal text for a plan directory that does not exist or
// holds no plan files.
type PlanRules struct {
	// DefaultPlanDir returns the default plan directory for a told base path. Webster fills it with
	// planparser.PlanDir, and ResolveStandalone is its only caller, invoking it with the stateDir
	// the prologue itself derived.
	//
	// This is a function field rather than a finished string because standalone's default depends
	// on that derived stateDir, so the caller cannot hand over a finished string. It is a function
	// field rather than an import of internal/planparser because that would put webster's plan
	// layout inside a module burler shares.
	//
	// This field is not also called from wireHub. _mill/discussion.md's exported-surface decision
	// anticipated a hub call site, but card 6 resolves hub mode's override against geom.PlanDir --
	// the value hubgeom.WebsterGeometry actually built -- rather than re-deriving the same path
	// through this field. The two are the same string today
	// (hubgeom.WebsterGeometry(loc).PlanDir is planparser.PlanDir(loc.AnchorPath())), and comparing
	// against the geometry's own field is what keeps the override check correct if internal/hubgeom
	// ever changes how it computes PlanDir. A later reader must not "restore" the hub call.
	DefaultPlanDir func(base string) string

	// MissingPlanRefusal produces the whole refusal text for a plan directory that does not exist
	// or holds no plan files.
	//
	// This is a function rather than a bare string or a format string because webster's live
	// message interpolates two distinct paths in a fixed order, and a function makes that argument
	// order a compile-time fact rather than a comment.
	//
	// The second parameter is named recourse, not defaultPlanDir, deliberately: it is the location
	// the refusal tells the operator to place the plan at, which is the mode's own default only
	// when --plan-dir actually moved the plan off it, and is the resolved plan directory itself
	// otherwise. See ResolveStandalone's own plan step for the rule that computes it.
	MissingPlanRefusal func(planDir, recourse string) string
}

// RefuseTargetDirInHubMode returns nil when flag is empty, and otherwise the hub refusal built
// from m.Name and m.HubTargetSubject.
//
// The refusal fires on the flag alone, before any config is loaded: hub mode's target is fixed by
// the worktree itself, so a told --target-dir is refused before wire reads a single file.
func (m Module) RefuseTargetDirInHubMode(flag string) error {
	if flag == "" {
		return nil
	}
	return fmt.Errorf("%s: --target-dir is not honoured in hub mode: %s, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec", m.Name, m.HubTargetSubject)
}

// refuseNestedStandaloneGeometry refuses a standalone target and derived state directory that are
// not disjoint, naming m.Name in every message so an operator reading a live error knows which CLI
// refused.
//
// Standalone geometry's whole premise is a state directory OUTSIDE the repository being driven:
// m.StateArtifacts all land under it, and none of them may appear inside the operator's own
// checkout.
// shuttleengine.NewDetachedRunner asserts the same disjointness -- but it asserts it LATE, on a
// Runner already constructed after this CLI has booted a tmux server and taken the run lock, and its
// message blames "a subpath-anchored hub geometry handed to the wrong constructor", which is not
// what happened here and offers the operator no lever at all.
//
// What actually happened is a state home nested under the target: a dotfiles repository rooted at
// the home directory, or an XDG_STATE_HOME deliberately pointed somewhere inside the checkout. That
// is an environment fact, it is fixable, and the fix is named here -- before any substrate is
// booted, which is the only point at which a refusal costs nothing to recover from.
//
// The reverse nesting (a target inside the state directory) is refused by the same guard for the
// same reason, with its own message: the lever there is m.TargetRecourse, not the state home.
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
