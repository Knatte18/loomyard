//go:build tier0

// landingdeps_verify_test.go covers the verify fields landingDeps tells landingshed:
// the gate-call closures render from the hub's gate.yaml at call time, and VerifyDir is the shared verify directory.
// It only writes files under t.TempDir(), so it stays Tier 1.

package loomcli

import (
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// gateConfigWithTwoTiers returns the gate config template with tier0 and tier1 declared, and the `finalize` and `code-burler-fix` keys listing both.
func gateConfigWithTwoTiers() string {
	const tiers = `tiers:
  tier0: {command: "go test -json {targets} {args}", all_targets: ./..., timeout_min: 60, counter: go-json}
  tier1: {command: "go test -json -tags integration {targets} {args}", all_targets: ./..., timeout_min: 60, counter: go-json}`
	return strings.NewReplacer(
		"tiers: {}", tiers,
		"  finalize: {tiers: [0], scope: all}", "  finalize: {tiers: [0, 1], scope: all}",
		"  code-burler-fix: {tiers: [0], scope: diff}", "  code-burler-fix: {tiers: [0, 1], scope: diff}",
	).Replace(gateslot.ConfigTemplate())
}

func TestLandingDeps_VerifyCommandReadsPlanAtCallTime(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	// The hub's board directory exists from the start, as in a real hub, so the config dirs the sources read name it before the gate.yaml does.
	if err := os.MkdirAll(fabricengine.BoardDir(loc.HubPath), 0o755); err != nil {
		t.Fatal(err)
	}
	deps := landingDeps(loc, shedrun.RecipeLoom, websterengine.Geometry{StencilsDir: "/stencils"}, "task/foo",
		"main", true,
		modelspec.Registry{"claude/sonnet-5": {}}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(loc), parentLockDeps(loc, shedrun.SelfRunID, "", ""))

	readers := map[string]func() (string, error){
		"VerifyCommand": func() (string, error) { return deps.VerifyCommand("merge-in") },
		"BaseCommand":   deps.BaseCommand,
	}
	for name, read := range readers {
		if _, err := read(); !errors.Is(err, gateslot.ErrConfigAbsent) {
			t.Errorf("%s before any gate.yaml exists = %v; want an error wrapping gateslot.ErrConfigAbsent", name, err)
		}
	}

	writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
	for name, want := range map[string]string{
		"VerifyCommand": "lyx gate test --wait --gate merge-in",
		"BaseCommand":   "lyx gate test --wait --gate code-burler-fix",
	} {
		if got, err := readers[name](); err != nil || got != want {
			t.Errorf("%s = (%q, %v), want (%q, nil)", name, got, err, want)
		}
	}
	if got, err := deps.VerifyCommand("finalize"); err != nil || got != "lyx gate test --wait --gate finalize" {
		t.Errorf("VerifyCommand(finalize) = (%q, %v), want the finalize key's call", got, err)
	}
	if got, err := deps.GateTiers("finalize"); err != nil || !slices.Equal(got, []int{0, 1}) {
		t.Errorf("GateTiers(finalize) = (%v, %v), want ([0 1], nil)", got, err)
	}

	// An edit between two calls is seen: the finalize key names a tier the hub does not declare.
	edited := strings.NewReplacer("  finalize: {tiers: [0, 1]", "  finalize: {tiers: [0, 7]").Replace(gateConfigWithTwoTiers())
	writeHubGateConfig(t, loc.HubPath, edited)
	if got, err := deps.VerifyCommand("finalize"); err == nil || !strings.Contains(err.Error(), "lyx config gate") {
		t.Errorf("VerifyCommand over a dangling finalize key = (%q, %v), want an error naming lyx config gate", got, err)
	}

	if want := verifytree.Dir(loc.AnchorPath()); deps.VerifyDir != want {
		t.Errorf("VerifyDir = %q, want %q", deps.VerifyDir, want)
	}

	log := "--- FAIL: TestA (0.00s)\n    --- FAIL: TestA/sub (0.00s)\nFAIL\nFAIL\texample.com/m/a\t0.01s\n"
	wantTests := []verifytree.FailedTest{{Package: "example.com/m/a", Test: "TestA/sub"}}
	if got := deps.FailingTests(log); !reflect.DeepEqual(got, wantTests) {
		t.Errorf("FailingTests = %+v, want %+v", got, wantTests)
	}
}
