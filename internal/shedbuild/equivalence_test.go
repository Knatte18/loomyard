// equivalence_test.go is the checkable form of the claim that this package's recipe format can
// express loom's real, current producer list: it builds internal/shedbuild/testdata/loom-recipe.yaml
// through Parse and Build, builds loom's own list through loomshed.New from a paired shedrecipe.Env
// and loomshed.Deps sharing one temp directory, and asserts the two thirteen-row lists agree field
// by field and type by type. It fails loudly if a future change to loom's own list outgrows the
// format -- the regression worth catching, since the conversion item sequenced after this task
// depends on this fixture staying in sync.

package shedbuild

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// equivalencePair is one t.TempDir()-derived Env/Deps pair, told the same values on both sides so
// every path either side reads is literally the same string.
type equivalencePair struct {
	env  shedrecipe.Env
	deps loomshed.Deps
}

// newEquivalencePair builds an equivalencePair from a single t.TempDir(), deliberately not reusing
// newTestEnv: that helper derives its own temp directory with no loom-side counterpart, which is
// exactly what this test must avoid -- both sides must be told the same values from one directory.
func newEquivalencePair(t *testing.T) equivalencePair {
	t.Helper()

	dir := t.TempDir()

	mustMkdir := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		return p
	}

	cwd := mustMkdir("cwd")
	anchorPath := mustMkdir("anchor")
	worktreeRoot := mustMkdir("worktree")
	stencilsDir := mustMkdir("stencils")
	runRoot := mustMkdir("run-root")
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	decisionRecordPath := filepath.Join(dir, "decision-record.md")
	supportLogPath := filepath.Join(dir, "support-log.md")

	landing := testLandingDeps(mustMkdir("landing"))

	websterRun := fakeWebsterRun
	websterDeps := websterengine.RunDeps{
		Starter:    fakeMasterStarter{},
		Reed:       fakeReedOps{},
		Engine:     fakeShuttleEngine{},
		RefMatcher: fakeRefMatcher{},
	}

	env := shedrecipe.Env{
		Cwd:                cwd,
		AnchorPath:         anchorPath,
		WorktreeRoot:       worktreeRoot,
		StatusPath:         statusPath,
		StatusLockPath:     statusLockPath,
		StencilsDir:        stencilsDir,
		RunRoot:            runRoot,
		DecisionRecordPath: decisionRecordPath,
		SupportLogPath:     supportLogPath,
		Shuttle:            &fakeShuttle{},
		Burler:             &fakeBurlerRunner{},
		WebsterRun:         websterRun,
		WebsterDeps:        websterDeps,
		Landing:            landing,
	}

	deps := loomshed.Deps{
		StatusPath:         statusPath,
		LockPath:           filepath.Join(dir, "run.lock"),
		StatusLockPath:     statusLockPath,
		AnchorPath:         anchorPath,
		WorktreeRoot:       worktreeRoot,
		DecisionRecordPath: decisionRecordPath,
		SupportLogPath:     supportLogPath,
		Preflight:          preflightshed.NewPreflight(loomshed.NamePreflight, cwd),
		WebsterRun:         websterRun,
		WebsterDeps:        websterDeps,
		Landing:            landing,
	}

	return equivalencePair{env: env, deps: deps}
}

// TestLoomEquivalence_ThirteenRowsMatch asserts that building
// internal/shedbuild/testdata/loom-recipe.yaml through Parse and Build produces the same
// thirteen-row producer list, field by field and type by type, as loomshed.New produces from a
// paired loomshed.Deps -- and that Check reports no findings against the built list.
func TestLoomEquivalence_ThirteenRowsMatch(t *testing.T) {
	pair := newEquivalencePair(t)

	data, err := os.ReadFile(filepath.Join("testdata", "loom-recipe.yaml"))
	if err != nil {
		t.Fatalf("os.ReadFile(loom-recipe.yaml) error = %v, want nil", err)
	}

	recipe, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	built, err := Build(recipe, pair.env)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	shed, err := loomshed.New(pair.deps)
	if err != nil {
		t.Fatalf("loomshed.New() error = %v, want nil", err)
	}
	want := shed.Producers

	if len(built) != 13 || len(want) != len(built) {
		t.Fatalf("row counts: Build() = %d, loomshed.New() = %d, want both = 13", len(built), len(want))
	}

	for i := range built {
		got := built[i]
		w := want[i]

		if got.Name != w.Name {
			t.Errorf("row %d: Name = %q, want %q", i, got.Name, w.Name)
		}
		if got.OnDone != w.OnDone {
			t.Errorf("row %d %q: OnDone = %q, want %q", i, got.Name, got.OnDone, w.OnDone)
		}
		if got.OnStuck != w.OnStuck {
			t.Errorf("row %d %q: OnStuck = %q, want %q", i, got.Name, got.OnStuck, w.OnStuck)
		}
		if got.Segment != w.Segment {
			t.Errorf("row %d %q: Segment = %q, want %q", i, got.Name, got.Segment, w.Segment)
		}
		if got.MaxBounces != w.MaxBounces {
			t.Errorf("row %d %q: MaxBounces = %d, want %d", i, got.Name, got.MaxBounces, w.MaxBounces)
		}

		gotType := reflect.TypeOf(got.Producer)
		wantType := reflect.TypeOf(w.Producer)
		if gotType != wantType {
			t.Errorf("row %d %q: Producer concrete type = %v, want %v", i, got.Name, gotType, wantType)
		}
	}

	if findings := Check(recipe, built); len(findings) != 0 {
		t.Errorf("Check() = %v, want no findings", findings)
	}
}
