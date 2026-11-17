package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// bouncerFixtureStampHash is a fake but well-formed sha256 hash of 64 lowercase hex digits.
// The fixture stamps every stencil file with it through stencilstore.ApplyStamp.
const bouncerFixtureStampHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

const (
	bouncerFixtureRubricName  = "bouncer-template-rubric"
	bouncerFixtureRubricBody  = "# Rubric\n\nBe thorough and cite evidence.\n"
	bouncerFixtureBareRubric  = "# Rubric\n\nBe thorough.\n"
	bouncerFixtureSpecsRubric = "# Rubric\n\nCite {{.specs_dir}}.\n\nBe thorough and cite evidence.\n"
)

// bouncerFixture pairs a BouncerConfig over a fresh run dir and a stencils directory.
// Build constructs the *Bouncer from that config on request.
type bouncerFixture struct {
	t           *testing.T
	Config      BouncerConfig
	StencilsDir string
}

type bouncerFixtureSpec struct {
	rubricName string
	rubricBody string
	overrides  map[string]string
	absent     []string
	specsDir   string
	shuttle    Shuttle
	clock      func() time.Time
	nestedRun  bool
	bare       bool
	escalation bool
}

type bouncerFixtureOpt func(*bouncerFixtureSpec)

// withRubric names the rubric stencil and sets its body.
func withRubric(name, body string) bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) {
		s.rubricName = name
		s.rubricBody = body
	}
}

// withRubricName names the rubric stencil and keeps the default body.
func withRubricName(name string) bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) { s.rubricName = name }
}

// withSpecsMarker seeds the rubric with a literal {{.specs_dir}} marker and sets cfg.SpecsDir.
func withSpecsMarker(specsDir string) bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) {
		s.rubricBody = bouncerFixtureSpecsRubric
		s.specsDir = specsDir
	}
}

// withStencilOverrides writes each named stencil body on top of the seeded stencils, the rubric included.
func withStencilOverrides(files map[string]string) bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) { s.overrides = files }
}

// withoutStencils removes the named stencils after seeding, for the tests that need one absent.
func withoutStencils(names ...string) bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) { s.absent = names }
}

func withShuttle(shuttle Shuttle) bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) { s.shuttle = shuttle }
}

func withClock(now func() time.Time) bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) { s.clock = now }
}

// withNestedRunDir puts RunDir in a subdirectory of the temp dir and the artifact beside it,
// so an archived sibling of RunDir lands inside the temp tree.
func withNestedRunDir() bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) { s.nestedRun = true }
}

// withEscalationInputs sets the slug, worktree root and decision record path that the escalation brief render needs.
// It makes an escalation render its brief and notice.
func withEscalationInputs() bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) { s.escalation = true }
}

// withBareConfig leaves Models and Now unset and writes the bare rubric body.
// It defaults the shuttle to an empty shedfake.Shuttle.
func withBareConfig() bouncerFixtureOpt {
	return func(s *bouncerFixtureSpec) { s.bare = true }
}

func newBouncerFixture(t *testing.T, opts ...bouncerFixtureOpt) *bouncerFixture {
	t.Helper()

	spec := bouncerFixtureSpec{rubricName: bouncerFixtureRubricName}
	for _, opt := range opts {
		opt(&spec)
	}

	if spec.rubricBody == "" {
		spec.rubricBody = bouncerFixtureRubricBody
		if spec.bare {
			spec.rubricBody = bouncerFixtureBareRubric
		}
	}
	stencilsDir := stencilkit.Seed(t)
	files := map[string]string{spec.rubricName: spec.rubricBody}
	for name, body := range spec.overrides {
		files[name] = body
	}
	writeBouncerStencils(t, stencilsDir, files)
	stencilkit.Remove(t, stencilsDir, spec.absent...)

	runDir := t.TempDir()
	artifactDir := runDir
	if spec.nestedRun {
		runDir = filepath.Join(artifactDir, "run")
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(runDir) = %v; want nil", err)
		}
	}

	cfg := BouncerConfig{
		Name:          "gate",
		RunDir:        runDir,
		ArtifactPaths: []string{filepath.Join(artifactDir, "artifact.md")},
		ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
		StencilsDir:   stencilsDir,
		RubricStencil: spec.rubricName,
		SpecsDir:      spec.specsDir,
		Shuttle:       spec.shuttle,

		CirclingCheckpoint: 1,
	}
	if spec.escalation {
		cfg.Slug = "my-task"
		cfg.WorktreeRoot = filepath.Join(artifactDir, "worktree")
		cfg.DecisionRecordPath = filepath.Join(artifactDir, "decision-record.md")
	}
	switch {
	case spec.clock != nil:
		cfg.Now = spec.clock
	case !spec.bare:
		cfg.Now = fixedClock(bouncerJudgeTestClock)
	}
	if spec.bare {
		if cfg.Shuttle == nil {
			cfg.Shuttle = &shedfake.Shuttle{}
		}
	} else {
		cfg.Models = BouncerModels{
			Seed:  shuttleengine.ModelChoice{Engine: "s", Model: "claude-seed", Effort: "low", Version: "v1"},
			Judge: shuttleengine.ModelChoice{Engine: "j", Model: "claude-judge", Effort: "high", Version: "v2"},
		}
	}
	return &bouncerFixture{t: t, Config: cfg, StencilsDir: cfg.StencilsDir}
}

// Build constructs the *Bouncer over the fixture's config.
func (fx *bouncerFixture) Build() (*Bouncer, BouncerConfig) {
	fx.t.Helper()

	b, err := NewBouncer(fx.Config)
	if err != nil {
		fx.t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	return b, fx.Config
}

// writeBouncerStencils writes each named stencil body on top of the seeded stencils in dir.
// Each body lands at the path stencilstore.RelPath returns.
// ApplyStamp merges into an existing leading comment instead of nesting a second one,
// so a body with its own leading comment still ends with one banner, which stencil.Fill strips.
func writeBouncerStencils(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, body := range files {
		absPath := filepath.Join(dir, filepath.FromSlash(stencilstore.RelPath(name)))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(absPath), err)
		}
		content := stencilstore.ApplyStamp([]byte(body), bouncerFixtureStampHash)
		if err := os.WriteFile(absPath, content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", absPath, err)
		}
	}
}

// carryOverRecorder is a fake of the CarryOver seam.
// It records each entry it is called with and appends "carry-over" to log when one is set.
// It calls onCall first and fails with err when set.
type carryOverRecorder struct {
	log     *seamLog
	entries []discussionparser.CarryOver
	err     error
	onCall  func()
}

// install sets the three carry-over fields on cfg, anchoring entry paths at the parent of RunDir.
func (r *carryOverRecorder) install(cfg *BouncerConfig) {
	cfg.Segment = "Plan-Review"
	cfg.AnchorPath = filepath.Dir(cfg.RunDir)
	cfg.CarryOver = func(entry discussionparser.CarryOver) error {
		if r.onCall != nil {
			r.onCall()
		}
		if r.log != nil {
			r.log.calls = append(r.log.calls, "carry-over")
		}
		r.entries = append(r.entries, entry)
		return r.err
	}
}
