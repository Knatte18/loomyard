// coverage_guard_test.go pins the registry's reason to exist: every row loomshed.New actually
// assembles resolves through this package's registry, and the registry ships exactly the twelve
// entries this task built. A table-only test would pass forever no matter what loomshed grew, which
// is precisely the failure this file's own coverage guard exists to prevent -- it builds a real
// loomshed.Deps and calls loomshed.New, rather than iterating its own table alone.

package shedrecipe

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// loomRowEngines maps each of loomshed.New's thirteen row names to the engine name backing it. The
// engine side genuinely has to be written down by hand here, because shedengine.ProducerDef carries
// no engine name at all -- only the row-name side is derivable from New's own assembled output.
var loomRowEngines = map[string]string{
	"Preflight":           "Preflight",
	"Loom-Preflight":      "LoomPreflight",
	"Discussion-Write":    "Stub",
	"Discussion-Validate": "DiscussionValidate",
	"Discussion-Review":   "Stub",
	"Plan-Write":          "Stub",
	"Plan-Validate":       "PlanValidate",
	"Plan-Review":         "Stub",
	"Batchifier":          "Batchifier",
	"Webster":             "Webster",
	"Webster-Review":      "Stub",
	"Publish":             "Publish",
	"Finalize":            "Finalize",
}

// coverageGuardFakePreflight is a minimal shedengine.ShedProducer fake, filled into
// loomshed.Deps.Preflight so loomshed.New has a non-nil row 1 to build from. It is never called --
// this test only reads Shed.Producers[i].Name.
type coverageGuardFakePreflight struct{}

var _ shedengine.ShedProducer = coverageGuardFakePreflight{}

// Call is never invoked by this file's tests; see coverageGuardFakePreflight's own doc comment.
func (coverageGuardFakePreflight) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// coverageGuardFakeMergeShuttle is a minimal mergeresolve.Shuttle fake, filled into the
// landingshed.Deps built below so landingshed.NewPublish/NewFinalize's resolver construction
// succeeds. It is never called -- this test only reads Shed.Producers[i].Name.
type coverageGuardFakeMergeShuttle struct{}

var _ mergeresolve.Shuttle = coverageGuardFakeMergeShuttle{}

// Run is never invoked by this file's tests; see coverageGuardFakeMergeShuttle's own doc comment.
func (coverageGuardFakeMergeShuttle) Run(shuttleengine.Spec) (shuttleengine.Result, error) {
	return shuttleengine.Result{}, nil
}

// coverageGuardNilFabricOpener is the landingshed pair-opener closure fake this file's
// landingshed.Deps fills OpenFabric and OpenParentFabric with, mirroring
// internal/loomshed/fixture_test.go's nilFabricOpener exactly: a typed-nil *fabricengine.Fabric and
// a nil error legally satisfy NewPublish/NewFinalize, since both construct their resolver from the
// interface value the fabric handle is stored behind, and a nil check on an interface holding a
// typed-nil pointer still passes.
func coverageGuardNilFabricOpener() (*fabricengine.Fabric, error) {
	return nil, nil
}

// coverageGuardLandingDeps builds a landingshed.Deps good enough for loomshed.New to succeed,
// filled the way internal/loomshed/fixture_test.go's testLandingDeps fills it: told absolute paths
// derived from dir, a non-nil PushBranch, and OpenFabric/OpenParentFabric closures returning a
// typed-nil *fabricengine.Fabric with a nil error, plus a minimal mergeresolve.Shuttle fake.
func coverageGuardLandingDeps(dir string) landingshed.Deps {
	return landingshed.Deps{
		WorktreeRoot:     dir,
		TaskBranch:       "task-branch",
		ParentBranch:     "coverage-guard-parent",
		WebsterDir:       dir,
		StencilsDir:      dir,
		ScratchDir:       filepath.Join(dir, "landing-scratch"),
		OriginURL:        "https://example.invalid/coverage-guard/coverage-guard.git",
		PushBranch:       func() error { return nil },
		OpenFabric:       coverageGuardNilFabricOpener,
		OpenParentFabric: coverageGuardNilFabricOpener,
		Shuttle:          coverageGuardFakeMergeShuttle{},
	}
}

// coverageGuardShed builds loomshed.New's real, assembled *shedengine.Shed against a t.TempDir()-
// derived loomshed.Deps -- good enough for New to succeed, since it never runs and only its
// assembled Producers slice is read below.
func coverageGuardShed(t *testing.T) *shedengine.Shed {
	t.Helper()

	dir := t.TempDir()

	deps := loomshed.Deps{
		StatusPath:         filepath.Join(dir, "status.json"),
		LockPath:           filepath.Join(dir, "run.lock"),
		StatusLockPath:     filepath.Join(dir, "status.json.lock"),
		AnchorPath:         dir,
		WorktreeRoot:       dir,
		DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
		SupportLogPath:     filepath.Join(dir, "support-log.md"),
		Preflight:          coverageGuardFakePreflight{},
		Landing:            coverageGuardLandingDeps(dir),
	}

	shed, err := loomshed.New(deps)
	if err != nil {
		t.Fatalf("loomshed.New() error = %v, want nil", err)
	}
	return shed
}

// TestCoverageGuard_EveryLoomRowHasAnEngine asserts three things about loomRowEngines against
// loomshed.New's real, current row list: every row New assembles has an entry in the table (the
// direction that catches a row added to loomshed before piece 4 lands); every key in the table
// names a row New actually has (the direction that keeps the table from accumulating dead entries);
// and every engine name the table maps to resolves through Lookup without error.
func TestCoverageGuard_EveryLoomRowHasAnEngine(t *testing.T) {
	shed := coverageGuardShed(t)

	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
		if _, ok := loomRowEngines[p.Name]; !ok {
			t.Errorf("loomshed.New() row %q has no entry in loomRowEngines", p.Name)
		}
	}

	for rowName, engineName := range loomRowEngines {
		if !rowNames[rowName] {
			t.Errorf("loomRowEngines names row %q, which loomshed.New() does not have", rowName)
		}
		if _, err := Lookup(engineName); err != nil {
			t.Errorf("Lookup(%q) (engine for row %q) error = %v, want nil", engineName, rowName, err)
		}
	}
}

// TestRegistry_ShipsTwelveEntries asserts Names() returns exactly the sorted twelve engine names
// this task's registry ships.
func TestRegistry_ShipsTwelveEntries(t *testing.T) {
	want := []string{
		"Batchifier",
		"Bouncer",
		"BurlerRound",
		"DiscussionValidate",
		"Finalize",
		"LoomPreflight",
		"PlanValidate",
		"Preflight",
		"Publish",
		"SingleLLM",
		"Stub",
		"Webster",
	}

	got := Names()
	if len(got) != len(want) {
		t.Fatalf("Names() = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

// TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity asserts the Publish and
// Finalize rows in loomshed.New's list are named exactly "Publish" and "Finalize".
//
// Both underlying constructors discard the name argument because their identity is a package
// constant carried by their log lines, error text, and stuck-reason filename -- publishName and
// finalizeName in internal/landingshed -- so a renamed row would produce a producer whose on-disk
// identity disagrees with its row name.
func TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity(t *testing.T) {
	shed := coverageGuardShed(t)

	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
	}

	if !rowNames["Publish"] {
		t.Error(`loomshed.New() has no row named "Publish"`)
	}
	if !rowNames["Finalize"] {
		t.Error(`loomshed.New() has no row named "Finalize"`)
	}
}
