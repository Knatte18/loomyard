// wiring_test.go drives wire directly against a hand-built *lyxcwd.Location over a temporary
// directory seeded with the one module config load that does not tolerate absence (loomengine's own
// strict configengine.Load): every other config load this task's wire performs
// (reedengine.LoadConfig, shuttleengine.LoadConfig, websterengine.LoadConfig, modelspec.LoadRegistry,
// batcher.Active) degrades to its embedded template on the same proven-present _lyx/ directory, so
// seeding loom.yaml alone is enough to drive the whole hub-only wire() without touching a real hub.

package loomcli

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// seedLoomConfig creates <anchorPath>/_lyx/config/loom.yaml with the embedded template's contents --
// the one config load loomengine.LoadConfig performs strictly (see loomengine's own config_test.go).
func seedLoomConfig(t *testing.T, anchorPath string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	if err := os.WriteFile(cfgPath, []byte(loomengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// seedLandingConfig creates <anchorPath>/_lyx/config/landing.yaml with the embedded template's
// contents. landingshed.LoadConfig is strict (an absent file is an error), so wire() fails on every
// existing test in this file without this seed.
func seedLandingConfig(t *testing.T, anchorPath string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "landing.yaml")
	if err := os.WriteFile(cfgPath, []byte(landingshed.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// seedDiscussionStencil writes stencils.LoomTemplateDiscussion's embedded bytes to
// <hubPath>/<fabricengine.StencilsDir relative form>/loom/loom-template-discussion.md, creating the
// parent directories first. This is the path hubgeom.WebsterGeometry's StencilsDir field actually
// resolves to (fabricengine.StencilsDir(l.HubPath)), and it is what the DiscussionSpec closure
// wire() builds actually reads: stencilstore.Read hard-errors on a missing file rather than falling
// back to the embedded default, so without this seed the closure returns an error and every Spec
// assertion below it is unreachable.
func seedDiscussionStencil(t *testing.T, hubPath string) {
	t.Helper()
	loomDir := filepath.Join(fabricengine.StencilsDir(hubPath), "loom")
	if err := os.MkdirAll(loomDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", loomDir, err)
	}
	cfgPath := filepath.Join(loomDir, "loom-template-discussion.md")
	if err := os.WriteFile(cfgPath, stencils.LoomTemplateDiscussion, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// seedPlanStencil writes stencils.LoomTemplatePlan's embedded bytes to
// <hubPath>/<fabricengine.StencilsDir relative form>/loom/loom-template-plan.md, creating the parent
// directories first, identical in shape to seedDiscussionStencil beside it. This is what the PlanSpec
// closure wire() builds actually reads: stencilstore.Read hard-errors on a missing file rather than
// falling back to the embedded default, so without this seed the closure returns an error and every
// Spec assertion below it is unreachable.
func seedPlanStencil(t *testing.T, hubPath string) {
	t.Helper()
	loomDir := filepath.Join(fabricengine.StencilsDir(hubPath), "loom")
	if err := os.MkdirAll(loomDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", loomDir, err)
	}
	cfgPath := filepath.Join(loomDir, "loom-template-plan.md")
	if err := os.WriteFile(cfgPath, stencils.LoomTemplatePlan, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// seedLoomConfigWithInteractive overwrites <anchorPath>/_lyx/config/loom.yaml with a full seven-key
// literal, identical to the embedded template's own values except discussion_interactive, which
// takes the caller-chosen value. All seven keys are written explicitly, rather than
// string-substituting the template, because configengine.Load is strict on missing keys -- an
// explicit literal is what internal/loomengine/config_test.go already does.
func seedLoomConfigWithInteractive(t *testing.T, anchorPath string, discussionInteractive bool) {
	t.Helper()
	configDir := filepath.Join(anchorPath, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	contents := fmt.Sprintf(`discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: %v
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
`, discussionInteractive)
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// hubLocation returns a *lyxcwd.Location standing in for a real hub location, with its anchor path
// seeded on disk with loom.yaml and landing.yaml, and its hub seeded with both loom stencils.
func hubLocation(t *testing.T, worktreeName, anchorRel string) *lyxcwd.Location {
	t.Helper()
	hub := t.TempDir()
	loc := &lyxcwd.Location{HubPath: hub, WorktreeName: worktreeName, AnchorRel: anchorRel}
	seedLoomConfig(t, loc.AnchorPath())
	seedLandingConfig(t, loc.AnchorPath())
	seedDiscussionStencil(t, hub)
	seedPlanStencil(t, hub)
	return loc
}

// TestWire_PathFieldsMatchLoomengineAccessors asserts every path field of the assembled
// shedrecipe.Env and loomrecipe.ShedPaths equals the corresponding loomengine accessor's own output
// for the same location.
//
// StatusPath and StatusLockPath are told twice -- once on c.env (read by loomPreflightEntry) and
// once on c.shedPaths (read by shedengine.Shed) -- so both copies are asserted against the same
// loomengine accessor, per the deliberate duplication ShedPaths' own doc comment describes.
func TestWire_PathFieldsMatchLoomengineAccessors(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")
	cwd := loc.AnchorPath()

	c := &loomCLI{}
	if err := c.wire(loc, cwd); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if want := loomengine.LoomStatusFile(loc); c.shedPaths.StatusPath != want {
		t.Errorf("c.shedPaths.StatusPath = %q; want %q", c.shedPaths.StatusPath, want)
	}
	if want := loomengine.LoomRunLock(loc); c.shedPaths.LockPath != want {
		t.Errorf("c.shedPaths.LockPath = %q; want %q", c.shedPaths.LockPath, want)
	}
	if want := loomengine.LoomStatusLock(loc); c.shedPaths.StatusLockPath != want {
		t.Errorf("c.shedPaths.StatusLockPath = %q; want %q", c.shedPaths.StatusLockPath, want)
	}
	if want := loc.AnchorPath(); c.env.AnchorPath != want {
		t.Errorf("c.env.AnchorPath = %q; want %q", c.env.AnchorPath, want)
	}
	if want := loc.WorktreePath(); c.env.WorktreeRoot != want {
		t.Errorf("c.env.WorktreeRoot = %q; want %q", c.env.WorktreeRoot, want)
	}
	if want := loomengine.DiscussionDecisionRecord(loc); c.env.DecisionRecordPath != want {
		t.Errorf("c.env.DecisionRecordPath = %q; want %q", c.env.DecisionRecordPath, want)
	}
	if want := loomengine.DiscussionSupportLog(loc); c.env.SupportLogPath != want {
		t.Errorf("c.env.SupportLogPath = %q; want %q", c.env.SupportLogPath, want)
	}
	if want := loomengine.LoomStatusFile(loc); c.env.StatusPath != want {
		t.Errorf("c.env.StatusPath = %q; want %q", c.env.StatusPath, want)
	}
	if want := loomengine.LoomStatusLock(loc); c.env.StatusLockPath != want {
		t.Errorf("c.env.StatusLockPath = %q; want %q", c.env.StatusLockPath, want)
	}
}

// TestWire_RunLockDiffersFromStatusLock asserts the run-lock path differs from the status-lock path,
// the pair shedengine.Shed rejects outright when equal.
func TestWire_RunLockDiffersFromStatusLock(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.shedPaths.LockPath == c.shedPaths.StatusLockPath {
		t.Errorf("c.shedPaths.LockPath == c.shedPaths.StatusLockPath == %q; want them distinct", c.shedPaths.LockPath)
	}
}

// TestWire_CwdIsToldToTheEnv asserts c.env.Cwd equals the cwd argument wire was called with -- the
// only preflight-related property wire still owns.
//
// wire no longer builds the Preflight row itself: preflightEntry now builds it from Env.Cwd,
// exactly the way loomshed.Deps.Preflight used to be built here. The old
// TestWire_PreflightIsTheAdapter assertion ("row 1 is the preflightshed adapter, not a bare func")
// moved with that construction -- it is now internal/shedrecipe's own entry test, and the row's
// engine name is pinned by the recipe-side coverage guard in internal/loomrecipe.
func TestWire_CwdIsToldToTheEnv(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")
	cwd := loc.AnchorPath()

	c := &loomCLI{}
	if err := c.wire(loc, cwd); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.env.Cwd != cwd {
		t.Errorf("c.env.Cwd = %q; want %q", c.env.Cwd, cwd)
	}
}

// TestWire_WebsterRunIsFilled asserts c.env.WebsterRun is non-nil.
//
// This is the single most likely regression the conversion reintroduces: websterEntry calls
// requireSeam("Webster", "WebsterRun", env.WebsterRun) and errors on nil, while the pre-conversion
// wire deliberately left the corresponding field (loomshed.Deps.WebsterRun) nil and relied on
// shedadapters.NewWebsterProducer's own nil-defaulting.
func TestWire_WebsterRunIsFilled(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.env.WebsterRun == nil {
		t.Error("c.env.WebsterRun = nil; want websterengine.Run")
	}
}

// TestWire_WebsterDepsFullyPopulated asserts every field the webster hub wiring fills is non-zero in
// the assembled websterengine.RunDeps.
func TestWire_WebsterDepsFullyPopulated(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	deps := c.runDeps
	if deps.Starter == nil {
		t.Error("runDeps.Starter = nil; want the runnerMasterStarter adapter")
	}
	if deps.Reed == nil {
		t.Error("runDeps.Reed = nil; want the constructed reed engine")
	}
	if deps.Engine == nil {
		t.Error("runDeps.Engine = nil; want the constructed claude engine")
	}
	if deps.Roles == nil {
		t.Error("runDeps.Roles = nil; want the resolved role map")
	}
	if deps.Batcher == nil {
		t.Error("runDeps.Batcher = nil; want the active batchifier")
	}
	if deps.Geom.WebsterDir == "" {
		t.Error("runDeps.Geom.WebsterDir = \"\"; want the told webster geometry")
	}
	if deps.RefMatcher == nil {
		t.Error("runDeps.RefMatcher = nil; want the real fabric reference matcher")
	}
	if deps.OpenBisector == nil {
		t.Error("runDeps.OpenBisector = nil; want the lazy fabric opener")
	}

	// The same value must also be embedded verbatim in c.env.WebsterDeps.
	if c.env.WebsterDeps.Geom != deps.Geom {
		t.Error("c.env.WebsterDeps is not the same value stored in c.runDeps")
	}
}

// TestWire_RefMatcherIsRealScanner asserts the reference matcher is a non-nil *fabricengine.RefScanner
// and never websterengine.NeverMatches, the standalone-only stand-in -- loom is hub-only.
func TestWire_RefMatcherIsRealScanner(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if _, ok := c.runDeps.RefMatcher.(*fabricengine.RefScanner); !ok {
		t.Errorf("runDeps.RefMatcher = %T; want *fabricengine.RefScanner", c.runDeps.RefMatcher)
	}
}

// TestWire_BisectorOpenerNonNilInHubOnlyMode asserts the bisector opener is non-nil, since loom is
// hub-only and always has a fabric repo to open.
func TestWire_BisectorOpenerNonNilInHubOnlyMode(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.runDeps.OpenBisector == nil {
		t.Fatal("runDeps.OpenBisector = nil; want a non-nil closure in hub-only mode")
	}
}

// TestWire_LandingSeamFieldsPopulated asserts wire() populates c.registry, c.runner, and
// c.landingCfg -- the three fields drive.go passes to landingDeps.
//
// c.landingCfg is compared via reflect.DeepEqual, not !=, because landingshed.Config carries a
// RequirePRToBase []string field, which makes the struct non-comparable and would fail to compile
// under a plain != the way TestWire_PathFieldsMatchLoomengineAccessors's plain-string field
// comparisons do.
func TestWire_LandingSeamFieldsPopulated(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.registry == nil {
		t.Error("c.registry = nil; want the resolved model-spec registry")
	}
	if c.runner == nil {
		t.Error("c.runner = nil; want the constructed shuttle runner")
	}

	want, err := landingshed.LoadConfig(loc.AnchorPath(), "landing")
	if err != nil {
		t.Fatalf("landingshed.LoadConfig(%q, \"landing\") = %v; want nil", loc.AnchorPath(), err)
	}
	if !reflect.DeepEqual(c.landingCfg, want) {
		t.Errorf("c.landingCfg = %+v; want %+v", c.landingCfg, want)
	}
}

// TestWire_DiscussionSeamsFilled asserts c.env.Shuttle, c.env.DiscussionSpec, and
// c.env.CommitDiscussion are each non-nil after wire(), and that c.env.Shuttle is the same
// *shuttleengine.Runner value c.runner holds.
func TestWire_DiscussionSeamsFilled(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.env.Shuttle == nil {
		t.Error("c.env.Shuttle = nil; want a non-nil shedadapters.Shuttle")
	}
	if c.env.DiscussionSpec == nil {
		t.Error("c.env.DiscussionSpec = nil; want a non-nil shedadapters.SpecSource")
	}
	if c.env.CommitDiscussion == nil {
		t.Error("c.env.CommitDiscussion = nil; want a non-nil commit closure")
	}
	if c.env.Shuttle != c.runner {
		t.Errorf("c.env.Shuttle = %v; want the same *shuttleengine.Runner value as c.runner = %v", c.env.Shuttle, c.runner)
	}
}

// TestWire_DiscussionSpecEvaluatesToExpectedShape evaluates c.env.DiscussionSpec() for both
// discussion_interactive values and asserts on the returned shuttleengine.Spec's shape. Interactive
// and AwaitOperator must both be false when discussion_interactive is false, and both true when
// discussion_interactive is true; every other assertion (Role, Timeout, Model, OutputFiles, Prompt)
// must hold in both cases.
func TestWire_DiscussionSpecEvaluatesToExpectedShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		discussionInteractive bool
		wantInteractive       bool
		wantAwaitOperator     bool
	}{
		{"Autonomous", false, false, false},
		{"Interactive", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := hubLocation(t, "warp", ".")
			if tt.discussionInteractive {
				seedLoomConfigWithInteractive(t, loc.AnchorPath(), true)
			}

			c := &loomCLI{}
			if err := c.wire(loc, loc.AnchorPath()); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}

			spec, err := c.env.DiscussionSpec()
			if err != nil {
				t.Fatalf("c.env.DiscussionSpec() = %v; want nil", err)
			}

			if spec.Interactive != tt.wantInteractive {
				t.Errorf("spec.Interactive = %v; want %v", spec.Interactive, tt.wantInteractive)
			}
			if spec.AwaitOperator != tt.wantAwaitOperator {
				t.Errorf("spec.AwaitOperator = %v; want %v", spec.AwaitOperator, tt.wantAwaitOperator)
			}
			if spec.Role != "discussion" {
				t.Errorf("spec.Role = %q; want %q", spec.Role, "discussion")
			}
			wantTimeout := time.Duration(c.cfg.DiscussionTimeoutMin) * time.Minute
			if spec.Timeout != wantTimeout {
				t.Errorf("spec.Timeout = %s; want %s", spec.Timeout, wantTimeout)
			}
			if spec.Model == "" {
				t.Error("spec.Model = \"\"; want non-empty")
			}

			wantOutputs := []string{loomengine.DiscussionDecisionRecord(loc), loomengine.DiscussionSupportLog(loc)}
			if !reflect.DeepEqual(spec.OutputFiles, wantOutputs) {
				t.Errorf("spec.OutputFiles = %v; want %v", spec.OutputFiles, wantOutputs)
			}

			if spec.Prompt == "" {
				t.Error("spec.Prompt = \"\"; want non-empty")
			}
			if strings.Contains(spec.Prompt, "{{") {
				t.Errorf("spec.Prompt contains an unrendered {{ marker: %q", spec.Prompt)
			}
		})
	}
}

// TestWire_PlanSeamsFilled asserts c.env.PlanSpec and c.env.CommitPlan are each non-nil after wire().
func TestWire_PlanSeamsFilled(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.env.PlanSpec == nil {
		t.Error("c.env.PlanSpec = nil; want a non-nil shedadapters.SpecSource")
	}
	if c.env.CommitPlan == nil {
		t.Error("c.env.CommitPlan = nil; want a non-nil commit closure")
	}
}

// TestWire_ReviewSegmentSeamsFilled asserts the four Env fields both review segments read
// (StencilsDir, RunRoot, Burler, Now) are filled after wire() -- shared by Discussion-Bouncer/
// Discussion-Burler and Plan-Bouncer/Plan-Burler alike -- following
// TestWire_PathFieldsMatchLoomengineAccessors' convention of asserting an Env path field against
// its own loomengine/fabricengine accessor rather than a re-derived literal.
func TestWire_ReviewSegmentSeamsFilled(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if want := fabricengine.StencilsDir(loc.HubPath); c.env.StencilsDir != want {
		t.Errorf("c.env.StencilsDir = %q; want %q", c.env.StencilsDir, want)
	}
	if want := loomengine.LoomReviewsDir(loc); c.env.RunRoot != want {
		t.Errorf("c.env.RunRoot = %q; want %q", c.env.RunRoot, want)
	}
	if c.env.Burler == nil {
		t.Error("c.env.Burler = nil; want a non-nil shedadapters.BurlerRunner")
	}
	if c.env.Now == nil {
		t.Error("c.env.Now = nil; want a non-nil clock")
	}
}

// TestWire_ReviewTripleMatchesLoadedConfig asserts c.env.ReviewModel, c.env.ReviewEffort,
// c.env.ReviewVersion, and c.env.ReviewTimeout equal what loomengine.ResolveReview returns for the
// same loaded config and registry -- resolved in the test rather than hardcoded against the
// template's literal spec, so a later template edit does not silently break this assertion's
// meaning.
func TestWire_ReviewTripleMatchesLoadedConfig(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	registry, err := modelspec.LoadRegistry(loc.AnchorPath())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(%q) = %v; want nil", loc.AnchorPath(), err)
	}
	want, err := loomengine.ResolveReview(c.cfg, registry)
	if err != nil {
		t.Fatalf("loomengine.ResolveReview(c.cfg, registry) = %v; want nil", err)
	}

	if c.env.ReviewModel != want.Model {
		t.Errorf("c.env.ReviewModel = %q; want %q", c.env.ReviewModel, want.Model)
	}
	if c.env.ReviewEffort != want.Effort {
		t.Errorf("c.env.ReviewEffort = %q; want %q", c.env.ReviewEffort, want.Effort)
	}
	if c.env.ReviewVersion != want.Version {
		t.Errorf("c.env.ReviewVersion = %q; want %q", c.env.ReviewVersion, want.Version)
	}
	if c.env.ReviewTimeout != want.Timeout {
		t.Errorf("c.env.ReviewTimeout = %s; want %s", c.env.ReviewTimeout, want.Timeout)
	}
}

// TestWire_PlanSpecEvaluatesToExpectedShape evaluates c.env.PlanSpec() once and asserts on the
// returned shuttleengine.Spec's shape.
func TestWire_PlanSpecEvaluatesToExpectedShape(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	spec, err := c.env.PlanSpec()
	if err != nil {
		t.Fatalf("c.env.PlanSpec() = %v; want nil", err)
	}

	if spec.Interactive {
		t.Error("spec.Interactive = true; want false (autonomous by design)")
	}
	if spec.Role != "plan" {
		t.Errorf("spec.Role = %q; want %q", spec.Role, "plan")
	}
	wantTimeout := time.Duration(c.cfg.PlanTimeoutMin) * time.Minute
	if spec.Timeout != wantTimeout {
		t.Errorf("spec.Timeout = %s; want %s", spec.Timeout, wantTimeout)
	}
	if spec.Model == "" {
		t.Error("spec.Model = \"\"; want non-empty")
	}

	wantOutputs := []string{planparser.PlanOverview(loc.AnchorPath())}
	if !reflect.DeepEqual(spec.OutputFiles, wantOutputs) {
		t.Errorf("spec.OutputFiles = %v; want %v", spec.OutputFiles, wantOutputs)
	}

	if spec.Prompt == "" {
		t.Error("spec.Prompt = \"\"; want non-empty")
	}
	if strings.Contains(spec.Prompt, "{{") {
		t.Errorf("spec.Prompt contains an unrendered {{ marker: %q", spec.Prompt)
	}
}

// TestVerbReadsStatusOnly pins the exact set of verbs that skip the full engine-stack construction.
// Adding a verb here silently would be a real regression: a verb that builds or drives producers
// needs wire()'s early config refusal, and getting that wrong moves the failure from the operator's
// terminal into a detached driver log.
func TestVerbReadsStatusOnly(t *testing.T) {
	tests := []struct {
		name string
		verb string
		want bool
	}{
		{"Status", "status", true},
		{"Pause", "pause", true},
		{"Run", "run", false},
		{"Drive", "drive", false},
		{"ValidateDiscussion", "validate-discussion", false},
		{"ValidatePlan", "validate-plan", false},
		{"UnknownVerb", "something-else", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := verbReadsStatusOnly(tt.verb); got != tt.want {
				t.Errorf("verbReadsStatusOnly(%q) = %v; want %v", tt.verb, got, tt.want)
			}
		})
	}
}

// TestWireStatusPathsOnly_FillsTheStatusPathsWithoutLoadingAnyConfig is the regression guard for the
// defect: "lyx loom pause" and "lyx loom status" used to run the whole of wire(), so a fault in any
// of eight module configs refused them both -- taking away the operator's read-out and the
// documented emergency brake for a run that was still going. The location fixture here has no
// _lyx/config directory at all, which is the strongest form of "no config is loaded".
func TestWireStatusPathsOnly_FillsTheStatusPathsWithoutLoadingAnyConfig(t *testing.T) {
	// Deliberately NOT hubLocation: this location's anchor has no _lyx/config directory at all, so
	// a path that loaded any module config could not possibly succeed here.
	location := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}

	c := &loomCLI{}
	c.wireStatusPathsOnly(location, location.AnchorPath())

	if c.location != location {
		t.Errorf("location = %v; want the told location", c.location)
	}
	if c.shedPaths.StatusPath != loomengine.LoomStatusFile(location) {
		t.Errorf("StatusPath = %q; want %q", c.shedPaths.StatusPath, loomengine.LoomStatusFile(location))
	}
	if c.shedPaths.StatusLockPath != loomengine.LoomStatusLock(location) {
		t.Errorf("StatusLockPath = %q; want %q", c.shedPaths.StatusLockPath, loomengine.LoomStatusLock(location))
	}
	if c.shedPaths.LockPath != loomengine.LoomRunLock(location) {
		t.Errorf("LockPath = %q; want %q", c.shedPaths.LockPath, loomengine.LoomRunLock(location))
	}
	if c.shedPaths.LockPath == c.shedPaths.StatusLockPath {
		t.Error("LockPath and StatusLockPath name the same file; shedengine.validate rejects that outright")
	}
	// Nothing that a module config would have filled may be populated: that is what proves no load
	// happened rather than merely that none failed.
	if c.reed != nil {
		t.Error("reed engine was constructed; want the status-only path to build no engine")
	}
	if c.runner != nil {
		t.Error("shuttle runner was constructed; want the status-only path to build no engine")
	}
	if c.env.AnchorPath != "" {
		t.Errorf("env was assembled (AnchorPath = %q); want the status-only path to assemble no Env", c.env.AnchorPath)
	}
}
