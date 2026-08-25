// fixture_test.go implements the package-internal test scaffolding every later test file in this
// package reuses: newTestEnv, the filled-Env builder, and the fake Shuttle/BurlerRunner/
// WebsterRunner/RunDeps-seam implementations it fills that Env with.

package shedrecipe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeShuttle implements shedadapters.Shuttle by returning a caller-settable shuttleengine.Result
// and error, recording every shuttleengine.Spec it was handed so a later test can assert on the
// composed spec.
type fakeShuttle struct {
	result shuttleengine.Result
	err    error
	specs  []shuttleengine.Spec
}

var _ shedadapters.Shuttle = (*fakeShuttle)(nil)

// Run implements shedadapters.Shuttle: it records spec and returns f's caller-settable result/err.
func (f *fakeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.specs = append(f.specs, spec)
	return f.result, f.err
}

// Attach implements shedadapters.Shuttle's probe method by always reporting not-found, so every
// existing sequence in this package still drives the unchanged archive-then-run path through Run.
func (f *fakeShuttle) Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	return shuttleengine.Result{}, false, nil
}

// fakeBurlerRunner implements shedadapters.BurlerRunner by returning a zero burlerengine.Result and
// a nil error, recording every burlerengine.Profile and burlerengine.RunOpts it was handed.
type fakeBurlerRunner struct {
	profiles []burlerengine.Profile
	opts     []burlerengine.RunOpts
}

var _ shedadapters.BurlerRunner = (*fakeBurlerRunner)(nil)

// Run implements shedadapters.BurlerRunner: it records p and opts and returns a zero Result with a
// nil error.
func (f *fakeBurlerRunner) Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error) {
	f.profiles = append(f.profiles, p)
	f.opts = append(f.opts, opts)
	return burlerengine.Result{}, nil
}

// fakeWebsterRun is a shedadapters.WebsterRunner func value returning a zero
// websterengine.RunResult and a nil error.
var fakeWebsterRun shedadapters.WebsterRunner = func(websterengine.RunDeps, websterengine.RunOptions) (websterengine.RunResult, error) {
	return websterengine.RunResult{}, nil
}

// fakeMasterStarter, fakeReedOps, fakeShuttleEngine, and fakeRefMatcher satisfy
// websterengine.RunDeps' four required seams by embedding the seam interface in an empty struct,
// which yields a non-nil value satisfying the interface without implementing a single method. Each
// is a placeholder for a non-nil check only: calling any promoted method panics on the embedded nil
// interface, which no test in this package does -- websterEntry's own Env validation stops at
// requireSeam's non-nil check and never calls through any of these seams.
type (
	fakeMasterStarter struct{ websterengine.MasterStarter }
	fakeReedOps       struct{ shuttleengine.ReedOps }
	fakeShuttleEngine struct{ shuttleengine.Engine }
	fakeRefMatcher    struct{ websterengine.RefMatcher }
)

// newTestEnv builds an Env whose every path field is an absolute path derived from a single
// t.TempDir(), one subdirectory per field: a directory field (Cwd, WorktreeRoot, StencilsDir,
// RunRoot, AnchorPath) is created with os.MkdirAll, while a file field (StatusPath,
// StatusLockPath, DecisionRecordPath, SupportLogPath) is left as a joined path nobody creates. It
// fills Shuttle, Burler, and WebsterRun with this file's fakes, fills WebsterDeps with the four
// required seams non-nil and every other field left zero, fills DiscussionSpec with a closure
// returning a shuttleengine.Spec over one absolute output path under the same temp root, fills
// CommitDiscussion with a closure returning nil, fills PlanSpec with a closure returning a
// shuttleengine.Spec over one absolute output path under the same temp root, fills CommitPlan with
// a closure returning nil, leaves Landing zero, and leaves Now nil.
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
		Cwd:                mustMkdir("cwd"),
		AnchorPath:         mustMkdir("anchor"),
		WorktreeRoot:       mustMkdir("worktree"),
		StatusPath:         filepath.Join(dir, "status.json"),
		StatusLockPath:     filepath.Join(dir, "status.json.lock"),
		StencilsDir:        mustMkdir("stencils"),
		RunRoot:            mustMkdir("run-root"),
		DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
		SupportLogPath:     filepath.Join(dir, "support-log.md"),
		Shuttle:            &fakeShuttle{},
		Burler:             &fakeBurlerRunner{},
		WebsterRun:         fakeWebsterRun,
		WebsterDeps: websterengine.RunDeps{
			Starter:    fakeMasterStarter{},
			Reed:       fakeReedOps{},
			Engine:     fakeShuttleEngine{},
			RefMatcher: fakeRefMatcher{},
		},
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
	}
}
