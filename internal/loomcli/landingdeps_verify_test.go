// landingdeps_verify_test.go covers the three verify fields landingDeps tells landingshed:
// the VerifyCommand closure reads the plan at call time, and VerifyDir is the shared verify directory.
// It only writes files under t.TempDir(), so it stays Tier 1.

package loomcli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

const verifySectionBody = "```\ngo build ./...\ngo test ./...\n```"

func writeVerifyPlan(t *testing.T, planDir string, sections ...plankit.Section) {
	t.Helper()
	plankit.Write(t, planDir, plankit.Plan{
		Approved: true,
		Framing:  "Framing paragraph.",
		Sections: sections,
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "only",
			Summary: "the only card",
			Groups:  []plankit.Group{{Label: "Edit", Targets: []string{"internal/x#Y"}}},
			Intent:  "placeholder card.",
		}},
	})
}

func TestLandingDeps_VerifyCommandReadsPlanAtCallTime(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	deps := landingDeps(loc, websterengine.Geometry{StencilsDir: "/stencils"}, "task/foo",
		"https://example.com/origin.git", "main", true, func() error { return nil },
		modelspec.Registry{"claude/sonnet-5": {}}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, planVerifySource(loc))

	if want := planVerifySource(loc).failedWayForward; deps.VerifyFailedWayForward != want || !strings.Contains(want, `"lyx loom goto --to Webster-Burler"`) {
		t.Errorf("VerifyFailedWayForward = %q, want the plan source's Webster-Burler clause %q", deps.VerifyFailedWayForward, want)
	}

	if _, err := deps.VerifyCommand(); err == nil {
		t.Fatal("VerifyCommand before any plan exists: want an error, got nil")
	}

	planDir := planparser.PlanDir(loc.AnchorPath())
	writeVerifyPlan(t, planDir, plankit.Section{Heading: "verify:", Body: verifySectionBody})
	got, err := deps.VerifyCommand()
	if err != nil {
		t.Fatalf("VerifyCommand after plan written: %v", err)
	}
	if want := "go build ./... && go test ./..."; got != want {
		t.Errorf("VerifyCommand = %q, want %q", got, want)
	}

	writeVerifyPlan(t, planDir)
	got, err = deps.VerifyCommand()
	if err != nil || got != "" {
		t.Errorf("VerifyCommand with no verify section = (%q, %v), want (\"\", nil)", got, err)
	}

	if want := verifytree.Dir(loc.AnchorPath()); deps.VerifyDir != want {
		t.Errorf("VerifyDir = %q, want %q", deps.VerifyDir, want)
	}

	if want := gateslot.Dir(fabricengine.BoardDir(loc.HubPath)); deps.GateSlots == nil || deps.GateSlots.Dir != want {
		t.Errorf("GateSlots = %+v, want a pool over %q", deps.GateSlots, want)
	}

	log := "--- FAIL: TestA (0.00s)\n    --- FAIL: TestA/sub (0.00s)\nFAIL\nFAIL\texample.com/m/a\t0.01s\n"
	wantTests := []verifytree.FailedTest{{Package: "example.com/m/a", Test: "TestA/sub"}}
	if got := deps.FailingTests(log); !reflect.DeepEqual(got, wantTests) {
		t.Errorf("FailingTests = %+v, want %+v", got, wantTests)
	}
}
