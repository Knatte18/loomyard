// fixture_test.go implements the package-internal test scaffolding every later test file in this package reuses: newTestEnv, the filled-Env builder, and the fake WebsterRunner it fills that Env with alongside the shedfake seams.

package shedrecipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeWebsterRun is a shedadapters.WebsterRunner func value returning a zero
// websterengine.RunResult and a nil error.
var fakeWebsterRun shedadapters.WebsterRunner = func(websterengine.RunDeps, websterengine.RunOptions) (websterengine.RunResult, error) {
	return websterengine.RunResult{}, nil
}

// placeholderIndex is a non-nil planindex.Index for constructor checks: it panics if a method is called, which no test here does.
type placeholderIndex struct{ planindex.Index }

// noFindingsPlanIndex is a planindex.Index whose ValidateFormat and ValidateRework, the methods the plan gates call, answer no findings and a nil error; any other method panics.
type noFindingsPlanIndex struct{ planindex.Index }

func (noFindingsPlanIndex) ValidateFormat(*planparser.Plan, string, []planparser.Card) ([]planindex.Finding, error) {
	return nil, nil
}

func (noFindingsPlanIndex) ValidateRework(*planparser.Plan, string, int) ([]planindex.Finding, error) {
	return nil, nil
}

// symbolRenamePlan returns a one-card plan whose card renames a symbol, and so needs the hub's rename tool.
func symbolRenamePlan() plankit.Plan {
	return plankit.Plan{
		Approved: true,
		Language: "none",
		Framing:  "Framing.",
		Sections: []plankit.Section{{Heading: "Rename mechanic", Body: "1. Run the rename tool."}},
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "first-card",
			Summary: "placeholder",
			Groups:  []plankit.Group{{Label: "Rename", Targets: []string{"`sub#Foo` -> `plan:sub#Bar`"}}},
			Intent:  "placeholder.",
		}},
	}
}

// writeStencilFile writes content to the on-disk location stencilstore.Read(dir, name) resolves name to, creating any parent directory the family convention requires.
func writeStencilFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := stencilstore.Path(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir stencil parent %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write stencil %s: %v", path, err)
	}
}

// testReviewSegment is the segment every Bouncer, BurlerReview and BurlerFix test row belongs to.
const testReviewSegment = "Test-Review"

// fakeSetting returns a setting lookup answering values["module.key"], and an error naming the reference for one values lacks.
func fakeSetting(values map[string]any) func(module, key string) (any, error) {
	return func(module, key string) (any, error) {
		reference := module + "." + key
		value, ok := values[reference]
		if !ok {
			return nil, fmt.Errorf("fake setting lookup: unknown setting %s", reference)
		}
		return value, nil
	}
}

// reviewRow returns the identity of a review-segment row called name, in testReviewSegment.
func reviewRow(name string) RowIdentity {
	return RowIdentity{Name: name, Segment: testReviewSegment}
}

// newTestEnv builds an Env whose every path field is an absolute path derived from a single t.TempDir(), one subdirectory per field: a directory field (Cwd, WorktreeRoot, StencilsDir, SpecsDir, RunRoot, AnchorPath, ScratchDir) is created with os.MkdirAll, while a file field (StatusPath, StatusLockPath, DecisionRecordPath, SupportLogPath, RelevantPath, PrimeLock.Path) is left as a joined path nobody creates.
// It fills Shuttle and Burler with shedfake's fakes and WebsterRun with this file's fake, fills WebsterDeps with shedfake.WebsterSeams, fills DiscussionTable with a closure returning a one-chair seatengine.Table over one absolute output path under the same temp root, fills CommitDiscussion with a closure returning nil, fills PlanTable with a closure returning a one-lead seatengine.Table over one absolute output path under the same temp root, fills CommitPlan with a closure returning nil, leaves Landing zero, and leaves Now nil.
//
// It also fills the six batten fields: a non-empty Slug, CreateWorktree returning nil, InnerRun
// and Teardown whose own closures return nil or zero values, and a PrimeLock whose Path sits under
// the same temp root and whose Acquire seam returns a no-op release with ok == true.
//
// No test in this package may reference a path outside its own t.TempDir(): a real repo path would
// mask a told-geometry violation, which is the exact property this package's own Env validation
// exists to enforce.
func newTestEnv(t *testing.T) Env {
	t.Helper()

	dir := t.TempDir()

	mustMkdir := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		return p
	}

	return Env{
		Cwd:            mustMkdir("cwd"),
		AnchorPath:     mustMkdir("anchor"),
		ConfigDirs:     configengine.Dirs{Anchor: filepath.Join(dir, "anchor")},
		WorktreeRoot:   mustMkdir("worktree"),
		VerifyDir:      mustMkdir("verify"),
		VerifyCommand:  func() (string, error) { return "go test ./...", nil },
		StatusPath:     filepath.Join(dir, "status.json"),
		StatusLockPath: filepath.Join(dir, "status.json.lock"),
		StencilsDir:    mustMkdir("stencils"),
		SpecsDir:       mustMkdir("specs"),
		// Inside the anchor, as in production.
		RunRoot:            mustMkdir("anchor/run-root"),
		DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
		SupportLogPath:     filepath.Join(dir, "support-log.md"),
		RelevantPath:       filepath.Join(dir, "relevant.md"),
		Shuttle:            &shedfake.Shuttle{},
		Burler:             &shedfake.BurlerHalfRunner{},
		Seats:              &shedfake.SeatRunner{},
		Models:             modelspec.Registry{"opus": {Engine: "claude", Model: "claude-opus-test"}},
		Segments:           map[string]SegmentSettings{testReviewSegment: {}},
		WebsterRun:         fakeWebsterRun,
		CommitWebster:      func() error { return nil },
		ReflectFriction:    func() string { return "skipped" },
		WebsterDeps:        shedfake.WebsterSeams(),
		PlanIndex:          placeholderIndex{},
		DiscussionTable: func() (seatengine.Table, error) {
			return seatengine.Table{
				RolePrefix: "discussion",
				Segment:    segmentcolor.Discussion,
				Seats: []seatengine.Seat{{
					Stencil: "loom-template-discussion-chair",
					Outputs: []string{filepath.Join(dir, "discussion-output.md")},
				}},
			}, nil
		},
		CommitDiscussion: func() error { return nil },
		PlanTable: func() (seatengine.Table, error) {
			return seatengine.Table{
				RolePrefix: "plan",
				Segment:    segmentcolor.Plan,
				Seats: []seatengine.Seat{{
					Stencil: "loom-template-plan",
					Outputs: []string{filepath.Join(dir, "plan-output.md")},
				}},
			}, nil
		},
		CommitPlan: func() error { return nil },
		Rework:     loomshed.PRReworkDeps{ArchiveWebster: func(string) error { return nil }},
		Slug:       "test-slug",
		ScratchDir: mustMkdir("scratch"),

		ReviewCirclingCheckpoint: 3,

		CreateWorktree: func(context.Context) error {
			return nil
		},
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
		},
		Teardown: battenshed.TeardownDeps{
			Shutdown: func(context.Context) (string, error) { return "", nil },
			Remove:   func(context.Context) error { return nil },
		},
		Land: battenshed.LandDeps{
			ParentBranch: func() (string, error) { return "main", nil },
			PrimeBranch:  func() (string, error) { return "main", nil },
			PrimeDirty:   func() ([]string, error) { return nil, nil },
			Pull:         func(context.Context) error { return nil },
			Landing:      func() (battenshed.LandingSettings, error) { return battenshed.LandingSettings{}, nil },
			RunDeploy: func(context.Context, string, string, time.Duration) (battenshed.DeployResult, error) {
				return battenshed.DeployResult{}, nil
			},
			ReadyTasks:     func() ([]string, error) { return nil, nil },
			PruneBoard:     func() error { return nil },
			PauseRequested: func() (bool, error) { return false, nil },
			LandLock: battenshed.PrimeLock{
				Path: filepath.Join(dir, "land.lock"),
				Acquire: func() (func() error, bool, error) {
					return func() error { return nil }, true, nil
				},
			},
		},
		PrimeLock: battenshed.PrimeLock{
			Path: filepath.Join(dir, "prime.lock"),
			Acquire: func() (func() error, bool, error) {
				return func() error { return nil }, true, nil
			},
		},
	}
}
