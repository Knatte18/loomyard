// entries_bouncer_test.go covers bouncerEntry: the happy path, the run-directory group (this
// batch's load-bearing assertion, since a missing directory would fail every fresh segment's first
// call), the pinned report-name behaviour, and every construction failure.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// assertErrContains fails the test unless err is non-nil and its message contains want.
func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil; want an error mentioning %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q; want it to mention %q", err.Error(), want)
	}
}

// writeStencil writes content into baseDir at name's stencilstore path, creating parent
// directories as needed.
func writeStencil(t *testing.T, baseDir, name, content string) {
	t.Helper()
	path := stencilstore.Path(baseDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// minimalBouncerConfig returns a Config carrying only the required Bouncer keys, naming a rubric
// stencil this function writes into env.StencilsDir.
func minimalBouncerConfig(t *testing.T, env Env) Config {
	t.Helper()
	writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
	return Config{
		"run_subdir":     "review-segment",
		"artifact_paths": []string{"artifact.md"},
		"rubric_stencil": "bouncer-rubric",
	}
}

func TestBouncerEntry_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	cfg := minimalBouncerConfig(t, env)

	producer, err := bouncerEntry("review-bounce", cfg, env)
	if err != nil {
		t.Fatalf("bouncerEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatal("bouncerEntry() producer = nil; want non-nil")
	}
}

func TestBouncerEntry_RunDirectory(t *testing.T) {
	t.Run("CreatedBeforeAnyCall", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)

		if _, err := bouncerEntry("review-bounce", cfg, env); err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}

		runDir := filepath.Join(env.RunRoot, "review-segment")
		info, err := os.Stat(runDir)
		if err != nil {
			t.Fatalf("os.Stat(%q) error = %v; want the run dir to exist", runDir, err)
		}
		if !info.IsDir() {
			t.Fatalf("os.Stat(%q) IsDir = false; want true", runDir)
		}
	})

	t.Run("DifferentSubdirsResolveToDifferentDirs", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")

		cfgA := Config{"run_subdir": "segment-a", "artifact_paths": []string{"artifact.md"}, "rubric_stencil": "bouncer-rubric"}
		cfgB := Config{"run_subdir": "segment-b", "artifact_paths": []string{"artifact.md"}, "rubric_stencil": "bouncer-rubric"}

		if _, err := bouncerEntry("a", cfgA, env); err != nil {
			t.Fatalf("bouncerEntry(a) error = %v; want nil", err)
		}
		if _, err := bouncerEntry("b", cfgB, env); err != nil {
			t.Fatalf("bouncerEntry(b) error = %v; want nil", err)
		}

		dirA := filepath.Join(env.RunRoot, "segment-a")
		dirB := filepath.Join(env.RunRoot, "segment-b")
		if dirA == dirB {
			t.Fatalf("segment-a and segment-b resolved to the same dir %q", dirA)
		}
		for _, dir := range []string{dirA, dirB} {
			info, err := os.Stat(dir)
			if err != nil {
				t.Fatalf("os.Stat(%q) error = %v; want the run dir to exist", dir, err)
			}
			if !info.IsDir() {
				t.Fatalf("os.Stat(%q) IsDir = false; want true", dir)
			}
		}
	})

	t.Run("OmittedFails", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
		cfg := Config{"artifact_paths": []string{"artifact.md"}, "rubric_stencil": "bouncer-rubric"}

		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})

	t.Run("EmptyStringFails", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
		cfg := Config{"run_subdir": "", "artifact_paths": []string{"artifact.md"}, "rubric_stencil": "bouncer-rubric"}

		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})

	t.Run("AbsoluteFails", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
		cfg := Config{"run_subdir": "/etc/passwd", "artifact_paths": []string{"artifact.md"}, "rubric_stencil": "bouncer-rubric"}

		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})

	t.Run("EscapingFails", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
		cfg := Config{"run_subdir": "../escape", "artifact_paths": []string{"artifact.md"}, "rubric_stencil": "bouncer-rubric"}

		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})
}

// TestBouncerEntry_ArtifactPathsResolveUnderAnchorPath is this batch's load-bearing divergent-roots
// assertion: newTestEnv's Env carries AnchorPath and WorktreeRoot as two distinct temp
// subdirectories, so an artifact_paths entry resolved against the wrong root would land under a
// directory this test can distinguish. shedadapters.BouncerConfig.ArtifactPaths is not exposed on
// the returned shedengine.ShedProducer, so the resolved path is asserted the way this file already
// asserts other BouncerConfig fields it cannot reach directly (TestBouncerEntry_EnvReviewFallback):
// through the seed template's rendered prompt, captured by the fakeShuttle.
func TestBouncerEntry_ArtifactPathsResolveUnderAnchorPath(t *testing.T) {
	env := newTestEnv(t)
	writeStencil(t, env.StencilsDir, "bouncer-template-seed", "{{.artifacts}}\n")
	cfg := minimalBouncerConfig(t, env)

	producer, err := bouncerEntry("review-bounce", cfg, env)
	if err != nil {
		t.Fatalf("bouncerEntry() error = %v; want nil", err)
	}
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	fake := env.Shuttle.(*fakeShuttle)
	if len(fake.specs) != 1 {
		t.Fatalf("len(fake.specs) = %d; want 1 (the seed spawn)", len(fake.specs))
	}
	wantUnderAnchor := filepath.Join(env.AnchorPath, "artifact.md")
	if !strings.Contains(fake.specs[0].Prompt, wantUnderAnchor) {
		t.Errorf("seed prompt = %q; want it to contain %q (artifact.md resolved under Env.AnchorPath)", fake.specs[0].Prompt, wantUnderAnchor)
	}
	notWantUnderWorktree := filepath.Join(env.WorktreeRoot, "artifact.md")
	if strings.Contains(fake.specs[0].Prompt, notWantUnderWorktree) {
		t.Errorf("seed prompt contains %q; want artifact_paths resolved under AnchorPath, not WorktreeRoot", notWantUnderWorktree)
	}
}

// TestBouncerEntry_ReportNamePinning is the batch's second load-bearing assertion: a drift in the
// pinned report name makes shedadapters.ResolveRound return 0 forever, and the Bouncer re-seeds
// every call rather than judging the report already on disk. This test writes round-1-review.md
// directly (bypassing BurlerProducer, which is out of this batch's scope) and asserts Call reaches
// the judge branch -- observable via the recorded shuttle spec's Role, which is "bouncer-judge" only
// past the seed branch and "bouncer-seed" inside it -- rather than the seed branch.
func TestBouncerEntry_ReportNamePinning(t *testing.T) {
	env := newTestEnv(t)
	cfg := minimalBouncerConfig(t, env)
	writeStencil(t, env.StencilsDir, "bouncer-template-seed", "seed template, no markers\n")
	writeStencil(t, env.StencilsDir, "bouncer-template-judge", "judge template, no markers\n")

	producer, err := bouncerEntry("review-bounce", cfg, env)
	if err != nil {
		t.Fatalf("bouncerEntry() error = %v; want nil", err)
	}

	runDir := filepath.Join(env.RunRoot, "review-segment")
	// shedadapters.BurlerProducer writes its round n report to round-<n>-review.md; write that exact
	// name here to simulate a completed round 1 without depending on BurlerProducer itself.
	if err := os.WriteFile(filepath.Join(runDir, "round-1-review.md"), []byte("a review\n"), 0o644); err != nil {
		t.Fatalf("write round-1-review.md: %v", err)
	}

	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	fake := env.Shuttle.(*fakeShuttle)
	if len(fake.specs) != 1 {
		t.Fatalf("len(fake.specs) = %d; want 1 (the judge spawn)", len(fake.specs))
	}
	if fake.specs[0].Role != "bouncer-judge" {
		t.Errorf("fake.specs[0].Role = %q; want %q (Call must reach the judge branch, not re-seed)", fake.specs[0].Role, "bouncer-judge")
	}
}

// TestBouncerEntry_EnvReviewFallback covers the three fallback outcomes for bouncerEntry's
// model/effort/version resolution: a row omitting the keys takes env.ReviewModel/ReviewEffort/
// ReviewVersion; a row setting all three overrides the Env values; both absent leaves all three
// empty (the provider default). shedadapters.BouncerConfig's cfg field is unexported and this is a
// different package, so the resolved triple is asserted through behaviour instead: one Call is
// driven against the entry's producer with the fakeShuttle already on newTestEnv's Env, and the
// recorded shuttleengine.Spec's Model, Effort, and Version are asserted.
func TestBouncerEntry_EnvReviewFallback(t *testing.T) {
	// callAndCaptureSpec constructs a Bouncer entry from cfg and env, drives the seed-pass Call --
	// the first Call on a fresh run directory spawns unconditionally -- and returns the recorded
	// shuttleengine.Spec.
	callAndCaptureSpec := func(t *testing.T, cfg Config, env Env) shuttleengine.Spec {
		t.Helper()
		writeStencil(t, env.StencilsDir, "bouncer-template-seed", "seed template, no markers\n")

		producer, err := bouncerEntry("review-bounce", cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		fake := env.Shuttle.(*fakeShuttle)
		if len(fake.specs) != 1 {
			t.Fatalf("len(fake.specs) = %d; want 1 (the seed spawn)", len(fake.specs))
		}
		return fake.specs[0]
	}

	t.Run("RowOmitsTakesEnvValues", func(t *testing.T) {
		env := newTestEnv(t)
		env.ReviewModel = "env-model"
		env.ReviewEffort = "env-effort"
		env.ReviewVersion = "env-version"
		cfg := minimalBouncerConfig(t, env)

		spec := callAndCaptureSpec(t, cfg, env)
		if spec.Model != "env-model" {
			t.Errorf("spec.Model = %q; want %q", spec.Model, "env-model")
		}
		if spec.Effort != "env-effort" {
			t.Errorf("spec.Effort = %q; want %q", spec.Effort, "env-effort")
		}
		if spec.Version != "env-version" {
			t.Errorf("spec.Version = %q; want %q", spec.Version, "env-version")
		}
	})

	t.Run("RowSetsOverridesEnvValues", func(t *testing.T) {
		env := newTestEnv(t)
		env.ReviewModel = "env-model"
		env.ReviewEffort = "env-effort"
		env.ReviewVersion = "env-version"
		cfg := minimalBouncerConfig(t, env)
		cfg["model"] = "row-model"
		cfg["effort"] = "row-effort"
		cfg["version"] = "row-version"

		spec := callAndCaptureSpec(t, cfg, env)
		if spec.Model != "row-model" {
			t.Errorf("spec.Model = %q; want %q", spec.Model, "row-model")
		}
		if spec.Effort != "row-effort" {
			t.Errorf("spec.Effort = %q; want %q", spec.Effort, "row-effort")
		}
		if spec.Version != "row-version" {
			t.Errorf("spec.Version = %q; want %q", spec.Version, "row-version")
		}
	})

	t.Run("BothAbsentLeavesProviderDefault", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)

		spec := callAndCaptureSpec(t, cfg, env)
		if spec.Model != "" {
			t.Errorf("spec.Model = %q; want \"\"", spec.Model)
		}
		if spec.Effort != "" {
			t.Errorf("spec.Effort = %q; want \"\"", spec.Effort)
		}
		if spec.Version != "" {
			t.Errorf("spec.Version = %q; want \"\"", spec.Version)
		}
	})
}

func TestBouncerEntry_ReportNameKeyRejected(t *testing.T) {
	env := newTestEnv(t)
	cfg := minimalBouncerConfig(t, env)
	cfg["report_name"] = "round-%d-review.md"

	_, err := bouncerEntry("review-bounce", cfg, env)
	assertErrContains(t, err, "report_name")
}

func TestBouncerEntry_ConstructionFailures(t *testing.T) {
	t.Run("MissingArtifactPaths", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
		cfg := Config{"run_subdir": "review-segment", "rubric_stencil": "bouncer-rubric"}
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "artifact_paths")
	})

	t.Run("EmptyArtifactPathsList", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
		cfg := Config{"run_subdir": "review-segment", "artifact_paths": []string{}, "rubric_stencil": "bouncer-rubric"}
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "artifact_paths")
	})

	t.Run("AbsoluteArtifactPathsEntry", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
		cfg := Config{"run_subdir": "review-segment", "artifact_paths": []string{"/etc/passwd"}, "rubric_stencil": "bouncer-rubric"}
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "artifact_paths")
	})

	t.Run("EscapingArtifactPathsEntry", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "bouncer-rubric", "BLOCKING: a bug.\n")
		cfg := Config{"run_subdir": "review-segment", "artifact_paths": []string{"../escape.md"}, "rubric_stencil": "bouncer-rubric"}
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "artifact_paths")
	})

	t.Run("MissingRubricStencil", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"run_subdir": "review-segment", "artifact_paths": []string{"artifact.md"}}
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "rubric_stencil")
	})

	t.Run("RubricStencilDoesNotExist", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"run_subdir": "review-segment", "artifact_paths": []string{"artifact.md"}, "rubric_stencil": "no-such-rubric"}
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "no-such-rubric")
	})

	t.Run("UnrecognisedConfigKey", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		cfg["unexpected"] = "value"
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "unexpected")
	})

	t.Run("BlankEnvRunRoot", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		env.RunRoot = ""
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "RunRoot")
	})

	t.Run("BlankEnvAnchorPath", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		env.AnchorPath = ""
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "AnchorPath")
	})

	t.Run("BlankEnvWorktreeRootStillConstructs", func(t *testing.T) {
		// env.WorktreeRoot is no longer read by bouncerEntry, so a blank value must not prevent a
		// Bouncer row from building.
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		env.WorktreeRoot = ""
		producer, err := bouncerEntry("review-bounce", cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatal("bouncerEntry() producer = nil; want non-nil")
		}
	})

	t.Run("BlankEnvStencilsDir", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		env.StencilsDir = ""
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "StencilsDir")
	})

	t.Run("NilEnvShuttle", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		env.Shuttle = nil
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "Shuttle")
	})

	t.Run("NilEnvNowConstructsSuccessfully", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		env.Now = nil
		producer, err := bouncerEntry("review-bounce", cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatal("bouncerEntry() producer = nil; want non-nil")
		}
	})
}

// layoutBouncerRound1Report writes round 1's report file directly into env.RunRoot/review-segment,
// so a bouncerEntry-built producer's first Call resolves round 1 and reaches the judge branch
// rather than the seed branch -- the same setup TestBouncerEntry_ReportNamePinning uses to reach
// the judge branch without depending on BurlerProducer. The file name mirrors bouncerEntry's own
// pinned ReportName convention.
func layoutBouncerRound1Report(t *testing.T, env Env) {
	t.Helper()
	runDir := filepath.Join(env.RunRoot, "review-segment")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", runDir, err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "round-1-review.md"), []byte("a review\n"), 0o644); err != nil {
		t.Fatalf("write round-1-review.md: %v", err)
	}
}

// judgeSeamFakeShuttle implements shedadapters.Shuttle by writing round 1's APPROVED verdict and
// ledger to the spec's declared OutputFiles during Run, so a bouncerEntry-built producer's judge
// call harvests and settles within the same Call that produced them -- the harvest vehicle this
// file's commit-seam subtests drive, following shedadapters/bouncer_commit_test.go's own treatment
// of the same removed APPROVED-replay vehicle.
type judgeSeamFakeShuttle struct {
	specs []shuttleengine.Spec
}

var _ shedadapters.Shuttle = (*judgeSeamFakeShuttle)(nil)

// Run implements shedadapters.Shuttle: it records spec, writes round 1's verdict and ledger to the
// judge call's first two declared OutputFiles, and reports shuttleengine.OutcomeDone.
func (f *judgeSeamFakeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.specs = append(f.specs, spec)
	if len(spec.OutputFiles) == 3 {
		verdict := "---\nverdict: APPROVED\nrationale: \"because reasons\"\n---\n"
		_ = os.WriteFile(spec.OutputFiles[0], []byte(verdict), 0o644)
		ledger := "---\nround: 1\nledger: []\n---\nno open findings\n"
		_ = os.WriteFile(spec.OutputFiles[1], []byte(ledger), 0o644)
	}
	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
}

// Attach implements shedadapters.Shuttle's probe method by always reporting not-found, matching
// fakeShuttle's own Attach in fixture_test.go.
func (f *judgeSeamFakeShuttle) Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	return shuttleengine.Result{}, false, nil
}

// TestBouncerEntry_CommitSeam covers the two-value resolution of commit_seam, the presence guard
// requireSeam enforces on a configured-but-missing Env closure, and the recognised-set edit to
// configRejectUnknown.
func TestBouncerEntry_CommitSeam(t *testing.T) {
	t.Run("PlanResolvesToCommitPlan", func(t *testing.T) {
		// This subtest also demonstrates commit_seam is accepted by configRejectUnknown rather
		// than rejected as unknown: bouncerEntry returning a nil error below means the key was
		// recognised.
		env := newTestEnv(t)
		env.Shuttle = &judgeSeamFakeShuttle{}
		writeStencil(t, env.StencilsDir, "bouncer-template-judge", "judge template, no markers\n")
		planCalls := 0
		env.CommitPlan = func() error { planCalls++; return nil }
		cfg := minimalBouncerConfig(t, env)
		cfg["commit_seam"] = "plan"
		layoutBouncerRound1Report(t, env)

		producer, err := bouncerEntry("review-bounce", cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if planCalls != 1 {
			t.Errorf("CommitPlan call count = %d; want 1", planCalls)
		}
	})

	t.Run("DiscussionResolvesToCommitDiscussion", func(t *testing.T) {
		env := newTestEnv(t)
		env.Shuttle = &judgeSeamFakeShuttle{}
		writeStencil(t, env.StencilsDir, "bouncer-template-judge", "judge template, no markers\n")
		planCalls := 0
		discussionCalls := 0
		env.CommitPlan = func() error { planCalls++; return nil }
		env.CommitDiscussion = func() error { discussionCalls++; return nil }
		cfg := minimalBouncerConfig(t, env)
		cfg["commit_seam"] = "discussion"
		layoutBouncerRound1Report(t, env)

		producer, err := bouncerEntry("review-bounce", cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if discussionCalls != 1 {
			t.Errorf("CommitDiscussion call count = %d; want 1", discussionCalls)
		}
		if planCalls != 0 {
			t.Errorf("CommitPlan call count = %d; want 0 (the two seams must not be interchangeable)", planCalls)
		}
	})

	t.Run("AbsentLeavesSeamNil", func(t *testing.T) {
		env := newTestEnv(t)
		env.Shuttle = &judgeSeamFakeShuttle{}
		writeStencil(t, env.StencilsDir, "bouncer-template-judge", "judge template, no markers\n")
		planCalls := 0
		discussionCalls := 0
		env.CommitPlan = func() error { planCalls++; return nil }
		env.CommitDiscussion = func() error { discussionCalls++; return nil }
		cfg := minimalBouncerConfig(t, env)
		layoutBouncerRound1Report(t, env)

		producer, err := bouncerEntry("review-bounce", cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if planCalls != 0 {
			t.Errorf("CommitPlan call count = %d; want 0", planCalls)
		}
		if discussionCalls != 0 {
			t.Errorf("CommitDiscussion call count = %d; want 0", discussionCalls)
		}
	})

	t.Run("UnrecognisedValueIsConstructionError", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		cfg["commit_seam"] = "bogus"

		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "commit_seam")
	})

	t.Run("PresentButMissingEnvClosureIsConstructionError", func(t *testing.T) {
		t.Run("Plan", func(t *testing.T) {
			env := newTestEnv(t)
			env.CommitPlan = nil
			cfg := minimalBouncerConfig(t, env)
			cfg["commit_seam"] = "plan"

			_, err := bouncerEntry("review-bounce", cfg, env)
			assertErrContains(t, err, "CommitPlan")
		})

		t.Run("Discussion", func(t *testing.T) {
			env := newTestEnv(t)
			env.CommitDiscussion = nil
			cfg := minimalBouncerConfig(t, env)
			cfg["commit_seam"] = "discussion"

			_, err := bouncerEntry("review-bounce", cfg, env)
			assertErrContains(t, err, "CommitDiscussion")
		})
	})

	t.Run("AbsentWithBothEnvClosuresNilConstructsSuccessfully", func(t *testing.T) {
		env := newTestEnv(t)
		env.CommitPlan = nil
		env.CommitDiscussion = nil
		cfg := minimalBouncerConfig(t, env)

		producer, err := bouncerEntry("review-bounce", cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil (the guard is on the key's presence, not on the Env field)", err)
		}
		if producer == nil {
			t.Fatal("bouncerEntry() producer = nil; want non-nil")
		}
	})
}
