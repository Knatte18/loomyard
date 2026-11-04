// wiring_beam_test.go covers the wiring a pawl run adds to wire: the BeamDescribe and Pawl seams, the nine-row routing, the landing deps a pawl run's Finalize is built with, and the reopen request PR-Review reads.
// wire resolves no cwd and spawns no process over a hand-built hub.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestReadReopenRequest asserts PR-Review reads only a reopen request that names the pair's own run:
// no request file, a request of another run and a request with no seed to name the run read as none, and a matching request reads with its close flag.
func TestReadReopenRequest(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const runStart = "2026-03-01T09:30:00Z"
	tests := []struct {
		name string
		// seeded says the pair's pawl run has a seed started at runStart.
		seeded bool
		// request is the reopen request written first; nil writes none.
		request   *pawlengine.ReopenRequest
		want      landingshed.ReopenRequest
		wantFound bool
	}{
		{name: "NoRequestFileReadsAsNone", seeded: true},
		{name: "RequestOfAnotherRunReadsAsNone", seeded: true, request: &pawlengine.ReopenRequest{RunID: "2026-02-28T08:00:00Z"}},
		{name: "RequestWithNoSeedReadsAsNone", request: &pawlengine.ReopenRequest{RunID: runStart}},
		{name: "RequestOfThePairsRunReadsAsKeepingThePullRequest", seeded: true, request: &pawlengine.ReopenRequest{RunID: runStart}, wantFound: true},
		{name: "RequestOfThePairsRunReadsWithItsCloseFlag", seeded: true, request: &pawlengine.ReopenRequest{RunID: runStart, Close: true}, want: landingshed.ReopenRequest{Close: true}, wantFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			if tt.seeded {
				if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipePawl, Driver: shedrun.DriverGo, StartedAt: runStart}); err != nil {
					t.Fatalf("WriteSeed() = %v; want nil", err)
				}
			}
			if tt.request != nil {
				if err := pawlengine.WriteReopenRequest(hubgeom.PawlPaths(loc), *tt.request); err != nil {
					t.Fatalf("WriteReopenRequest() = %v; want nil", err)
				}
			}

			got, found, err := readReopenRequest(loc)

			if err != nil || found != tt.wantFound || got != tt.want {
				t.Errorf("readReopenRequest() = %+v, %v, %v; want %+v, %v, nil", got, found, err, tt.want, tt.wantFound)
			}
		})
	}
}

//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#BeamDescribeDeps
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/landingshed#PullRequestDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#RecordWriteDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/loomshed#SamplerDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_BeamRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("a pawl seed fills the BeamDescribe and Pawl seams and routes its rows with PR-Review's one reroute edge back to Beam-Await", func(t *testing.T) {
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
		// A project record carries an added decision that the discussion directory lacks, so the BeamDescribe seam can only read it from the project path.
		if err := os.MkdirAll(filepath.Dir(wantRecord), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(wantRecord), err)
		}
		added := "### Added after Discussion (orch, 2026-01-02): Keep the cache\n\nDecision: Keep it per run.\n"
		if err := os.WriteFile(wantRecord, []byte("## Goal\n\n## Decisions\n\n"+added), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", wantRecord, err)
		}
		if got, err := c.env.BeamDescribe.ProjectDecisions(); err != nil || len(got) != 1 || !strings.Contains(got[0], "Keep the cache") {
			t.Errorf("c.env.BeamDescribe.ProjectDecisions() = %q, %v; want the project record's one added decision", got, err)
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
		if got, want := c.env.BeamDescribe.Slug, "pair"; got != want {
			t.Errorf("c.env.BeamDescribe.Slug = %q; want %q", got, want)
		}
		if got, want := filepath.Base(c.env.BeamDescribe.DescriptionPath), filepath.Base(c.env.DescriptionPath); got != want {
			t.Errorf("c.env.BeamDescribe.DescriptionPath base = %q; want the landing description's %q", got, want)
		}
		if _, err := c.env.BeamDescribe.Commits(); err == nil || !strings.Contains(err.Error(), "way forward") {
			t.Errorf("c.env.BeamDescribe.Commits() of a pair with no origin record = %v; want a refusal naming the way forward", err)
		}

		routing, err := c.recipeRouting()
		if err != nil {
			t.Fatalf("recipeRouting() = %v; want nil", err)
		}
		var rows []string
		for _, producer := range routing.Producers {
			rows = append(rows, producer.Name)
			want := ""
			if producer.Name == "PR-Review" {
				want = "Beam-Await"
			}
			if producer.OnReroute != want {
				t.Errorf("row %q OnReroute = %q; want %q", producer.Name, producer.OnReroute, want)
			}
		}
		if want := []string{"Preflight", "Record-Write", "Beam-Up", "Beam-Await", "Parent-Align", "Beam-Describe", "PR-Review", "Finalize", "Beam-Down"}; strings.Join(rows, ",") != strings.Join(want, ",") {
			t.Errorf("recipeRouting() rows = %v; want %v", rows, want)
		}
	})

	t.Run("a loom seed leaves the BeamDescribe seams unfilled", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeTapestry}
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
