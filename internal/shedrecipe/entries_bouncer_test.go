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

	t.Run("BlankEnvWorktreeRoot", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		env.WorktreeRoot = ""
		_, err := bouncerEntry("review-bounce", cfg, env)
		assertErrContains(t, err, "WorktreeRoot")
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
