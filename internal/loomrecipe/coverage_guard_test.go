// coverage_guard_test.go pins the registry's reason to exist: every row the recipe's built list
// actually assembles resolves through internal/shedrecipe's registry via the row-to-engine table
// below, checked in both directions against New's real, current output rather than against a
// standalone literal that could drift silently. It builds a real shedrecipe.Env/ShedPaths pair and
// calls this package's own New, rather than iterating the table alone.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// loomRowEngines maps each of New's seventeen row names to the engine name backing it. The row-name
// side is keyed off loomshed's own Name* constants, per the row-name-authority-stays-with-the-go-
// constants Shared Decision -- loomshed reads two of them for status-seed and resume purposes, so
// those constants remain the authority even though this package now builds the list. The engine
// side genuinely has to be written down by hand, because shedengine.ProducerDef carries no engine
// name at all -- only the row-name side is derivable from New's own assembled output.
var loomRowEngines = map[string]string{
	loomshed.NamePreflight:          "Preflight",
	loomshed.NameLoomPreflight:      "LoomPreflight",
	loomshed.NameDiscussionWrite:    "DiscussionWrite",
	loomshed.NameDiscussionValidate: "DiscussionValidate",
	loomshed.NameDiscussionBouncer:  "Bouncer",
	loomshed.NameDiscussionBurler:   "BurlerRound",
	loomshed.NamePlanWrite:          "PlanWrite",
	loomshed.NamePlanValidate:       "PlanValidate",
	loomshed.NamePlanBouncer:        "Bouncer",
	loomshed.NamePlanBurler:         "BurlerRound",
	loomshed.NamePlanRevalidate:     "PlanValidate",
	loomshed.NameBatchifier:         "Batchifier",
	loomshed.NameWebster:            "Webster",
	loomshed.NameWebsterBouncer:     "Bouncer",
	loomshed.NameWebsterBurler:      "BurlerRound",
	loomshed.NamePublish:            "Publish",
	loomshed.NameFinalize:           "Finalize",
}

// coverageGuardAllowedUnreachableEngines names the registry engines this task's coverage guard
// tolerates as unreferenced by any of the seventeen built rows. Stub joins this allowlist now that
// the last stubbed row -- Webster-Review -- is real: no loom row reaches Stub any more, and the
// engine stays registered because internal/shedrecipe's registry is generic Shed machinery shared
// by reference with a future product's producer list rather than loom's private property.
// SingleLLM is the other tolerated entry: the two other "loom: real LLM producers" roadmap items
// (manifest/roadmap.md) have not yet landed a row that reaches it.
var coverageGuardAllowedUnreachableEngines = map[string]bool{
	"SingleLLM": true,
	"Stub":      true,
}

// TestCoverageGuard_EveryLoomRowHasAnEngine asserts four things about loomRowEngines against New's
// real, current row list: every row New assembles has an entry in the table (the direction that
// catches a row added to the recipe before its consuming task lands); every key in the table names a
// row New actually has (the direction that keeps the table from accumulating dead entries); every
// engine name the table maps to resolves through shedrecipe.Lookup without error; and, as a fourth
// half, that shedrecipe.Names() carries no entry left unreachable by the table beyond
// coverageGuardAllowedUnreachableEngines. This last half is a newly added assertion, not a
// weakening of an earlier one -- the guard previously made no claim at all about unused registry
// entries.
func TestCoverageGuard_EveryLoomRowHasAnEngine(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	rowNames := make(map[string]bool, len(shed.Producers))
	usedEngines := make(map[string]bool, len(loomRowEngines))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
		if _, ok := loomRowEngines[p.Name]; !ok {
			t.Errorf("New() row %q has no entry in loomRowEngines", p.Name)
		}
	}

	for rowName, engineName := range loomRowEngines {
		if !rowNames[rowName] {
			t.Errorf("loomRowEngines names row %q, which New() does not have", rowName)
		}
		if _, err := shedrecipe.Lookup(engineName); err != nil {
			t.Errorf("Lookup(%q) (engine for row %q) error = %v, want nil", engineName, rowName, err)
		}
		usedEngines[engineName] = true
	}

	for _, name := range shedrecipe.Names() {
		if usedEngines[name] || coverageGuardAllowedUnreachableEngines[name] {
			continue
		}
		t.Errorf("shedrecipe.Names() has %q, which no row reaches and which is not in coverageGuardAllowedUnreachableEngines", name)
	}
}

// TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity asserts the rows named
// loomshed.NamePublish and loomshed.NameFinalize exist in the recipe's built list.
//
// Both underlying constructors discard the name argument because their identity is a package
// constant carried by their log lines, error text, and stuck-reason filename -- publishName and
// finalizeName in internal/landingshed -- so a renamed row would produce a producer whose on-disk
// identity disagrees with its row name.
func TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
	}

	if !rowNames[loomshed.NamePublish] {
		t.Errorf("New() has no row named %q", loomshed.NamePublish)
	}
	if !rowNames[loomshed.NameFinalize] {
		t.Errorf("New() has no row named %q", loomshed.NameFinalize)
	}
}
