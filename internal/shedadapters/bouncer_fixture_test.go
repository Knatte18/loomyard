package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// bouncerFixtureStampHash is the fake but well-formed 64-lowercase-hex sha256 the fixture stamps every stencil file with, via stencilstore.ApplyStamp.
const bouncerFixtureStampHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

const (
	bouncerFixtureRubricName  = "bouncer-template-rubric"
	bouncerFixtureRubricBody  = "# Rubric\n\nBe thorough and cite evidence.\n"
	bouncerFixtureBareRubric  = "# Rubric\n\nBe thorough.\n"
	bouncerFixtureSpecsRubric = "# Rubric\n\nCite {{.specs_dir}}.\n\nBe thorough and cite evidence.\n"
)

// bouncerFixture is a BouncerConfig over a fresh run dir and a stencils directory, with Build constructing the *Bouncer on request.
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

// withBareConfig leaves Model, Effort, Version and Now unset, writes the bare rubric body, and defaults the shuttle to an empty shedfake.Shuttle.
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
		cfg.Model = "claude-x"
		cfg.Effort = "high"
		cfg.Version = "v1"
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

// writeBouncerStencils writes each named stencil body on top of the seeded stencils in dir, at the path stencilstore.RelPath derives.
// ApplyStamp merges into an existing leading comment rather than nesting a second one,
// so a body already carrying its own leading comment still ends up with exactly one banner for stencil.Fill's leading-comment strip to remove.
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

type burlerProducerSpec struct {
	profile burlerengine.Profile
	opts    burlerengine.RunOpts
	attach  Shuttle
	now     func() time.Time
}

type burlerProducerOpt func(*burlerProducerSpec)

func withBurlerProfile(profile burlerengine.Profile) burlerProducerOpt {
	return func(s *burlerProducerSpec) { s.profile = profile }
}

func withBurlerRunOpts(opts burlerengine.RunOpts) burlerProducerOpt {
	return func(s *burlerProducerSpec) { s.opts = opts }
}

// withAttach supplies the attach seam for the cases that script what the live-round probe finds.
func withAttach(attach Shuttle) burlerProducerOpt {
	return func(s *burlerProducerSpec) { s.attach = attach }
}

func withBurlerClock(now func() time.Time) burlerProducerOpt {
	return func(s *burlerProducerSpec) { s.now = now }
}

// newBurlerProducer builds a BurlerProducer over runDir with runner, failing the test on constructor error.
// The attach seam defaults to a shedfake.Shuttle finding nothing live, the ordinary no-live-round condition.
func newBurlerProducer(t *testing.T, runDir string, runner *shedfake.BurlerRunner, opts ...burlerProducerOpt) *BurlerProducer {
	t.Helper()

	spec := burlerProducerSpec{
		profile: simpleBurlerProfile(),
		attach:  &shedfake.Shuttle{},
	}
	for _, opt := range opts {
		opt(&spec)
	}
	p, err := NewBurlerProducer("burler", runner, spec.attach, spec.profile, spec.opts, runDir, spec.now)
	if err != nil {
		t.Fatalf("NewBurlerProducer() error = %v; want nil", err)
	}
	return p
}
