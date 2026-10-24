//go:build tier0

// wiring_beam_test.go covers the wiring a beam run adds to wire: the BeamDescribe seams, the two-row routing, and the landing deps a beam run's Finalize is built with.
// wire resolves no cwd and spawns no process over a hand-built hub.

package loomcli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestWire_BeamRecipe(t *testing.T) {
	t.Parallel()

	t.Run("a beam seed fills the BeamDescribe seams and routes two rows with no reroute edge", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeBeam}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		for _, nilSeam := range envkit.NilSeams(c.env) {
			if strings.HasPrefix(nilSeam, "BeamDescribe") {
				t.Errorf("c.env.%s is nil after wire() of a beam seed", nilSeam)
			}
		}
		if got, want := c.env.BeamDescribe.Slug, "pair"; got != want {
			t.Errorf("c.env.BeamDescribe.Slug = %q; want %q", got, want)
		}
		if got, want := filepath.Base(c.env.BeamDescribe.DescriptionPath), filepath.Base(c.env.DescriptionPath); got != want {
			t.Errorf("c.env.BeamDescribe.DescriptionPath base = %q; want the landing description's %q", got, want)
		}
		if _, err := c.env.BeamDescribe.CommitSubjects(); err == nil || !strings.Contains(err.Error(), "way forward") {
			t.Errorf("c.env.BeamDescribe.CommitSubjects() of a pair with no origin record = %v; want a refusal naming the way forward", err)
		}

		routing, err := c.recipeRouting()
		if err != nil {
			t.Fatalf("recipeRouting() = %v; want nil", err)
		}
		var rows []string
		for _, producer := range routing.Producers {
			rows = append(rows, producer.Name)
			if producer.OnReroute != "" {
				t.Errorf("row %q OnReroute = %q; want none", producer.Name, producer.OnReroute)
			}
		}
		if want := []string{"Beam-Describe", "Finalize"}; strings.Join(rows, ",") != strings.Join(want, ",") {
			t.Errorf("recipeRouting() rows = %v; want %v", rows, want)
		}
	})

	t.Run("a loom seed leaves the BeamDescribe seams unfilled", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeLoom}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.env.BeamDescribe.Entry != nil || c.env.BeamDescribe.Slug != "" {
			t.Errorf("a loom seed filled c.env.BeamDescribe = %+v; want the zero value", c.env.BeamDescribe)
		}
	})

	t.Run("the landing deps of a beam run let a large merge land and halt a spent fixer", func(t *testing.T) {
		t.Parallel()
		loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
		runner := &shuttleengine.Runner{}
		runner.SetPause(pauserecord.Geometry{RecordPath: filepath.Join(t.TempDir(), "pause.json"), Slug: loc.WorktreeName})
		lock := parentLockDeps(loc, shedrun.SelfRunID, "", "")

		tests := []struct {
			recipe               string
			wantLargeMerge       bool
			wantConflictReroutes bool
			wantRerouteSeam      bool
		}{
			{shedrun.RecipeLoom, false, true, true},
			{shedrun.RecipeDarn, false, false, true},
			{shedrun.RecipeBeam, true, false, false},
		}
		for _, tt := range tests {
			deps := landingDeps(
				loc,
				tt.recipe,
				websterengine.Geometry{WebsterDir: "/webster", StencilsDir: "/stencils"},
				"task/foo",
				"main",
				true,
				modelspec.Registry{},
				runner,
				landingshed.Config{Squash: true},
				"parent-session",
				nil,
				func(string, time.Time) error { return nil },
				func() (string, error) { return "", nil },
				planVerifySource(loc),
				lock,
			)
			if deps.LargeMergeLands != tt.wantLargeMerge {
				t.Errorf("%s: LargeMergeLands = %v; want %v", tt.recipe, deps.LargeMergeLands, tt.wantLargeMerge)
			}
			if deps.ConflictReroutes != tt.wantConflictReroutes {
				t.Errorf("%s: ConflictReroutes = %v; want %v", tt.recipe, deps.ConflictReroutes, tt.wantConflictReroutes)
			}
			if (deps.RerouteReviewed != nil) != tt.wantRerouteSeam {
				t.Errorf("%s: RerouteReviewed set = %v; want %v", tt.recipe, deps.RerouteReviewed != nil, tt.wantRerouteSeam)
			}
			if tt.recipe == shedrun.RecipeBeam {
				if deps.TraceFailure != nil {
					t.Error("beam: TraceFailure is set; want nil")
				}
				if deps.PlanDir != "" {
					t.Errorf("beam: PlanDir = %q; want none", deps.PlanDir)
				}
				if off, err := deps.CodeReviewOff(); err != nil || off {
					t.Errorf("beam: CodeReviewOff() = %v, %v; want false, nil", off, err)
				}
				if !deps.IsTestFile("pkg/widget_test.go") {
					t.Error("beam: IsTestFile(pkg/widget_test.go) = false; want a test file of any language")
				}
			}
		}
	})
}
