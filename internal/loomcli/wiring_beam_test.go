// wiring_beam_test.go covers the wiring a pawl run adds to wire.
// It checks the Pawl seams, the routing, and the landing deps a pawl run's Finalize is built with.
// wire resolves no cwd and spawns no process over a hand-built hub.

package loomcli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/landingshed#PullRequestDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#RaddleCheck
//lym:seam internal/loomshed#RaddleFileDeps
//lym:seam internal/loomshed#RaddleWriteDeps
//lym:seam internal/loomshed#RecordWriteDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/loomshed#SamplerDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shedrecipe#RaddleWriteEnv
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_BeamRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("a pawl seed fills the Pawl seams and routes its six rows with no reroute edge", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipePawl}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		for _, nilSeam := range envkit.NilSeams(c.env) {
			if strings.HasPrefix(nilSeam, "Pawl") {
				t.Errorf("c.env.%s is nil after wire() of a pawl seed", nilSeam)
			}
		}
		if got, want := c.env.Pawl.Slug, "pair"; got != want {
			t.Errorf("c.env.Pawl.Slug = %q; want %q", got, want)
		}

		wantRecord := loomengine.ProjectDecisionRecord(loc, "pair")
		if c.env.DecisionRecordPath != wantRecord || c.env.SupportLogPath != loomengine.ProjectSupportLog(loc, "pair") {
			t.Errorf("c.env record = %q, support log = %q; want the project record %q and its support log", c.env.DecisionRecordPath, c.env.SupportLogPath, wantRecord)
		}
		if c.env.PatternRecordPath != "" {
			t.Errorf("c.env.PatternRecordPath = %q; want none for a beam's project record", c.env.PatternRecordPath)
		}
		if got := c.env.RecordWrite; got.RecordPath != wantRecord || got.SupportLogPath != loomengine.ProjectSupportLog(loc, "pair") || got.DiscussionDir != loomengine.ProjectDir(loc, "pair") {
			t.Errorf("c.env.RecordWrite = %q, %q, %q; want the project record, its support log and the project directory", got.RecordPath, got.SupportLogPath, got.DiscussionDir)
		}
		arbiter, err := c.arbiterDepsFor()
		if err != nil {
			t.Fatalf("arbiterDepsFor() = %v; want nil", err)
		}
		if arbiter.Prompt.DecisionRecordPath != wantRecord || arbiter.Prompt.SupportLogPath != loomengine.ProjectSupportLog(loc, "pair") {
			t.Errorf("arbiter prompt record = %q, support log = %q; want the project record %q and its support log", arbiter.Prompt.DecisionRecordPath, arbiter.Prompt.SupportLogPath, wantRecord)
		}
		if want := hubgeom.PawlPaths(loc); c.env.Pawl.Paths != want {
			t.Errorf("c.env.Pawl.Paths = %+v; want %+v", c.env.Pawl.Paths, want)
		}
		if len(c.env.Pawl.Steps) != 1 {
			t.Errorf("c.env.Pawl.Steps = %d steps; want the one beam-up step", len(c.env.Pawl.Steps))
		}
		wantRetired, err := loomrecipe.RetiredRowNames(shedrun.RecipePawl)
		if err != nil {
			t.Fatalf("RetiredRowNames(pawl) = %v; want nil", err)
		}
		if len(wantRetired) == 0 || !slices.Equal(c.env.Pawl.RetiredRows, wantRetired) {
			t.Errorf("c.env.Pawl.RetiredRows = %v; want the recipe's retired rows %v", c.env.Pawl.RetiredRows, wantRetired)
		}

		if c.shedPaths.RetiredMove == nil || c.shedPaths.RetiredWayForward != "lym loom resume" {
			t.Errorf("c.shedPaths RetiredMove set = %v, RetiredWayForward = %q; want the move set and `lym loom resume` as the way forward", c.shedPaths.RetiredMove != nil, c.shedPaths.RetiredWayForward)
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
		if want := []string{"Preflight", "Record-Write", "Beam-Up", "Beam-Await", "Finalize", "Beam-Down"}; strings.Join(rows, ",") != strings.Join(want, ",") {
			t.Errorf("recipeRouting() rows = %v; want %v", rows, want)
		}
	})

	t.Run("a loom seed leaves the Pawl seams unfilled", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeTapestry}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.env.Pawl.Down != nil || c.env.Pawl.Slug != "" {
			t.Errorf("a loom seed filled c.env.Pawl = %+v; want the zero value", c.env.Pawl)
		}
		if c.shedPaths.RetiredMove != nil || c.shedPaths.RetiredWayForward != "lym loom resume" {
			t.Errorf("c.shedPaths RetiredMove set = %v, RetiredWayForward = %q; want no move and `lym loom resume` as the way forward", c.shedPaths.RetiredMove != nil, c.shedPaths.RetiredWayForward)
		}
	})

	t.Run("the landing deps of a pawl run halt a spent fixer, and every loom run judges an escalate first", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		runner := &shuttleengine.Runner{}
		runner.SetPause(pauserecord.Geometry{RecordPath: filepath.Join(t.TempDir(), "pause.json"), Slug: loc.WorktreeName})
		lock := parentLockDeps(loc, shedrun.SelfRunID, "", "")

		tests := []struct {
			recipe                  string
			wantLandBlockers        bool
			wantEscalateJudgedFirst bool
		}{
			{shedrun.RecipeTapestry, false, true},
			{shedrun.RecipeDarn, false, true},
			{shedrun.RecipePawl, true, true},
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
			if (deps.LandBlockers != nil) != tt.wantLandBlockers {
				t.Errorf("%s: LandBlockers set = %v; want %v", tt.recipe, deps.LandBlockers != nil, tt.wantLandBlockers)
			}
			if deps.EscalateJudgedFirst != tt.wantEscalateJudgedFirst {
				t.Errorf("%s: EscalateJudgedFirst = %v; want %v", tt.recipe, deps.EscalateJudgedFirst, tt.wantEscalateJudgedFirst)
			}
			wantRecord := loomengine.DiscussionDecisionRecord(loc)
			if tt.recipe == shedrun.RecipePawl {
				wantRecord = loomengine.ProjectDecisionRecord(loc, "pair")
			}
			if deps.DecisionRecordPath != wantRecord {
				t.Errorf("%s: DecisionRecordPath = %q; want %q", tt.recipe, deps.DecisionRecordPath, wantRecord)
			}
			if tt.recipe == shedrun.RecipePawl {
				if deps.TraceFailure != nil {
					t.Error("pawl: TraceFailure is set; want nil")
				}
				if deps.PlanDir != "" {
					t.Errorf("pawl: PlanDir = %q; want none", deps.PlanDir)
				}
				// A pawl run has Parent-Align, so its seam reads the perch's budget from loom.yaml.
				// The fixture seeds that budget above 0.
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
