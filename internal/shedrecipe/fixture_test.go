// fixture_test.go implements the package-internal test scaffolding every later test file in this package reuses: newTestEnv, the filled-Env builder, and the fake WebsterRunner it fills that Env with alongside the shedfake seams.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// newTestEnv builds an Env whose every path field is an absolute path derived from a single t.TempDir(), one subdirectory per field: a directory field (Cwd, WorktreeRoot, StencilsDir, SpecsDir, RunRoot, AnchorPath, ScratchDir) is created with os.MkdirAll, while a file field (StatusPath, StatusLockPath, DecisionRecordPath, SupportLogPath, PrimeLock.Path) is left as a joined path nobody creates.
// It fills Shuttle and Burler with shedfake's fakes and WebsterRun with this file's fake, fills WebsterDeps with shedfake.WebsterSeams, fills DiscussionSpec with a closure returning a shuttleengine.Spec over one absolute output path under the same temp root, fills CommitDiscussion with a closure returning nil, fills PlanSpec with a closure returning a shuttleengine.Spec over one absolute output path under the same temp root, fills CommitPlan with a closure returning nil, leaves Landing zero, and leaves Now nil.
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
		WorktreeRoot:   mustMkdir("worktree"),
		VerifyDir:      mustMkdir("verify"),
		StatusPath:     filepath.Join(dir, "status.json"),
		StatusLockPath: filepath.Join(dir, "status.json.lock"),
		StencilsDir:    mustMkdir("stencils"),
		SpecsDir:       mustMkdir("specs"),
		// Inside the anchor, as in production: the round's ready marker is derived from a review path under the run root and must lie inside the anchor.
		RunRoot:            mustMkdir("anchor/run-root"),
		DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
		SupportLogPath:     filepath.Join(dir, "support-log.md"),
		Shuttle:            &shedfake.Shuttle{},
		Burler:             &shedfake.BurlerRunner{},
		WebsterRun:         fakeWebsterRun,
		CommitWebster:      func() error { return nil },
		ReflectFriction:    func() string { return "skipped" },
		WebsterDeps:        shedfake.WebsterSeams(),
		PlanIndex:          placeholderIndex{},
		DiscussionSpec: func() (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      "test discussion prompt",
				OutputFiles: []string{filepath.Join(dir, "discussion-output.md")},
				Interactive: false,
			}, nil
		},
		CommitDiscussion: func() error { return nil },
		PlanSpec: func() (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      "test plan prompt",
				OutputFiles: []string{filepath.Join(dir, "plan-output.md")},
				Interactive: false,
			}, nil
		},
		CommitPlan: func() error { return nil },
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
		PrimeLock: battenshed.PrimeLock{
			Path: filepath.Join(dir, "prime.lock"),
			Acquire: func() (func() error, bool, error) {
				return func() error { return nil }, true, nil
			},
		},
	}
}
