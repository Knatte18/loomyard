// coverage_guard_test.go pins the registry's reason to exist for this package's own three rows: it
// builds a real shedrecipe.Env/ShedPaths pair, calls this package's own New, and checks the
// row-to-engine table below in both directions against New's real, current output rather than
// against a standalone literal that could drift silently.
//
// It carries no closed-coverage assertion over shedrecipe.Names() -- that claim now lives in one
// cross-consumer place, internal/shedrecipe's own external test package, and a second copy here
// would fail on the other consumer's engines.

package battenrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// battenRowEngines maps each of New's three row names to the engine name backing it. The
// row-name side is keyed off this package's own Name* constants, per the
// row-name-authority-stays-with-the-go-constants Shared Decision.
var battenRowEngines = map[string]string{
	NameWorktreeCreate:   "WorktreeCreate",
	NameRunShed:          "InnerRun",
	NameWorktreeTeardown: "WorktreeTeardown",
}

// TestCoverageGuard_EveryBattenRowHasAnEngine asserts three things about battenRowEngines
// against New's real, current row list: every row New assembles has an entry in the table (the
// direction that catches a row added to the recipe before its consuming code lands); every key in
// the table names a row New actually has (the direction that keeps the table from accumulating
// dead entries); and every engine name the table maps to resolves through shedrecipe.Lookup
// without error.
func TestCoverageGuard_EveryBattenRowHasAnEngine(t *testing.T) {
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
