// auditengine.go declares the audit engine type and the engine lookup webster is told, and resolves the engine one recorded session is read through.

package websterengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// AuditEngine is an engine that parses a run's events and audits its fork subagents.
// Webster's recover path needs both: it reads the batch's turn ends through the Engine and its fork writes through the ForkAuditor.
type AuditEngine interface {
	shuttleengine.Engine
	shuttleengine.ForkAuditor
}

// AuditEngines is the engine lookup webster reads its sessions through.
// An empty name resolves the default engine, and DefaultName names that engine.
type AuditEngines interface {
	Lookup(name string) (shuttleengine.Engine, error)
	DefaultName() string
}

// roleAuditEngine checks that the engine role's choice resolves to holds fork audit, which every session webster spawns is read through.
// An empty engine resolves the default engine.
// The refusal names the role's key in webster.yaml and the engine, and wraps the lookup's error for an engine engines does not hold.
func roleAuditEngine(engines AuditEngines, role Role, choice shuttleengine.ModelChoice) error {
	name := choice.Engine
	if name == "" {
		name = engines.DefaultName()
	}
	const wayForward = "set %s in webster.yaml to an alias whose engine audits forks with lyx config webster, then re-run the verb, or under the Webster row re-step the row"
	engine, err := engines.Lookup(choice.Engine)
	if err != nil {
		return fmt.Errorf("webster: the %s role's engine %q is not held by this lyx: %w; way forward: "+wayForward, role, name, err, role)
	}
	if _, ok := engine.(AuditEngine); !ok {
		return fmt.Errorf("webster: the %s role's engine %q cannot audit forks, which webster needs; way forward: "+wayForward, role, name, role)
	}
	return nil
}

// sessionAuditEngine resolves the engine that session ran under from recorded, the engine name its state entry holds.
// A name engines holds resolves to that engine; one without fork audit is an internal error, since every spawn checks its engine.
// A name engines does not hold is refused wrapping the lookup's error, naming the session and the engine.
// An empty name, an entry written before engines were recorded, reads as the default engine and is refused when that default has no fork audit.
func sessionAuditEngine(engines AuditEngines, session, recorded string) (AuditEngine, error) {
	engine, err := engines.Lookup(recorded)
	if err != nil {
		if recorded == "" {
			return nil, fmt.Errorf("webster: resolve the default engine for session %s, which records none: %w", session, err)
		}
		return nil, fmt.Errorf("webster: session %s ran under engine %q, which this lyx does not hold: %w; way forward: re-run the verb, or under the Webster row re-step the row, with a lyx whose engine set holds %q", session, recorded, err, recorded)
	}
	auditEngine, ok := engine.(AuditEngine)
	if ok {
		return auditEngine, nil
	}
	if recorded == "" {
		return nil, fmt.Errorf("webster: session %s records no engine, and shuttle.yaml's engine: %q cannot audit forks, which webster needs; way forward: set engine: in shuttle.yaml back to an engine with fork audit, then re-run the verb, or under the Webster row re-step the row", session, engines.DefaultName())
	}
	return nil, fmt.Errorf("webster: internal error: session %s ran under engine %q, which cannot audit forks", session, recorded)
}
