// landingdeps_verify_test.go covers the three verify fields landingDeps tells landingshed:
// the VerifyCommand closure reads the plan at call time, and the two scratch paths match loomengine's accessors.
// It only writes files under t.TempDir(), so it stays Tier 1.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
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

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	deps := landingDeps(loc, websterengine.Geometry{StencilsDir: "/stencils"}, "task/foo",
		"https://example.com/origin.git", "main", true, func() error { return nil },
		modelspec.Registry{"claude/sonnet-5": {}}, &shuttleengine.Runner{}, landingshed.Config{})

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

	if want := loomengine.LoomVerifyPendingPath(loc); deps.VerifyPendingPath != want {
		t.Errorf("VerifyPendingPath = %q, want %q", deps.VerifyPendingPath, want)
	}
	if want := loomengine.LoomVerifyOutputPath(loc); deps.VerifyOutputPath != want {
		t.Errorf("VerifyOutputPath = %q, want %q", deps.VerifyOutputPath, want)
	}
}
