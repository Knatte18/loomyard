//go:build tier0

// wiring_beam_test.go covers the wiring a pawl run adds to wire: the BeamDescribe and Pawl seams, the nine-row routing, and the landing deps a pawl run's Finalize is built with.
// wire resolves no cwd and spawns no process over a hand-built hub.

package loomcli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestWire_BeamRecipe(t *testing.T) {
	t.Parallel()

	t.Run("a pawl seed fills the BeamDescribe and Pawl seams and routes its nine rows with no reroute edge", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipePawl}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		for _, nilSeam := range envkit.NilSeams(c.env) {
			if strings.HasPrefix(nilSeam, "BeamDescribe") || strings.HasPrefix(nilSeam, "Pawl") {
				t.Errorf("c.env.%s is nil after wire() of a pawl seed", nilSeam)
			}
		}
		if got, want := c.env.Pawl.Slug, "pair"; got != want {
			t.Errorf("c.env.Pawl.Slug = %q; want %q", got, want)
		}
		if want := hubgeom.PawlPaths(loc); c.env.Pawl.Paths != want {
			t.Errorf("c.env.Pawl.Paths = %+v; want %+v", c.env.Pawl.Paths, want)
		}
		if len(c.env.Pawl.Steps) != 1 {
			t.Errorf("c.env.Pawl.Steps = %d steps; want the one beam-up step", len(c.env.Pawl.Steps))
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
		if want := []string{"Beam-Up", "Beam-Await", "Parent-Align", "Code-Bouncer", "Code-Burler-Review", "Code-Burler-Fix", "Beam-Describe", "Finalize", "Beam-Down"}; strings.Join(rows, ",") != strings.Join(want, ",") {
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
		if c.env.Pawl.Down != nil || c.env.Pawl.Slug != "" {
			t.Errorf("a loom seed filled c.env.Pawl = %+v; want the zero value", c.env.Pawl)
		}
	})

	t.Run("the landing deps of a pawl run halt a spent fixer, and only a loom or darn run judges an escalate first", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		runner := &shuttleengine.Runner{}
		runner.SetPause(pauserecord.Geometry{RecordPath: filepath.Join(t.TempDir(), "pause.json"), Slug: loc.WorktreeName})
		lock := parentLockDeps(loc, shedrun.SelfRunID, "", "")

		tests := []struct {
			recipe                  string
			wantRerouteSeam         bool
			wantLandBlockers        bool
			wantEscalateJudgedFirst bool
		}{
			{shedrun.RecipeLoom, true, false, true},
			{shedrun.RecipeDarn, true, false, true},
			{shedrun.RecipePawl, false, true, false},
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
			if (deps.RerouteReviewed != nil) != tt.wantRerouteSeam {
				t.Errorf("%s: RerouteReviewed set = %v; want %v", tt.recipe, deps.RerouteReviewed != nil, tt.wantRerouteSeam)
			}
			if (deps.LandBlockers != nil) != tt.wantLandBlockers {
				t.Errorf("%s: LandBlockers set = %v; want %v", tt.recipe, deps.LandBlockers != nil, tt.wantLandBlockers)
			}
			if deps.EscalateJudgedFirst != tt.wantEscalateJudgedFirst {
				t.Errorf("%s: EscalateJudgedFirst = %v; want %v", tt.recipe, deps.EscalateJudgedFirst, tt.wantEscalateJudgedFirst)
			}
			if tt.recipe == shedrun.RecipePawl {
				if deps.TraceFailure != nil {
					t.Error("pawl: TraceFailure is set; want nil")
				}
				if deps.PlanDir != "" {
					t.Errorf("pawl: PlanDir = %q; want none", deps.PlanDir)
				}
				// A pawl run has Parent-Align, so its seam reads the perch's budget from loom.yaml, which the fixture seeds above 0.
				if off, err := deps.CodeReviewOff(); err != nil || off {
					t.Errorf("pawl: CodeReviewOff() = %v, %v; want false, nil", off, err)
				}
				if !deps.IsTestFile("pkg/widget_test.go") {
					t.Error("pawl: IsTestFile(pkg/widget_test.go) = false; want a test file of any language")
				}
			}
		}
	})
}
