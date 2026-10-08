// entries_burler_test.go covers burlerRoundEntry: the happy path, the profile-to-Profile mapping (including the target/fasit relative-path exception), strict unknown-key rejection at all three levels, the run-directory group shared with entries_bouncer_test.go's Bouncer coverage, every construction failure, and the "gates" Config key resolveGateSpec resolves into RunOpts.Gate.
//
// seam_enforcement_test.go's allowlist assertion is the standing guard that loomshed's two gate
// closures did not drift into this package's own import set; it needs no edit for this coverage and
// none is made here.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
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
// env.Burler's shedfake.BurlerRunner records the Profile and RunOpts it was handed, and returns them.
// Call's own return values are ignored: the unscripted shedfake.BurlerRunner returns a zero burlerengine.Result
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

	fake := env.Burler.(*shedfake.BurlerRunner)
	if len(fake.GotProfiles) != 1 || len(fake.GotOpts) != 1 {
		t.Fatalf("BurlerRunner recorded %d profiles and %d opts; want 1 and 1", len(fake.GotProfiles), len(fake.GotOpts))
	}
	return fake.GotProfiles[0], fake.GotOpts[0]
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

	t.Run("EmptySpecsDirFailsOnRubricStencilPathOnly", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "round-rubric", "BLOCKING: a round bug.\n")
		env.SpecsDir = ""
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric_stencil": "round-rubric"},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "SpecsDir")
	})

	t.Run("EmptySpecsDirConstructsCleanlyWithLiteralRubric", func(t *testing.T) {
		env := newTestEnv(t)
		env.SpecsDir = ""
		cfg := minimalBurlerConfig()

		producer, err := burlerRoundEntry("review-round", cfg, env)
		if err != nil {
			t.Fatalf("burlerRoundEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatal("burlerRoundEntry() producer = nil; want non-nil")
		}
	})

	t.Run("RelativeAnchorPathIsRefused", func(t *testing.T) {
		env := newTestEnv(t)
		env.AnchorPath = "relative/anchor"
		cfg := minimalBurlerConfig()

		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "AnchorPath")
	})

	// A nil BurlerRemover is refused rather than tolerated: it stops the one live half of a resumed round.
	// A producer built without it respawns beside a still-live half, which is two agents writing one file and, on a fix-scope: source row, two agents committing to one branch.
	// A wiring slip must fail here, at construction, not silently at the next crash.
	t.Run("NilBurlerRemoverSeamIsRefused", func(t *testing.T) {
		env := newTestEnv(t)
		env.BurlerRemover = nil
		cfg := minimalBurlerConfig()

		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "BurlerRemover")
	})

	// The round probes through its runner, so the row reads no Shuttle.
	t.Run("NilShuttleSeamIsTolerated", func(t *testing.T) {
		env := newTestEnv(t)
		env.Shuttle = nil
		cfg := minimalBurlerConfig()

		if _, err := burlerRoundEntry("review-round", cfg, env); err != nil {
			t.Fatalf("burlerRoundEntry() error = %v; want nil", err)
		}
	})
}

// TestBurlerRoundEntry_EnvReviewFallback covers burlerRoundEntry's model and timeout_s resolution:
// each half's model is the first pick of its list in env.ReviewModels and a row has no key to change it;
// a row omitting timeout_s takes env.ReviewTimeout and a row setting it overrides the Env value;
// both absent with an empty Env leaves the zero values.
func TestBurlerRoundEntry_EnvReviewFallback(t *testing.T) {
	t.Run("RowOmitsTakesEnvValues", func(t *testing.T) {
		env := newTestEnv(t)
		env.ReviewModels = burlerengine.RoundModels{
			Review: []burlerengine.ModelChoice{{Model: "env-model", Effort: "env-effort"}},
			Fix:    []burlerengine.ModelChoice{{Model: "env-fix-model", Effort: "env-fix-effort"}},
		}
		env.ReviewTimeout = 45 * time.Second
		cfg := minimalBurlerConfig()

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if want := (burlerengine.ModelChoice{Model: "env-model", Effort: "env-effort"}); opts.Review != want {
			t.Errorf("opts.Review = %+v; want %+v", opts.Review, want)
		}
		if want := (burlerengine.ModelChoice{Model: "env-fix-model", Effort: "env-fix-effort"}); opts.Fix != want {
			t.Errorf("opts.Fix = %+v; want %+v", opts.Fix, want)
		}
		if opts.Timeout != 45*time.Second {
			t.Errorf("opts.Timeout = %v; want %v", opts.Timeout, 45*time.Second)
		}
	})

	t.Run("RowMapEntryReplacesEnvModelsForItsOwnRowOnly", func(t *testing.T) {
		env := newTestEnv(t)
		env.ReviewModels = burlerengine.RoundModels{
			Review: []burlerengine.ModelChoice{{Model: "env-model"}},
			Fix:    []burlerengine.ModelChoice{{Model: "env-fix-model"}},
		}
		env.RowReviewModels = map[string]burlerengine.RoundModels{
			"review-round": {
				Review: []burlerengine.ModelChoice{{Model: "row-model"}},
				Fix:    []burlerengine.ModelChoice{{Model: "row-fix-model"}},
			},
		}
		cfg := minimalBurlerConfig()

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if opts.Review.Model != "row-model" || opts.Fix.Model != "row-fix-model" {
			t.Errorf("opts models = (%+v, %+v); want the row's own entry (row-model, row-fix-model)", opts.Review, opts.Fix)
		}

		otherEnv := newTestEnv(t)
		otherEnv.ReviewModels, otherEnv.RowReviewModels = env.ReviewModels, env.RowReviewModels
		_, other := callAndCaptureProfile(t, "other-round", cfg, otherEnv)
		if other.Review.Model != "env-model" || other.Fix.Model != "env-fix-model" {
			t.Errorf("other row's models = (%+v, %+v); want Env.ReviewModels (env-model, env-fix-model)", other.Review, other.Fix)
		}
	})

	t.Run("RowSetsOverridesEnvValues", func(t *testing.T) {
		env := newTestEnv(t)
		env.ReviewTimeout = 45 * time.Second
		cfg := minimalBurlerConfig()
		cfg["timeout_s"] = 30

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if opts.Timeout != 30*time.Second {
			t.Errorf("opts.Timeout = %v; want %v", opts.Timeout, 30*time.Second)
		}
	})

	t.Run("BothAbsentLeavesZeroValues", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if opts.Review != (burlerengine.ModelChoice{}) || opts.Fix != (burlerengine.ModelChoice{}) {
			t.Errorf("opts models = (%+v, %+v); want the zero choices", opts.Review, opts.Fix)
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
	t.Run("ProfileKey", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric": "a rubric", "unexpected": "value"},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "unexpected")
	})

	// The retired model and effort keys: model choice lives in loom.yaml's review and fix keys only.
	for _, key := range []string{"model", "effort"} {
		t.Run("RowKey_"+key, func(t *testing.T) {
			env := newTestEnv(t)
			cfg := minimalBurlerConfig()
			cfg[key] = "value"
			_, err := burlerRoundEntry("review-round", cfg, env)
			assertErrContains(t, err, key)
		})
	}

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

	t.Run("EscapingFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{"run_subdir": "../escape", "profile": map[string]any{"rubric": "a rubric"}}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "run_subdir")
	})
}

// TestBurlerRoundEntry_GateConfig covers the "gates" Config key burlerRoundEntry resolves through resolveGateSpec into RunOpts.Gate:
// a "gates" list is accepted at row level and selects the matching validators, "gates" is rejected inside the profile: sub-map -- the guard against the two allowlists being confused -- and an unrecognised name fails loud, matching the Webster round's own deliberate lack of a third validator to name.
func TestBurlerRoundEntry_GateConfig(t *testing.T) {
	t.Run("RowLevelGatesAccepted", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()
		cfg["gates"] = gatesCfg("discussion", 5)["gates"]

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if len(opts.Gate) != 1 || opts.Gate[0].Gate == nil {
			t.Fatalf("opts.Gate = %+v; want one entry carrying the discussion closure", opts.Gate)
		}
		if opts.Gate[0].Attempts != 5 {
			t.Errorf("opts.Gate[0].Attempts = %d; want 5", opts.Gate[0].Attempts)
		}

		// Drive the constructed gate rather than comparing func values, which Go cannot compare.
		result, err := opts.Gate[0].Gate()
		if err != nil {
			t.Fatalf("opts.Gate[0].Gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatal("opts.Gate[0].Gate() Passed = true; want false for a missing support log")
		}
		if !strings.Contains(result.Findings, "support log does not exist") {
			t.Errorf("opts.Gate[0].Gate() Findings = %q; want it to name the missing support log, proving the discussion validator ran", result.Findings)
		}
	})

	t.Run("GatePlanResolvesToPlanValidator", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()
		cfg["gates"] = gatesCfg("plan", 3)["gates"]

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if len(opts.Gate) != 1 {
			t.Fatalf("opts.Gate = %+v; want one entry", opts.Gate)
		}
		result, err := opts.Gate[0].Gate()
		if err != nil {
			t.Fatalf("opts.Gate[0].Gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatal("opts.Gate[0].Gate() Passed = true; want false for a missing plan overview")
		}
		if !strings.Contains(result.Findings, "plan overview not found") {
			t.Errorf("opts.Gate[0].Gate() Findings = %q; want it to name the missing plan overview, proving the plan validator ran", result.Findings)
		}
	})

	t.Run("NoGateKeyBuildsUngatedRound", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()

		_, opts := callAndCaptureProfile(t, "review-round", cfg, env)
		if len(opts.Gate) != 0 {
			t.Errorf("opts.Gate = %+v; want an empty list for a row carrying no gate key -- the Webster round's own shape", opts.Gate)
		}
	})

	t.Run("UnrecognisedGateValueFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBurlerConfig()
		cfg["gates"] = gatesCfg("webster", 3)["gates"]

		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "name")
		assertErrContains(t, err, "discussion")
		assertErrContains(t, err, "plan")
	})

	t.Run("GatesRejectedInsideProfileSubMap", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric": "a rubric", "gates": gatesCfg("plan", 3)["gates"]},
		}
		_, err := burlerRoundEntry("review-round", cfg, env)
		assertErrContains(t, err, "unrecognized config key")
		assertErrContains(t, err, "gates")
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
