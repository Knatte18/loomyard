// entries_burler_test.go covers burlerRoundEntry: the happy path, the profile-to-Profile mapping
// (including the target/fasit relative-path exception), strict unknown-key rejection at all three
// levels, the run-directory group shared with entries_bouncer_test.go's Bouncer coverage, and every
// construction failure.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// minimalBurlerConfig returns a Config carrying only the required BurlerRound keys: a run_subdir
// and a profile map with a single rubric key, non-empty so it does not count as absent.
func minimalBurlerConfig() Config {
	return Config{
		"run_subdir": "review-segment",
		"profile":    map[string]any{"rubric": "a rubric"},
	}
}

func TestBurlerRoundEntry_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	cfg := minimalBurlerConfig()

	producer, err := burlerRoundEntry("review-round", cfg, env)
	if err != nil {
		t.Fatalf("burlerRoundEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatal("burlerRoundEntry() producer = nil; want non-nil")
	}
}

// callAndCaptureProfile constructs a BurlerRound entry from cfg and env, drives one Call so
// env.Burler's fakeBurlerRunner records the Profile and RunOpts it was handed, and returns them.
// Call's own return values are ignored: fakeBurlerRunner always returns a zero burlerengine.Result
// with a nil error, which BurlerProducer.Call's own outcome switch does not recognise, so Call
// itself returns a non-nil error on every path here -- that error is not this helper's concern, only
// what the runner recorded is.
func callAndCaptureProfile(t *testing.T, name string, cfg Config, env Env) (burlerengine.Profile, burlerengine.RunOpts) {
	t.Helper()
	producer, err := burlerRoundEntry(name, cfg, env)
	if err != nil {
		t.Fatalf("burlerRoundEntry() error = %v; want nil", err)
	}

	_, _, _ = producer.Call(context.Background())

	fake := env.Burler.(*fakeBurlerRunner)
	if len(fake.profiles) != 1 || len(fake.opts) != 1 {
		t.Fatalf("fakeBurlerRunner recorded %d profiles and %d opts; want 1 and 1", len(fake.profiles), len(fake.opts))
	}
	return fake.profiles[0], fake.opts[0]
}

func TestBurlerRoundEntry_ProfileMapping(t *testing.T) {
	env := newTestEnv(t)
	cfg := Config{
		"run_subdir": "review-segment",
		"profile": map[string]any{
			"target":      map[string]any{"paths": []string{"target.md"}, "instructions": "review this"},
			"fasit":       map[string]any{"paths": []string{"fasit.md"}, "instructions": "against this"},
			"rubric":      "BLOCKING: a bug.",
			"fix-scope":   "source",
			"tool-use":    true,
			"cluster-fan": "",
		},
		"model":     "opus",
		"effort":    "high",
		"timeout_s": 30,
	}

	profile, opts := callAndCaptureProfile(t, "review-round", cfg, env)

	if len(profile.Target.Paths) != 1 || profile.Target.Paths[0] != "target.md" {
		t.Errorf("profile.Target.Paths = %v; want [\"target.md\"]", profile.Target.Paths)
	}
	if profile.Target.Instructions != "review this" {
		t.Errorf("profile.Target.Instructions = %q; want %q", profile.Target.Instructions, "review this")
	}
	if len(profile.Fasit.Paths) != 1 || profile.Fasit.Paths[0] != "fasit.md" {
		t.Errorf("profile.Fasit.Paths = %v; want [\"fasit.md\"]", profile.Fasit.Paths)
	}
	if profile.Fasit.Instructions != "against this" {
		t.Errorf("profile.Fasit.Instructions = %q; want %q", profile.Fasit.Instructions, "against this")
	}
	if profile.Rubric != "BLOCKING: a bug." {
		t.Errorf("profile.Rubric = %q; want %q", profile.Rubric, "BLOCKING: a bug.")
	}
	if profile.FixScope != burlerengine.FixScopeSource {
		t.Errorf("profile.FixScope = %q; want %q", profile.FixScope, burlerengine.FixScopeSource)
	}
	if !profile.ToolUse {
		t.Error("profile.ToolUse = false; want true")
	}
	if profile.ClusterFan != "" {
		t.Errorf("profile.ClusterFan = %q; want \"\"", profile.ClusterFan)
	}

	if opts.Model != "opus" {
		t.Errorf("opts.Model = %q; want %q", opts.Model, "opus")
	}
	if opts.Effort != "high" {
		t.Errorf("opts.Effort = %q; want %q", opts.Effort, "high")
	}
	if opts.Timeout != 30*time.Second {
		t.Errorf("opts.Timeout = %v; want %v", opts.Timeout, 30*time.Second)
	}
}

// TestBurlerRoundEntry_RubricStencil covers the profile.rubric_stencil key: resolving a seeded
// stencil's content into Profile.Rubric, stripping a leading stamp banner from it, the
// rubric/rubric_stencil mutual-exclusivity rule, an unseeded stencil name, and the
// empty-Env.StencilsDir construction error being scoped to the rubric_stencil path only.
func TestBurlerRoundEntry_RubricStencil(t *testing.T) {
	t.Run("ResolvesSeededStencilContent", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "round-rubric", "BLOCKING: a round bug.\n")
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric_stencil": "round-rubric"},
		}

		profile, _ := callAndCaptureProfile(t, "review-round", cfg, env)
		if profile.Rubric != "BLOCKING: a round bug.\n" {
			t.Errorf("profile.Rubric = %q; want %q", profile.Rubric, "BLOCKING: a round bug.\n")
		}
	})

	t.Run("StripsLeadingStampBanner", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "round-rubric", "<!-- lyx-stencil: sha256=aaaa -->\nBLOCKING: a round bug.\n")
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric_stencil": "round-rubric"},
		}

		profile, _ := callAndCaptureProfile(t, "review-round", cfg, env)
		if profile.Rubric != "BLOCKING: a round bug.\n" {
			t.Errorf("profile.Rubric = %q; want the banner stripped, got the raw stencil bytes", profile.Rubric)
		}
	})

	t.Run("BothRubricAndRubricStencilFails", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "round-rubric", "a rubric\n")
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric": "a literal rubric", "rubric_stencil": "round-rubric"},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "rubric")
		assertErrContains(t, err, "rubric_stencil")
	})

	t.Run("NeitherRubricNorRubricStencilFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"fix-scope": "source"},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "rubric")
		assertErrContains(t, err, "rubric_stencil")
	})

	t.Run("UnseededRubricStencilFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric_stencil": "no-such-round-rubric"},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "no-such-round-rubric")
	})

	t.Run("EmptyStencilsDirFailsOnRubricStencilPathOnly", func(t *testing.T) {
		env := newTestEnv(t)
		env.StencilsDir = ""
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric_stencil": "round-rubric"},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "StencilsDir")
	})

	t.Run("EmptyStencilsDirConstructsCleanlyWithLiteralRubric", func(t *testing.T) {
		env := newTestEnv(t)
		env.StencilsDir = ""
		cfg := minimalBurlerConfig()

		producer, err := burlerRoundEntry("review-round", cfg, env)
		if err != nil {
			t.Fatalf("burlerRoundEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatal("burlerRoundEntry() producer = nil; want non-nil")
		}
	})

	// A nil Shuttle is refused rather than tolerated: it is the round's live-agent probe, and a
	// producer built without it respawns over a still-live round -- two agents writing one review,
	// and on a fix-scope: source row, two agents committing to one branch. A wiring slip must fail
	// here, at construction, not silently at the next crash.
	t.Run("NilShuttleSeamIsRefused", func(t *testing.T) {
		env := newTestEnv(t)
		env.Shuttle = nil
		cfg := minimalBurlerConfig()

		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "Shuttle")
	})
}

// TestBurlerRoundEntry_EnvReviewFallback covers the three fallback outcomes for
// burlerRoundEntry's model/effort/timeout_s resolution: a row omitting the keys takes
// env.ReviewModel/ReviewEffort/ReviewTimeout; a row setting all three overrides the Env values;
// both absent with an empty Env leaves the zero values.
func TestBurlerRoundEntry_EnvReviewFallback(t *testing.T) {
	t.Run("RowOmitsTakesEnvValues", func(t *testing.T) {
		env := newTestEnv(t)
		env.ReviewModel = "env-model"
		env.ReviewEffort = "env-effort"
		env.ReviewTimeout = 45 * time.Second
		cfg := minimalBurlerConfig()

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if opts.Model != "env-model" {
			t.Errorf("opts.Model = %q; want %q", opts.Model, "env-model")
		}
		if opts.Effort != "env-effort" {
			t.Errorf("opts.Effort = %q; want %q", opts.Effort, "env-effort")
		}
		if opts.Timeout != 45*time.Second {
			t.Errorf("opts.Timeout = %v; want %v", opts.Timeout, 45*time.Second)
		}
	})

	t.Run("RowSetsOverridesEnvValues", func(t *testing.T) {
		env := newTestEnv(t)
		env.ReviewModel = "env-model"
		env.ReviewEffort = "env-effort"
		env.ReviewTimeout = 45 * time.Second
		cfg := minimalBurlerConfig()
		cfg["model"] = "row-model"
		cfg["effort"] = "row-effort"
		cfg["timeout_s"] = 30

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if opts.Model != "row-model" {
			t.Errorf("opts.Model = %q; want %q", opts.Model, "row-model")
		}
		if opts.Effort != "row-effort" {
			t.Errorf("opts.Effort = %q; want %q", opts.Effort, "row-effort")
		}
		if opts.Timeout != 30*time.Second {
			t.Errorf("opts.Timeout = %v; want %v", opts.Timeout, 30*time.Second)
		}
	})

	t.Run("BothAbsentLeavesZeroValues", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if opts.Model != "" {
			t.Errorf("opts.Model = %q; want \"\"", opts.Model)
		}
		if opts.Effort != "" {
			t.Errorf("opts.Effort = %q; want \"\"", opts.Effort)
		}
		if opts.Timeout != 0 {
			t.Errorf("opts.Timeout = %v; want 0", opts.Timeout)
		}
	})
}

func TestBurlerRoundEntry_FractionalTimeoutRejected(t *testing.T) {
	env := newTestEnv(t)
	cfg := minimalBurlerConfig()
	cfg["timeout_s"] = 30.5

	_, err := burlerRoundEntry("review-round", cfg, env)
	assertErrContains(t, err, "timeout_s")
}

func TestBurlerRoundEntry_RelativePathException(t *testing.T) {
	env := newTestEnv(t)
	cfg := Config{
		"run_subdir": "review-segment",
		"profile": map[string]any{
			"target": map[string]any{"paths": []string{"relative/target.md"}},
			"fasit":  map[string]any{"paths": []string{"/absolute/fasit.md"}},
			"rubric": "a rubric",
		},
	}

	profile, _ := callAndCaptureProfile(t, "review-round", cfg, env)

	if len(profile.Target.Paths) != 1 || profile.Target.Paths[0] != "relative/target.md" {
		t.Errorf("profile.Target.Paths = %v; want [\"relative/target.md\"] unchanged", profile.Target.Paths)
	}
	if len(profile.Fasit.Paths) != 1 || profile.Fasit.Paths[0] != "/absolute/fasit.md" {
		t.Errorf("profile.Fasit.Paths = %v; want [\"/absolute/fasit.md\"] unchanged -- an absolute value here is not rejected", profile.Fasit.Paths)
	}
}

func TestBurlerRoundEntry_StrictUnknownKeys(t *testing.T) {
	t.Run("OuterConfigKey", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()
		cfg["unexpected"] = "value"
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "unexpected")
	})

	t.Run("ProfileKey", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric": "a rubric", "unexpected": "value"},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "unexpected")
	})

	for _, key := range []string{"review-path", "fixer-report-path", "prior-reviews", "prior-fixer-reports", "cluster-exclude"} {
		t.Run("ProfileKey_"+key, func(t *testing.T) {
			env := newTestEnv(t)
			cfg := Config{
				"run_subdir": "review-segment",
				"profile":    map[string]any{"rubric": "a rubric", key: "value"},
			}
			_, err := burlerRoundEntry("review-round", cfg, env)
			assertErrContains(t, err, key)
		})
	}

	t.Run("TargetInnerKey", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile": map[string]any{
				"rubric": "a rubric",
				"target": map[string]any{"paths": []string{"t.md"}, "unexpected": "value"},
			},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "unexpected")
	})

	t.Run("FasitInnerKey", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile": map[string]any{
				"rubric": "a rubric",
				"fasit":  map[string]any{"paths": []string{"f.md"}, "unexpected": "value"},
			},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "unexpected")
	})
}

func TestBurlerRoundEntry_RunDirectory(t *testing.T) {
	t.Run("CreatedBeforeAnyCall", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()

		if _, err := burlerRoundEntry("review-round", cfg, env); err != nil {
			t.Fatalf("burlerRoundEntry() error = %v; want nil", err)
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

	t.Run("SameSubdirAsBouncerResolvesToSameDir", func(t *testing.T) {
		env := newTestEnv(t)
		burlerCfg := minimalBurlerConfig()
		bouncerCfg := minimalBouncerConfig(t, env)
		bouncerCfg["run_subdir"] = "review-segment"

		if _, err := burlerRoundEntry("review-round", burlerCfg, env); err != nil {
			t.Fatalf("burlerRoundEntry() error = %v; want nil", err)
		}
		if _, err := bouncerEntry("review-bounce", bouncerCfg, env); err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}

		wantDir := filepath.Join(env.RunRoot, "review-segment")
		if _, err := os.Stat(wantDir); err != nil {
			t.Fatalf("os.Stat(%q) error = %v; want the shared run dir to exist", wantDir, err)
		}
	})

	t.Run("OmittedFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"profile": map[string]any{"rubric": "a rubric"}}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})

	t.Run("EmptyStringFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"run_subdir": "", "profile": map[string]any{"rubric": "a rubric"}}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})

	t.Run("AbsoluteFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"run_subdir": "/etc/passwd", "profile": map[string]any{"rubric": "a rubric"}}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})

	t.Run("EscapingFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"run_subdir": "../escape", "profile": map[string]any{"rubric": "a rubric"}}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})
}

func TestBurlerRoundEntry_ConstructionFailures(t *testing.T) {
	t.Run("MissingProfile", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"run_subdir": "review-segment"}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "profile")
	})

	t.Run("BlankEnvRunRoot", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()
		env.RunRoot = ""
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "RunRoot")
	})

	t.Run("NilEnvBurler", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()
		env.Burler = nil
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "Burler")
	})

	t.Run("NilEnvNowConstructsSuccessfully", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()
		env.Now = nil
		producer, err := burlerRoundEntry("review-round", cfg, env)
		if err != nil {
			t.Fatalf("burlerRoundEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatal("burlerRoundEntry() producer = nil; want non-nil")
		}
	})
}

// TestBurlerRoundEntry_FileSetErrorsNameTheirOwnMap pins the entry/field qualification on
// profile.target and profile.fasit errors. The two maps carry identical key sets, so without it a
// recipe author with a typo in one of them is told which key and never which map -- and the two
// error strings are byte-identical.
func TestBurlerRoundEntry_FileSetErrorsNameTheirOwnMap(t *testing.T) {
	tests := []struct {
		name    string
		profile map[string]any
		want    string
	}{
		{
			name:    "TargetWrongType",
			profile: map[string]any{"rubric": "a rubric", "target": map[string]any{"paths": "not-a-list"}},
			want:    "target",
		},
		{
			name:    "FasitWrongType",
			profile: map[string]any{"rubric": "a rubric", "fasit": map[string]any{"paths": "not-a-list"}},
			want:    "fasit",
		},
		{
			name:    "TargetUnknownKey",
			profile: map[string]any{"rubric": "a rubric", "target": map[string]any{"pathz": []any{"x"}}},
			want:    "target",
		},
		{
			name:    "FasitUnknownKey",
			profile: map[string]any{"rubric": "a rubric", "fasit": map[string]any{"pathz": []any{"x"}}},
			want:    "fasit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			cfg := Config{"run_subdir": "review-segment", "profile": tt.profile}

			_, err := burlerRoundEntry("review-round", cfg, env)
			if err == nil {
				t.Fatal("burlerRoundEntry() error = nil; want non-nil")
			}
			assertErrContains(t, err, tt.want)
			assertErrContains(t, err, "BurlerRound")
		})
	}
}
