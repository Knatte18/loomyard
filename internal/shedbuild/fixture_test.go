// fixture_test.go implements the package-internal test scaffolding every later test file in this
// package reuses: newTestEnv, the filled-shedrecipe.Env builder, testLandingDeps, its own paired
// landingshed.Deps builder, and the fake Shuttle/BurlerRunner/WebsterRunner/RunDeps-seam/
// mergeresolve.Shuttle/fabric-opener implementations they fill Env with. It is a fresh copy rather
// than an import of internal/shedrecipe's fixture_test.go, because that file lives in package
// shedrecipe and is not reachable from here.

package shedbuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeShuttle implements shedadapters.Shuttle by returning a caller-settable shuttleengine.Result
// and error, recording every shuttleengine.Spec it was handed. It is never called: no test in this
// package invokes a producer's Call.
type fakeShuttle struct {
	result shuttleengine.Result
	err    error
	specs  []shuttleengine.Spec
}

var _ shedadapters.Shuttle = (*fakeShuttle)(nil)

// Run implements shedadapters.Shuttle; see fakeShuttle's own doc comment.
func (f *fakeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.specs = append(f.specs, spec)
	return f.result, f.err
}

// Attach implements shedadapters.Shuttle's probe method by always reporting not-found; see
// fakeShuttle's own doc comment for why it is never called.
func (f *fakeShuttle) Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	return shuttleengine.Result{}, false, nil
}

// fakeBurlerRunner implements shedadapters.BurlerRunner by returning a zero burlerengine.Result and
// a nil error, recording every burlerengine.Profile and burlerengine.RunOpts it was handed. It is
// never called: no test in this package invokes a producer's Call.
type fakeBurlerRunner struct {
	profiles []burlerengine.Profile
	opts     []burlerengine.RunOpts
}

var _ shedadapters.BurlerRunner = (*fakeBurlerRunner)(nil)

// Run implements shedadapters.BurlerRunner; see fakeBurlerRunner's own doc comment.
func (f *fakeBurlerRunner) Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error) {
	f.profiles = append(f.profiles, p)
	f.opts = append(f.opts, opts)
	return burlerengine.Result{}, nil
}

// fakeWebsterRun is a shedadapters.WebsterRunner func value returning a zero
// websterengine.RunResult and a nil error. It is never called: no test in this package invokes a
// producer's Call.
var fakeWebsterRun shedadapters.WebsterRunner = func(websterengine.RunDeps, websterengine.RunOptions) (websterengine.RunResult, error) {
	return websterengine.RunResult{}, nil
}

// fakeMasterStarter, fakeReedOps, fakeShuttleEngine, and fakeRefMatcher satisfy
// websterengine.RunDeps' four required seams by embedding the seam interface in an empty struct,
// which yields a non-nil value satisfying the interface without implementing a single method. Each
// is a placeholder for a non-nil check only: calling any promoted method panics on the embedded nil
// interface, which no test in this package does.
type (
	fakeMasterStarter struct{ websterengine.MasterStarter }
	fakeReedOps       struct{ shuttleengine.ReedOps }
	fakeShuttleEngine struct{ shuttleengine.Engine }
	fakeRefMatcher    struct{ websterengine.RefMatcher }
)

// fakeMergeShuttle implements mergeresolve.Shuttle, satisfying the non-nil Deps.Shuttle
// requirement landingshed.NewPublish/NewFinalize enforce at construction. It is never called: no
// test in this package invokes a producer's Call.
type fakeMergeShuttle struct{}

var _ mergeresolve.Shuttle = fakeMergeShuttle{}

// Run implements mergeresolve.Shuttle; see fakeMergeShuttle's own doc comment.
func (fakeMergeShuttle) Run(shuttleengine.Spec) (shuttleengine.Result, error) {
	return shuttleengine.Result{}, nil
}

// nilFabricOpener is the landingshed pair-opener closure fake filled into both OpenFabric and
// OpenParentFabric. It returns a typed-nil *fabricengine.Fabric and a nil error, which legally
// satisfies NewPublish/NewFinalize -- both construct their resolver from the interface value the
// fabric handle is stored behind, and a nil check on an interface holding a typed-nil pointer still
// passes -- without ever dereferencing the handle at construction time. It is never called: no test
// in this package invokes a producer's Call.
func nilFabricOpener() (*fabricengine.Fabric, error) {
	return nil, nil
}

// testLandingDeps returns a landingshed.Deps with every field the Publish/Finalize constructors
// require filled with a synthetic-but-valid told value derived from dir: told absolute paths, a
// non-nil push-branch closure, a typed-nil fabric-opener closure used for both openers, and a
// minimal merge-resolve shuttle fake -- the same shape testLandingDeps in
// internal/loomrecipe/fixture_test.go, its single surviving sibling copy, uses.
func testLandingDeps(dir string) landingshed.Deps {
	return landingshed.Deps{
		WorktreeRoot:     dir,
		TaskBranch:       "task-branch",
		ParentBranch:     "fixture-parent",
		WebsterDir:       dir,
		StencilsDir:      dir,
		ScratchDir:       filepath.Join(dir, "landing-scratch"),
		OriginURL:        "https://example.invalid/fixture/fixture.git",
		PushBranch:       func() error { return nil },
		OpenFabric:       nilFabricOpener,
		OpenParentFabric: nilFabricOpener,
		Shuttle:          fakeMergeShuttle{},
	}
}

// newTestEnv builds a shedrecipe.Env whose every path field is an absolute path derived from a
// single t.TempDir(), one subdirectory per directory-valued field, and a joined-but-uncreated path
// for each file-valued field, modelled on internal/shedrecipe/fixture_test.go's builder of the same
// name. It fills the four per-producer DiscussionWrite/PlanWrite seams the same way that sibling
// does -- a DiscussionSpec closure returning a Spec over one absolute output path under the same
// temp root, a CommitDiscussion closure returning nil, a PlanSpec closure returning a Spec over one
// absolute output path under the same temp root, and a CommitPlan closure returning nil -- and
// additionally fills Env.Landing via testLandingDeps, because two of the fourteen engines need it,
// which its sibling does not do.
//
// Every seam any registered engine requires non-nil must be filled here:
// TestBuild_EveryRegisteredEngineBuilds drives its assertion off shedrecipe.Names(), so a new
// registry entry with a new required Env seam fails that test until this builder covers it.
//
// No test in this package may reference a path outside its own t.TempDir(): a real repo path would
// mask a told-geometry violation, which is the exact property this package's guard exists to catch.
func newTestEnv(t *testing.T) shedrecipe.Env {
	t.Helper()

	dir := t.TempDir()

	mustMkdir := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		return p
	}

	return shedrecipe.Env{
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
		Landing:    testLandingDeps(mustMkdir("landing")),
	}
}

// writeStencilFile writes content to the on-disk location stencilstore.Path(dir, name) resolves
// name to, creating any parent directory the family convention requires -- stencilstore.RelPath
// splits a hyphenated stencil name on its first hyphen into a family subdirectory, so writing
// straight into a fresh temp directory would otherwise fail with a missing-parent error.
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
