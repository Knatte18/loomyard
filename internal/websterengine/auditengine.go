// auditengine.go declares the engine type webster's recover path needs and the assertion that wires it from an engine set.

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

// AuditEngineFrom resolves the default engine of engines and asserts that it audits forks.
// It returns the lookup's error when the default does not resolve,
// and a refusal naming the engine and the way forward when the default has no fork audit.
func AuditEngineFrom(engines shuttleengine.EngineSet) (AuditEngine, error) {
	engine, err := engines.Lookup("")
	if err != nil {
		return nil, err
	}
	auditEngine, ok := engine.(AuditEngine)
	if !ok {
		return nil, fmt.Errorf("webster: shuttle.yaml's engine: %q cannot audit forks, which webster needs; way forward: set engine: in shuttle.yaml to an engine with fork support, then re-run the verb, or under the Webster row re-step the row", engines.DefaultName())
	}
	return auditEngine, nil
}
