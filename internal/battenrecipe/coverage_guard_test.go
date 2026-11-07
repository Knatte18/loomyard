// coverage_guard_test.go checks battenRowEngines, this package's row-to-engine table, against the rows New builds.
// It builds a real shedrecipe.Env and ShedPaths pair and calls New.
// It compares the table with New's real output in both directions.
// A standalone literal could drift silently; the check runs against New's output instead.
//
// It carries no closed-coverage assertion over shedrecipe.Names():
// internal/shedrecipe's external test package owns that check,
// and a copy here would fail on the other consumer's engines.

package battenrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// battenRowEngines maps each row name New assembles to the name of the engine that backs it.
// Its keys are this package's Name* constants.
var battenRowEngines = map[string]string{
	NameWorktreeCreate:   "WorktreeCreate",
	NameSeedChild:        "SeedChild",
	NameOpenWindow:       "Command",
	NameSeedIDE:          "Command",
	NameOpenIDE:          "Command",
	NameRunShed:          "InnerRun",
	NameLand:             "Land",
	NameWorktreeTeardown: "WorktreeTeardown",
}

// TestCoverageGuard_EveryBattenRowHasAnEngine checks battenRowEngines against the rows New builds.
// Every row New assembles has a table entry, which catches a row added to the recipe before its consuming code lands.
// Every key names a row New has, which keeps dead entries out of the table.
// Every engine name the table maps to resolves through shedrecipe.Lookup without error.
func TestCoverageGuard_EveryBattenRowHasAnEngine(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
		if _, ok := battenRowEngines[p.Name]; !ok {
			t.Errorf("New() row %q has no entry in battenRowEngines", p.Name)
		}
	}

	for rowName, engineName := range battenRowEngines {
		if !rowNames[rowName] {
			t.Errorf("battenRowEngines names row %q, which New() does not have", rowName)
		}
		if _, err := shedrecipe.Lookup(engineName); err != nil {
			t.Errorf("Lookup(%q) (engine for row %q) error = %v, want nil", engineName, rowName, err)
		}
	}
}
