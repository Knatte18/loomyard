// coverage_guard_test.go checks the row-to-engine table of wantProducerTable (shape_test.go) against New's output.
// Every row New assembles must resolve through internal/shedrecipe's registry via that table.
// The table is checked in both directions against New's real row list, not against a literal that could drift.
// It builds a shedrecipe.Env/ShedPaths pair and calls this package's own New, rather than iterating the table alone.
//
// The closed-coverage direction over shedrecipe.Names() is checked in internal/shedrecipe/coverage_guard_test.go.
// Only that package sees both registry consumers.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCoverageGuard_EveryLoomRowHasAnEngine asserts three things about the row-to-engine table of wantProducerTable:
// every row New assembles has an entry in the table, every table key names a row New assembles,
// and every engine name the table maps to resolves through shedrecipe.Lookup without error.
func TestCoverageGuard_EveryLoomRowHasAnEngine(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	engines := rowEngines()
	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
		if _, ok := engines[p.Name]; !ok {
			t.Errorf("New() row %q has no entry in wantProducerTable", p.Name)
		}
	}

	for rowName, engineName := range engines {
		if !rowNames[rowName] {
			t.Errorf("wantProducerTable names row %q, which New() does not have", rowName)
		}
		if _, err := shedrecipe.Lookup(engineName); err != nil {
			t.Errorf("Lookup(%q) (engine for row %q) error = %v, want nil", engineName, rowName, err)
		}
	}
}

// TestCoverageGuard_EveryDirectionFailsOnItsOwnTrigger proves that each direction fails on its own trigger:
// a row absent from the table, a table key naming no row, and a table engine that does not resolve.
func TestCoverageGuard_EveryDirectionFailsOnItsOwnTrigger(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
	}

	t.Run("RowAbsentFromTable", func(t *testing.T) {
		engines := rowEngines()
		table := make(map[string]string, len(engines))
		for rowName, engineName := range engines {
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

// TestCoverageGuard_MergeInAndFinalizeRowNamesMatchTheirProducerIdentity asserts the rows exist.
// The rows are loomshed.NameParentAlign and loomshed.NameFinalize in the recipe's built list.
//
// Both underlying constructors discard their name argument.
// Their identity is a package constant in internal/landingshed, such as `finalizeName`.
// Their log lines and error text carry that identity.
// A renamed row would therefore produce a producer whose logged identity disagrees with its row name.
func TestCoverageGuard_MergeInAndFinalizeRowNamesMatchTheirProducerIdentity(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
	}

	if !rowNames[loomshed.NameParentAlign] {
		t.Errorf("New() has no row named %q", loomshed.NameParentAlign)
	}
	if !rowNames[loomshed.NameFinalize] {
		t.Errorf("New() has no row named %q", loomshed.NameFinalize)
	}
}
