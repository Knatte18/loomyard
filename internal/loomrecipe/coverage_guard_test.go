// coverage_guard_test.go pins the registry's reason to exist: every row the recipe's built list
// actually assembles resolves through internal/shedrecipe's registry via the row-to-engine table
// below, checked in both directions against New's real, current output rather than against a
// standalone literal that could drift silently. It builds a real shedrecipe.Env/ShedPaths pair and
// calls this package's own New, rather than iterating the table alone.
//
// This file used to also assert that shedrecipe.Names() carried no engine unreachable by this
// table beyond an allowlist -- a fourth, closed-coverage direction. That assertion was correct but
// was stated in a package that structurally cannot answer it once the registry has two consumers:
// it now lives in internal/shedrecipe's own external test package, where both consumers are
// visible, as internal/shedrecipe/coverage_guard_test.go.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// loomRowEngines maps each of New's fourteen row names to the engine name backing it. The row-name
// side is keyed off loomshed's own Name* constants, per the row-name-authority-stays-with-the-go-
// constants Shared Decision -- loomshed reads two of them for status-seed and resume purposes, so
// those constants remain the authority even though this package now builds the list. The engine
// side genuinely has to be written down by hand, because shedengine.ProducerDef carries no engine
// name at all -- only the row-name side is derivable from New's own assembled output.
var loomRowEngines = map[string]string{
	loomshed.NamePreflight:         "Preflight",
	loomshed.NameLoomPreflight:     "LoomPreflight",
	loomshed.NameDiscussionWrite:   "DiscussionWrite",
	loomshed.NameDiscussionBouncer: "Bouncer",
	loomshed.NameDiscussionBurler:  "BurlerRound",
	loomshed.NamePlanWrite:         "PlanWrite",
	loomshed.NamePlanBouncer:       "Bouncer",
	loomshed.NamePlanBurler:        "BurlerRound",
	loomshed.NameBatchifier:        "Batchifier",
	loomshed.NameWebster:           "Webster",
	loomshed.NameWebsterBouncer:    "Bouncer",
	loomshed.NameWebsterBurler:     "BurlerRound",
	loomshed.NamePublish:           "Publish",
	loomshed.NameFinalize:          "Finalize",
}

// TestCoverageGuard_EveryLoomRowHasAnEngine asserts three things about loomRowEngines against
// New's real, current row list: every row New assembles has an entry in the table (the direction
// that catches a row added to the recipe before its consuming task lands); every key in the table
// names a row New actually has (the direction that keeps the table from accumulating dead
// entries); and every engine name the table maps to resolves through shedrecipe.Lookup without
// error.
func TestCoverageGuard_EveryLoomRowHasAnEngine(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	rowNames := make(map[string]bool, len(shed.Producers))
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
	}
}

// TestCoverageGuard_EveryDirectionFailsOnItsOwnTrigger proves each of the three surviving
// directions actually fails on its own trigger, rather than assuming the deletion above left them
// intact: a row absent from loomRowEngines, a table key naming no row, and a table engine that
// does not resolve.
func TestCoverageGuard_EveryDirectionFailsOnItsOwnTrigger(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
	}

	t.Run("RowAbsentFromTable", func(t *testing.T) {
		table := make(map[string]string, len(loomRowEngines))
		for rowName, engineName := range loomRowEngines {
			if rowName == loomshed.NamePreflight {
				continue
			}
			table[rowName] = engineName
		}

		var failures []string
		for _, p := range shed.Producers {
			if _, ok := table[p.Name]; !ok {
				failures = append(failures, p.Name)
			}
		}
		if len(failures) == 0 {
			t.Error("expected a row absent from the trimmed table to be reported, found none")
		}
	})

	t.Run("TableKeyNamesNoRow", func(t *testing.T) {
		table := map[string]string{
			"Not-A-Real-Row": "Preflight",
		}

		var failures []string
		for rowName := range table {
			if !rowNames[rowName] {
				failures = append(failures, rowName)
			}
		}
		if len(failures) == 0 {
			t.Error("expected a table key naming no row to be reported, found none")
		}
	})

	t.Run("TableEngineDoesNotResolve", func(t *testing.T) {
		if _, err := shedrecipe.Lookup("Not-A-Real-Engine"); err == nil {
			t.Error("Lookup(\"Not-A-Real-Engine\") error = nil, want non-nil")
		}
	})
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
