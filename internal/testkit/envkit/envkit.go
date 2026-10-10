// Package envkit builds the fully filled seam structs the recipe tests share: a `shedrecipe.Env`, a `landingshed.Deps` and a `loomshed.PRReworkDeps`.
//
// A new required field is added to the kit once, instead of to every package's private copy.
// Builders return plain values the caller mutates; there are no option structs, and a builder fails the test only on its own setup.
// NilSeams reports and never asserts, so the CLI wiring tests and this kit's own test assert on its answer.
//
// `shedrecipe` and `landingshed` cannot import the kit without a cycle, so each keeps one local builder.
package envkit

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// nilLegal holds the dotted paths NilSeams skips: seams whose nil is a documented default rather than a missing wiring.
var nilLegal = map[string]bool{
	"Now":                                              true,
	"GateSlots":                                        true,
	"VerifyMergeBase":                                  true,
	"PublishFailure":                                   true,
	"Landing.CommitStatus":                             true,
	"Landing.CommitParentRecords":                      true,
	"Landing.MergeState":                               true,
	"Landing.AbortMerge":                               true,
	"Landing.StopConflictSession":                      true,
	"Landing.MarkTaskDone":                             true,
	"Landing.ConfigChanges":                            true,
	"Landing.Notify":                                   true,
	"Landing.VerifyCommand":                            true,
	"Landing.VerifyWaitMark":                           true,
	"Landing.GateSlots":                                true,
	"Landing.FailingTests":                             true,
	"Landing.Registry":                                 true,
	"ParentReview.Store":                               true,
	"ParentReview.ReviewerLive":                        true,
	"ParentReview.RenderDelivery":                      true,
	"ParentReview.RenderBrief":                         true,
	"WebsterDeps.Batcher":                              true,
	"WebsterDeps.Roles":                                true,
	"WebsterDeps.ParentBranch":                         true,
	"WebsterDeps.Geom.Git":                             true,
	"WebsterDeps.Geom.GateSlots":                       true,
	"WebsterDeps.ShuttleCfg.ClaudePromptCacheTTLRoles": true,
	"InnerRun.Sleep":                                   true,
	"InnerRun.ReviewWait":                              true,
	"InnerRun.Now":                                     true,
	"InnerRun.Notify":                                  true,
	"InnerRun.AttachDir":                               true,
	"PrimeLock.Sleep":                                  true,
	"SegmentBounces":                                   true,
	"RowReviewModels":                                  true,
	"RowClusterFans":                                   true,
}

// noFindingsIndex is a planindex.Index that answers no findings, so the packages built on the kit do not link the resolve-backed index.
// Its Delta panics: a test that needs a delta supplies its own index.
type noFindingsIndex struct{ planindex.Index }

func (noFindingsIndex) ValidateFormat(*planparser.Plan, string, []planparser.Card) ([]planindex.Finding, error) {
	return nil, nil
}

func (noFindingsIndex) ValidateRework(*planparser.Plan, string, int) ([]planindex.Finding, error) {
	return nil, nil
}

// nilFabricOpener is a typed-nil fabric handle with a nil error.
// The Publish and Finalize constructors only nil-check the interface the handle is stored behind, and never dereference it.
func nilFabricOpener() (*fabricengine.Fabric, error) {
	return nil, nil
}

// LandingDeps returns a `landingshed.Deps` with every field the Publish and Finalize constructors require, filled with told values derived from dir.
func LandingDeps(dir string) landingshed.Deps {
	return landingshed.Deps{
		WorktreeRoot:      dir,
		TaskBranch:        "task-branch",
		ParentBranch:      "fixture-parent",
		DescriptionPath:   summaryparser.Path(dir),
		StencilsDir:       dir,
		ParentName:        "fixture-parent-session",
		ScratchDir:        filepath.Join(dir, "landing-scratch"),
		OriginURL:         "https://example.invalid/fixture/fixture.git",
		PushBranch:        func() error { return nil },
		RemoteOnlyCommits: func() (string, []string, error) { return "", nil, nil },
		OpenFabric:        nilFabricOpener,
		OpenParentFabric:  nilFabricOpener,
		Shuttle:           &shedfake.MergeShuttle{},
		ApprovalPath:      filepath.Join(dir, "approval.json"),
		RejectionPath:     filepath.Join(dir, "rejection.json"),
		TaskHead:          func() (string, error) { return "head", nil },
	}
}

// ReworkDeps returns a `loomshed.PRReworkDeps` whose directories exist under dir and whose closures are no-ops.
func ReworkDeps(t testing.TB, dir string) loomshed.PRReworkDeps {
	t.Helper()
	return loomshed.PRReworkDeps{
		PlanDir:       mustMkdir(t, filepath.Join(dir, "rework-plan")),
		ReworkDir:     mustMkdir(t, filepath.Join(dir, "rework-rounds")),
		ReworkDirRel:  "_lyx/loom/rework",
		ReviewsDir:    mustMkdir(t, filepath.Join(dir, "rework-reviews")),
		ReadCommitted: func(string) ([]byte, bool, error) { return nil, false, nil },
		ReadRejection: func() (loomshed.PendingRejection, bool, error) {
			return loomshed.PendingRejection{}, false, nil
		},
		ClearRejection: func() error { return nil },
		ArchiveWebster: func(string) error { return nil },
		Commit:         func() error { return nil },
	}
}

// FullEnv returns a `shedrecipe.Env` with every path field an absolute path under one `t.TempDir()` and every seam any registered engine requires filled.
// A seam added to the registry's requirements must be filled here, which NilSeams(FullEnv(t)) catches.
func FullEnv(t testing.TB) shedrecipe.Env {
	t.Helper()
	dir := t.TempDir()
	specOver := func(prompt, output string) func() (shuttleengine.Spec, error) {
		return func() (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      prompt,
				OutputFiles: []string{filepath.Join(dir, output)},
			}, nil
		}
	}

	return shedrecipe.Env{
		Cwd:                mustMkdir(t, filepath.Join(dir, "cwd")),
		AnchorPath:         mustMkdir(t, filepath.Join(dir, "anchor")),
		WorktreeRoot:       mustMkdir(t, filepath.Join(dir, "worktree")),
		VerifyDir:          mustMkdir(t, filepath.Join(dir, "verify")),
		VerifyCommand:      func() (string, error) { return "go test ./...", nil },
		StatusPath:         filepath.Join(dir, "status.json"),
		StatusLockPath:     filepath.Join(dir, "status.json.lock"),
		StencilsDir:        mustMkdir(t, filepath.Join(dir, "stencils")),
		SpecsDir:           mustMkdir(t, filepath.Join(dir, "specs")),
		RunRoot:            mustMkdir(t, filepath.Join(dir, "run-root")),
		RunScratchDir:      mustMkdir(t, filepath.Join(dir, "run-scratch")),
		DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
		SupportLogPath:     filepath.Join(dir, "support-log.md"),
		DescriptionPath:    filepath.Join(dir, "description.md"),
		Shuttle:            &shedfake.Shuttle{},
		Burler:             &shedfake.BurlerRunner{},
		Seats:              &shedfake.SeatRunner{},
		Models:             modelspec.Registry{"opus": {Engine: "claude", Model: "claude-opus-test"}},
		WebsterRun: func(websterengine.RunDeps, websterengine.RunOptions) (websterengine.RunResult, error) {
			return websterengine.RunResult{}, nil
		},
		WebsterDeps:    shedfake.WebsterSeams(),
		CommitWebster:  func() error { return nil },
		Landing:        LandingDeps(mustMkdir(t, filepath.Join(dir, "landing"))),
		DiscussionSpec: specOver("test discussion prompt", "discussion-output.md"),
		DiscussionTable: func() (seatengine.Table, error) {
			return seatengine.Table{
				RolePrefix: "discussion",
				Segment:    segmentcolor.Discussion,
				Seats: []seatengine.Seat{{
					Name:    seatengine.RoleChair,
					Stencil: "loom-template-discussion-chair",
					Outputs: []string{filepath.Join(dir, "discussion-output.md")},
				}},
			}, nil
		},
		CommitDiscussion:  func() error { return nil },
		DescribeSpec:      specOver("test describe prompt", "description.md"),
		CommitDescription: func() error { return nil },
		PlanSpec:          specOver("test plan prompt", "plan-output.md"),
		CommitPlan:        func() error { return nil },
		ApprovePlan:       func() error { return nil },
		SkipPlanReview:    func() (bool, error) { return false, nil },
		CarryOver:         func(discussionparser.CarryOver) error { return nil },
		ReflectFriction:   func() string { return "skipped" },
		ReworkSpec: func(loomshed.ReworkTold) (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      "test rework prompt",
				OutputFiles: []string{filepath.Join(dir, "rework-coverage.md")},
			}, nil
		},
		Rework: ReworkDeps(t, dir),
		DarnSpec: func(loomshed.DarnTold) (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      "test darn prompt",
				OutputFiles: []string{filepath.Join(dir, "darn-description.md")},
			}, nil
		},
		Darn: loomshed.DarnDeps{
			ReadRejection: func() (loomshed.PendingRejection, bool, error) {
				return loomshed.PendingRejection{}, false, nil
			},
			ClearRejection: func() error { return nil },
			Commit:         func() error { return nil },
			LatestOutcome:  func() (loomshed.DarnOutcome, bool, error) { return loomshed.DarnOutcome{}, false, nil },
			PublishFailure: func() (string, bool, error) { return "", false, nil },
		},
		PlanIndex: noFindingsIndex{},

		Slug:                     "test-slug",
		ReviewMaxBounces:         3,
		ReviewCirclingCheckpoint: 3,

		ScratchDir:     mustMkdir(t, filepath.Join(dir, "scratch")),
		CreateWorktree: func(context.Context) error { return nil },
		InnerRun: battenshed.InnerRunDeps{
			Spawn: func(context.Context) error { return nil },
			ResolveStatus: func() (string, string, error) {
				return filepath.Join(dir, "loomrun-status.json"), filepath.Join(dir, "loomrun-status.json.lock"), nil
			},
			ReadStatus: func(string, string) (shedengine.Status, bool, error) {
				return shedengine.Status{}, false, nil
			},
			ReadDecision: func() (battenshed.ChildDecision, bool, error) {
				return battenshed.ChildDecision{}, false, nil
			},
			DriverAlive: func(context.Context) (bool, error) { return false, nil },
			DriverStrand: func(context.Context) (battenshed.ChildDriverStrand, error) {
				return battenshed.ChildDriverNone, nil
			},
			ChildRunLockHeld:   func() (bool, error) { return false, nil },
			ReviveStrands:      func(context.Context) error { return nil },
			PauseRequested:     func() (bool, error) { return false, nil },
			MarkWatched:        func(context.Context) (bool, error) { return false, nil },
			OrchStrandRecorded: func() (bool, error) { return true, nil },
			StopReport:         func() (string, time.Time, bool, error) { return "", time.Time{}, false, nil },
			Activity: func(context.Context) ([]battenshed.AgentActivity, bool, error) {
				return nil, false, nil
			},
		},
		Teardown: battenshed.TeardownDeps{
			Shutdown: func(context.Context) (string, error) { return "", nil },
			Remove:   func(context.Context) error { return nil },
		},
		PrimeLock: battenshed.PrimeLock{
			Path: filepath.Join(dir, "prime.lock"),
			Acquire: func() (func() error, bool, error) {
				return func() error { return nil }, true, nil
			},
		},
		SeedChild: battenshed.SeedChildDeps{
			ReadBoardType: func(context.Context) (string, error) { return "loom", nil },
			ChildDriver:   func() (string, error) { return "claude", nil },
			WriteSeed:     func(context.Context, string, string) error { return nil },
			CommitSeed:    func(context.Context) error { return nil },
			PushSeed:      func(context.Context) error { return nil },
			MarkWatched:   func(context.Context) (bool, error) { return false, nil },
		},
	}
}

// NilSeams returns the dotted path of every func, interface, pointer, map and slice-of-func field of env that is nil, recursing into nested structs.
// It skips the paths in nilLegal, whose nil is a documented default.
// It reports and never asserts.
func NilSeams(env shedrecipe.Env) []string {
	var nils []string
	walkNil(reflect.ValueOf(env), "", &nils)
	return nils
}

func walkNil(v reflect.Value, prefix string, nils *[]string) {
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		path := field.Name
		if prefix != "" {
			path = prefix + "." + field.Name
		}
		if nilLegal[path] {
			continue
		}
		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.Struct:
			walkNil(fv, path, nils)
		case reflect.Func, reflect.Interface, reflect.Pointer, reflect.Map:
			if fv.IsNil() {
				*nils = append(*nils, path)
			}
		case reflect.Slice:
			if fv.IsNil() && fv.Type().Elem().Kind() == reflect.Func {
				*nils = append(*nils, path)
			}
		}
	}
}

func mustMkdir(t testing.TB, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	return path
}
