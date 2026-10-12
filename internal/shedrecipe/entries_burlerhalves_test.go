// entries_burlerhalves_test.go covers burlerReviewEntry and burlerFixEntry: the happy paths, the profile-to-Profile mapping (including the target relative-path exception), strict unknown-key rejection, the run-directory group shared with entries_bouncer_test.go's Bouncer coverage, every construction failure, and the "gates" Config key the fix entry resolves into RunOpts.Gate.

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

// minimalReviewConfig returns a Config carrying only the required BurlerReview keys:
// a run_subdir and a profile map with a single rubric key, non-empty so it does not count as absent.
func minimalReviewConfig() Config {
	return Config{
		"run_subdir": "review-segment",
		"profile":    map[string]any{"rubric": "a rubric"},
	}
}

// minimalFixConfig returns a Config carrying only the required BurlerFix keys:
// a run_subdir, the review row it answers to and a profile map with a single fix-scope key, non-empty so it does not count as absent.
func minimalFixConfig() Config {
	return Config{
		"run_subdir": "review-segment",
		"review_row": "review-row",
		"profile":    map[string]any{"fix-scope": "source"},
	}
}

// lookupEntry returns the registry Constructor called name.
func lookupEntry(t *testing.T, name string) Constructor {
	t.Helper()
	constructor, err := Lookup(name)
	if err != nil {
		t.Fatalf("Lookup(%q) error = %v; want nil", name, err)
	}
	return constructor
}

// captureReview constructs a BurlerReview entry from cfg and env, drives one Call so env.Burler's shedfake.BurlerHalfRunner records the Profile and RunOpts it was handed, and returns them.
// Call's own return values are ignored:
// the unscripted fake returns a zero burlerengine.ReviewResult with a nil error, which the producer's outcome switch does not recognise, so Call itself returns a non-nil error on every path here.
// That error is not this helper's concern, only what the runner recorded is.
func captureReview(t *testing.T, name string, cfg Config, env Env) (burlerengine.Profile, burlerengine.RunOpts) {
	t.Helper()
	producer, err := burlerReviewEntry(name, cfg, env)
	if err != nil {
		t.Fatalf("burlerReviewEntry() error = %v; want nil", err)
	}

	_, _, _ = producer.Call(context.Background())

	fake := env.Burler.(*shedfake.BurlerHalfRunner)
	if len(fake.GotReviewProfiles) != 1 || len(fake.GotReviewOpts) != 1 {
		t.Fatalf("BurlerHalfRunner recorded %d profiles and %d opts for Review; want 1 and 1", len(fake.GotReviewProfiles), len(fake.GotReviewOpts))
	}
	return fake.GotReviewProfiles[0], fake.GotReviewOpts[0]
}

// captureFix constructs a BurlerFix entry from cfg and env, accepts a round-1 review in the run directory the row's run_subdir names so the producer reaches its runner, drives one Call and returns the Profile and RunOpts env.Burler's fake recorded for Fix.
func captureFix(t *testing.T, name string, cfg Config, env Env) (burlerengine.Profile, burlerengine.RunOpts) {
	t.Helper()
	producer, err := burlerFixEntry(name, cfg, env)
	if err != nil {
		t.Fatalf("burlerFixEntry() error = %v; want nil", err)
	}

	runDir := filepath.Join(env.RunRoot, cfg["run_subdir"].(string))
	if err := os.WriteFile(filepath.Join(runDir, "round-1-review.md"), []byte("review\n"), 0o644); err != nil {
		t.Fatalf("write the round-1 review: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "round-1-acceptance.md"), []byte("---\nround: 1\ndigest: abc\n---\n"), 0o644); err != nil {
		t.Fatalf("write the round-1 acceptance record: %v", err)
	}
	_, _, _ = producer.Call(context.Background())

	fake := env.Burler.(*shedfake.BurlerHalfRunner)
	if len(fake.GotFixProfiles) != 1 || len(fake.GotFixOpts) != 1 {
		t.Fatalf("BurlerHalfRunner recorded %d profiles and %d opts for Fix; want 1 and 1", len(fake.GotFixProfiles), len(fake.GotFixOpts))
	}
	return fake.GotFixProfiles[0], fake.GotFixOpts[0]
}

func TestBurlerReviewEntry_ProfileMapping(t *testing.T) {
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

	profile, opts := captureReview(t, "review-row", cfg, env)

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
	if len(opts.Gate) != 0 {
		t.Errorf("opts.Gate = %+v; want none, the review row's only gate is the engine's review parse", opts.Gate)
	}
}

// TestBurlerReviewEntry_RubricStencil covers the profile.rubric_stencil key:
// resolving a seeded stencil's content into Profile.Rubric, stripping a leading stamp banner from it, the rubric/rubric_stencil mutual-exclusivity rule, an unseeded stencil name,
// and the empty-Env.StencilsDir construction error being scoped to the rubric_stencil path only.
func TestBurlerReviewEntry_RubricStencil(t *testing.T) {
	stencilCfg := Config{
		"run_subdir": "review-segment",
		"profile":    map[string]any{"rubric_stencil": "round-rubric"},
	}

	t.Run("ResolvesSeededStencilContent", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "round-rubric", "BLOCKING: a round bug.\n")

		profile, _ := captureReview(t, "review-row", stencilCfg, env)
		if profile.Rubric != "BLOCKING: a round bug.\n" {
			t.Errorf("profile.Rubric = %q; want %q", profile.Rubric, "BLOCKING: a round bug.\n")
		}
	})

	t.Run("FillsTheWebsterRecordNote", func(t *testing.T) {
		env := newTestEnv(t)
		env.WebsterRecord = func() string { return "A known note." }
		writeStencil(t, env.StencilsDir, "round-rubric", "Record: {{.webster_record}}\n")

		profile, _ := captureReview(t, "review-row", stencilCfg, env)
		if profile.Rubric != "Record: A known note.\n" {
			t.Errorf("profile.Rubric = %q; want the env's webster record note filled into the marker", profile.Rubric)
		}
	})

	t.Run("StripsLeadingStampBanner", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "round-rubric", "<!-- lyx-stencil: sha256=aaaa -->\nBLOCKING: a round bug.\n")

		profile, _ := captureReview(t, "review-row", stencilCfg, env)
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
		_, err := burlerReviewEntry("review-row", cfg, env)
		assertErrContains(t, err, "rubric")
		assertErrContains(t, err, "rubric_stencil")
	})

	t.Run("NeitherRubricNorRubricStencilFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"fix-scope": "source"},
		}
		_, err := burlerReviewEntry("review-row", cfg, env)
		assertErrContains(t, err, "rubric")
		assertErrContains(t, err, "rubric_stencil")
	})

	t.Run("UnseededRubricStencilFails", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := Config{
			"run_subdir": "review-segment",
			"profile":    map[string]any{"rubric_stencil": "no-such-round-rubric"},
		}
		_, err := burlerReviewEntry("review-row", cfg, env)
		assertErrContains(t, err, "no-such-round-rubric")
	})

	t.Run("EmptyStencilsDirFailsOnRubricStencilPathOnly", func(t *testing.T) {
		env := newTestEnv(t)
		env.StencilsDir = ""
		_, err := burlerReviewEntry("review-row", stencilCfg, env)
		assertErrContains(t, err, "StencilsDir")

		if _, err := burlerReviewEntry("review-row", minimalReviewConfig(), env); err != nil {
			t.Fatalf("burlerReviewEntry() with a literal rubric error = %v; want nil", err)
		}
	})

	t.Run("EmptySpecsDirFailsOnRubricStencilPathOnly", func(t *testing.T) {
		env := newTestEnv(t)
		writeStencil(t, env.StencilsDir, "round-rubric", "BLOCKING: a round bug.\n")
		env.SpecsDir = ""
		_, err := burlerReviewEntry("review-row", stencilCfg, env)
		assertErrContains(t, err, "SpecsDir")

		if _, err := burlerReviewEntry("review-row", minimalReviewConfig(), env); err != nil {
			t.Fatalf("burlerReviewEntry() with a literal rubric error = %v; want nil", err)
		}
	})
}

// TestBurlerHalfEntries_EnvFallback covers the model and timeout_s resolution both entries share:
// each half's model is the first pick of its list in env.ReviewModels and a row has no key to change it;
// a row omitting timeout_s takes env.ReviewTimeout and a row setting it overrides the Env value;
// both absent with an empty Env leaves the zero values.
func TestBurlerHalfEntries_EnvFallback(t *testing.T) {
	halves := []struct {
		name    string
		entry   string
		cfg     func() Config
		capture func(t *testing.T, name string, cfg Config, env Env) (burlerengine.Profile, burlerengine.RunOpts)
	}{
		{"Review", "BurlerReview", minimalReviewConfig, captureReview},
		{"Fix", "BurlerFix", minimalFixConfig, captureFix},
	}
	for _, half := range halves {
		t.Run(half.name, func(t *testing.T) {
			t.Run("RowOmitsTakesEnvValues", func(t *testing.T) {
				env := newTestEnv(t)
				env.ReviewModels = burlerengine.RoundModels{
					Review: []burlerengine.ModelChoice{{Model: "env-model", Effort: "env-effort"}},
					Fix:    []burlerengine.ModelChoice{{Model: "env-fix-model", Effort: "env-fix-effort"}},
				}
				env.ReviewTimeout = 45 * time.Second

				_, opts := half.capture(t, "a-row", half.cfg(), env)
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
					"a-row": {
						Review: []burlerengine.ModelChoice{{Model: "row-model"}},
						Fix:    []burlerengine.ModelChoice{{Model: "row-fix-model"}},
					},
				}

				_, opts := half.capture(t, "a-row", half.cfg(), env)
				if opts.Review.Model != "row-model" || opts.Fix.Model != "row-fix-model" {
					t.Errorf("opts models = (%+v, %+v); want the row's own entry (row-model, row-fix-model)", opts.Review, opts.Fix)
				}

				otherEnv := newTestEnv(t)
				otherEnv.ReviewModels, otherEnv.RowReviewModels = env.ReviewModels, env.RowReviewModels
				_, other := half.capture(t, "other-row", half.cfg(), otherEnv)
				if other.Review.Model != "env-model" || other.Fix.Model != "env-fix-model" {
					t.Errorf("other row's models = (%+v, %+v); want Env.ReviewModels (env-model, env-fix-model)", other.Review, other.Fix)
				}
			})

			t.Run("RowSetsOverridesEnvValues", func(t *testing.T) {
				env := newTestEnv(t)
				env.ReviewTimeout = 45 * time.Second
				cfg := half.cfg()
				cfg["timeout_s"] = 30

				_, opts := half.capture(t, "a-row", cfg, env)
				if opts.Timeout != 30*time.Second {
					t.Errorf("opts.Timeout = %v; want %v", opts.Timeout, 30*time.Second)
				}
			})

			t.Run("BothAbsentLeavesZeroValues", func(t *testing.T) {
				_, opts := half.capture(t, "a-row", half.cfg(), newTestEnv(t))
				if opts.Review != (burlerengine.ModelChoice{}) || opts.Fix != (burlerengine.ModelChoice{}) {
					t.Errorf("opts models = (%+v, %+v); want the zero choices", opts.Review, opts.Fix)
				}
				if opts.Timeout != 0 {
					t.Errorf("opts.Timeout = %v; want 0", opts.Timeout)
				}
			})

			t.Run("FractionalTimeoutRejected", func(t *testing.T) {
				cfg := half.cfg()
				cfg["timeout_s"] = 30.5
				_, err := lookupEntry(t, half.entry)("a-row", cfg, newTestEnv(t))
				assertErrContains(t, err, "timeout_s")
			})
		})
	}
}

func TestBurlerReviewEntry_ClusterFan(t *testing.T) {
	t.Run("RowClusterFansEntryReachesItsOwnRowOnly", func(t *testing.T) {
		env := newTestEnv(t)
		env.RowClusterFans = map[string]string{"review-row": "env-fan"}

		profile, _ := captureReview(t, "review-row", minimalReviewConfig(), env)
		if profile.ClusterFan != "env-fan" {
			t.Errorf("profile.ClusterFan = %q; want the row's RowClusterFans entry env-fan", profile.ClusterFan)
		}

		otherEnv := newTestEnv(t)
		otherEnv.RowClusterFans = env.RowClusterFans
		cfg := minimalReviewConfig()
		cfg["profile"].(map[string]any)["cluster-fan"] = "recipe-fan"
		other, _ := captureReview(t, "other-row", cfg, otherEnv)
		if other.ClusterFan != "recipe-fan" {
			t.Errorf("other row's ClusterFan = %q; want the recipe's recipe-fan, the map names another row", other.ClusterFan)
		}
	})

	t.Run("EnvFanBesideRecipeClusterFanNamesBoth", func(t *testing.T) {
		env := newTestEnv(t)
		env.RowClusterFans = map[string]string{"review-row": "env-fan"}
		cfg := minimalReviewConfig()
		cfg["profile"].(map[string]any)["cluster-fan"] = "recipe-fan"
		_, err := burlerReviewEntry("review-row", cfg, env)
		for _, want := range []string{"review-row", "env-fan", "recipe-fan", "loom.yaml", "remove cluster-fan"} {
			assertErrContains(t, err, want)
		}
	})
}

func TestBurlerHalfEntries_RelativePathException(t *testing.T) {
	env := newTestEnv(t)
	cfg := Config{
		"run_subdir": "review-segment",
		"profile": map[string]any{
			"target": map[string]any{"paths": []string{"relative/target.md"}},
			"fasit":  map[string]any{"paths": []string{"/absolute/fasit.md"}},
			"rubric": "a rubric",
		},
	}

	profile, _ := captureReview(t, "review-row", cfg, env)

	if len(profile.Target.Paths) != 1 || profile.Target.Paths[0] != "relative/target.md" {
		t.Errorf("profile.Target.Paths = %v; want [\"relative/target.md\"] unchanged", profile.Target.Paths)
	}
	if len(profile.Fasit.Paths) != 1 || profile.Fasit.Paths[0] != "/absolute/fasit.md" {
		t.Errorf("profile.Fasit.Paths = %v; want [\"/absolute/fasit.md\"] unchanged -- an absolute value here is not rejected", profile.Fasit.Paths)
	}

	fixProfile, _ := captureFix(t, "fix-row", Config{
		"run_subdir": "review-segment",
		"review_row": "review-row",
		"profile":    map[string]any{"target": map[string]any{"paths": []string{"relative/target.md"}}, "fix-scope": "overlay"},
	}, newTestEnv(t))
	if len(fixProfile.Target.Paths) != 1 || fixProfile.Target.Paths[0] != "relative/target.md" {
		t.Errorf("fix profile.Target.Paths = %v; want [\"relative/target.md\"] unchanged", fixProfile.Target.Paths)
	}
	if fixProfile.FixScope != burlerengine.FixScopeOverlay {
		t.Errorf("fix profile.FixScope = %q; want %q", fixProfile.FixScope, burlerengine.FixScopeOverlay)
	}
}

// TestBurlerHalfEntries_StrictUnknownKeys covers unknown-key rejection at every level of both entries.
// The fix entry refuses the review-only keys: the fasit, the rubric, the cluster fan, tool use and the row-level gates it alone takes are not shared.
func TestBurlerHalfEntries_StrictUnknownKeys(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		cfg   func() Config
		want  string
	}{
		{"ReviewProfileKey", "BurlerReview", func() Config {
			cfg := minimalReviewConfig()
			cfg["profile"].(map[string]any)["unexpected"] = "value"
			return cfg
		}, "unexpected"},
		{"ReviewRowKeyModel", "BurlerReview", func() Config { cfg := minimalReviewConfig(); cfg["model"] = "value"; return cfg }, "model"},
		{"ReviewRowKeyEffort", "BurlerReview", func() Config { cfg := minimalReviewConfig(); cfg["effort"] = "value"; return cfg }, "effort"},
		{"ReviewRowKeyGates", "BurlerReview", func() Config { cfg := minimalReviewConfig(); cfg["gates"] = gatesCfg("plan", 3)["gates"]; return cfg }, "gates"},
		{"ReviewRowKeyReviewRow", "BurlerReview", func() Config { cfg := minimalReviewConfig(); cfg["review_row"] = "x"; return cfg }, "review_row"},
		{"ReviewProfileGates", "BurlerReview", func() Config {
			cfg := minimalReviewConfig()
			cfg["profile"].(map[string]any)["gates"] = gatesCfg("plan", 3)["gates"]
			return cfg
		}, "gates"},
		{"ReviewTargetInnerKey", "BurlerReview", func() Config {
			cfg := minimalReviewConfig()
			cfg["profile"].(map[string]any)["target"] = map[string]any{"paths": []string{"t.md"}, "unexpected": "value"}
			return cfg
		}, "unexpected"},
		{"ReviewFasitInnerKey", "BurlerReview", func() Config {
			cfg := minimalReviewConfig()
			cfg["profile"].(map[string]any)["fasit"] = map[string]any{"paths": []string{"f.md"}, "unexpected": "value"}
			return cfg
		}, "unexpected"},
		{"FixRowKeyModel", "BurlerFix", func() Config { cfg := minimalFixConfig(); cfg["model"] = "value"; return cfg }, "model"},
		{"FixTargetInnerKey", "BurlerFix", func() Config {
			cfg := minimalFixConfig()
			cfg["profile"].(map[string]any)["target"] = map[string]any{"paths": []string{"t.md"}, "unexpected": "value"}
			return cfg
		}, "unexpected"},
	}
	for _, key := range []string{"review-path", "fixer-report-path", "prior-reviews", "prior-fixer-reports", "cluster-exclude"} {
		tests = append(tests, struct {
			name  string
			entry string
			cfg   func() Config
			want  string
		}{"ReviewProfileKey_" + key, "BurlerReview", func() Config {
			cfg := minimalReviewConfig()
			cfg["profile"].(map[string]any)[key] = "value"
			return cfg
		}, key})
	}
	for _, key := range []string{"fasit", "rubric", "rubric_stencil", "tool-use", "cluster-fan", "review-path"} {
		tests = append(tests, struct {
			name  string
			entry string
			cfg   func() Config
			want  string
		}{"FixProfileKey_" + key, "BurlerFix", func() Config {
			cfg := minimalFixConfig()
			cfg["profile"].(map[string]any)[key] = "value"
			return cfg
		}, key})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := lookupEntry(t, tt.entry)("a-row", tt.cfg(), newTestEnv(t))
			assertErrContains(t, err, "unrecognized config key")
			assertErrContains(t, err, tt.want)
		})
	}
}

func TestBurlerHalfEntries_RunDirectory(t *testing.T) {
	halves := []struct {
		name  string
		build func(name string, cfg Config, env Env) error
		cfg   func() Config
	}{
		{"Review", func(name string, cfg Config, env Env) error { _, err := burlerReviewEntry(name, cfg, env); return err }, minimalReviewConfig},
		{"Fix", func(name string, cfg Config, env Env) error { _, err := burlerFixEntry(name, cfg, env); return err }, minimalFixConfig},
	}
	for _, half := range halves {
		t.Run(half.name+"/CreatedBeforeAnyCall", func(t *testing.T) {
			env := newTestEnv(t)
			if err := half.build("a-row", half.cfg(), env); err != nil {
				t.Fatalf("entry error = %v; want nil", err)
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

		t.Run(half.name+"/EscapingFails", func(t *testing.T) {
			cfg := half.cfg()
			cfg["run_subdir"] = "../escape"
			assertErrContains(t, half.build("a-row", cfg, newTestEnv(t)), "run_subdir")
		})
	}

	t.Run("SameSubdirAsBouncerResolvesToSameDir", func(t *testing.T) {
		env := newTestEnv(t)
		bouncerCfg := minimalBouncerConfig(t, env)
		bouncerCfg["run_subdir"] = "review-segment"

		if _, err := burlerReviewEntry("review-row", minimalReviewConfig(), env); err != nil {
			t.Fatalf("burlerReviewEntry() error = %v; want nil", err)
		}
		if _, err := burlerFixEntry("fix-row", minimalFixConfig(), env); err != nil {
			t.Fatalf("burlerFixEntry() error = %v; want nil", err)
		}
		if _, err := bouncerEntry("review-bounce", bouncerCfg, env); err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}

		wantDir := filepath.Join(env.RunRoot, "review-segment")
		if _, err := os.Stat(wantDir); err != nil {
			t.Fatalf("os.Stat(%q) error = %v; want the shared run dir to exist", wantDir, err)
		}
	})
}

// TestBurlerFixEntry_GateConfig covers the "gates" Config key burlerFixEntry resolves through resolveRowGateSpec into RunOpts.Gate:
// a "gates" list is accepted at row level and selects the matching validators, "gates" is rejected inside the profile: sub-map -- the guard against the two allowlists being confused -- and an unrecognised name fails loud.
func TestBurlerFixEntry_GateConfig(t *testing.T) {
	t.Run("RowLevelGatesAccepted", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalFixConfig()
		cfg["gates"] = gatesCfg("discussion", 5)["gates"]

		_, opts := captureFix(t, "fix-row", cfg, env)
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
		cfg := minimalFixConfig()
		cfg["gates"] = gatesCfg("plan", 3)["gates"]

		_, opts := captureFix(t, "fix-row", cfg, env)
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

	t.Run("NoGateKeyBuildsUngatedFixer", func(t *testing.T) {
		_, opts := captureFix(t, "fix-row", minimalFixConfig(), newTestEnv(t))
		if len(opts.Gate) != 0 {
			t.Errorf("opts.Gate = %+v; want an empty list for a row carrying no gate key", opts.Gate)
		}
	})

	t.Run("UnrecognisedGateValueFails", func(t *testing.T) {
		cfg := minimalFixConfig()
		cfg["gates"] = gatesCfg("webster", 3)["gates"]

		_, err := burlerFixEntry("fix-row", cfg, newTestEnv(t))
		assertErrContains(t, err, "name")
		assertErrContains(t, err, "discussion")
		assertErrContains(t, err, "plan")
	})

	t.Run("GatesRejectedInsideProfileSubMap", func(t *testing.T) {
		cfg := minimalFixConfig()
		cfg["profile"].(map[string]any)["gates"] = gatesCfg("plan", 3)["gates"]

		_, err := burlerFixEntry("fix-row", cfg, newTestEnv(t))
		assertErrContains(t, err, "unrecognized config key")
		assertErrContains(t, err, "gates")
	})
}

func TestBurlerHalfEntries_ConstructionFailures(t *testing.T) {
	halves := []struct {
		name  string
		entry string
		cfg   func() Config
	}{
		{"Review", "BurlerReview", minimalReviewConfig},
		{"Fix", "BurlerFix", minimalFixConfig},
	}
	for _, half := range halves {
		build := func(t *testing.T, cfg Config, env Env) error {
			t.Helper()
			_, err := lookupEntry(t, half.entry)("a-row", cfg, env)
			return err
		}
		t.Run(half.name+"/HappyPath", func(t *testing.T) {
			producer, err := lookupEntry(t, half.entry)("a-row", half.cfg(), newTestEnv(t))
			if err != nil || producer == nil {
				t.Fatalf("entry() = %v, %v; want a producer", producer, err)
			}
		})
		t.Run(half.name+"/MissingProfile", func(t *testing.T) {
			cfg := half.cfg()
			delete(cfg, "profile")
			assertErrContains(t, build(t, cfg, newTestEnv(t)), "profile")
		})
		t.Run(half.name+"/BlankEnvRunRoot", func(t *testing.T) {
			env := newTestEnv(t)
			env.RunRoot = ""
			assertErrContains(t, build(t, half.cfg(), env), "RunRoot")
		})
		t.Run(half.name+"/NilEnvBurler", func(t *testing.T) {
			env := newTestEnv(t)
			env.Burler = nil
			assertErrContains(t, build(t, half.cfg(), env), "Burler")
		})
		t.Run(half.name+"/NilEnvNowAndShuttleConstructSuccessfully", func(t *testing.T) {
			env := newTestEnv(t)
			env.Now = nil
			env.Shuttle = nil
			if err := build(t, half.cfg(), env); err != nil {
				t.Fatalf("entry() error = %v; want nil", err)
			}
		})
	}

	t.Run("FixMissingReviewRow", func(t *testing.T) {
		cfg := minimalFixConfig()
		delete(cfg, "review_row")
		_, err := burlerFixEntry("fix-row", cfg, newTestEnv(t))
		assertErrContains(t, err, "review_row")
	})
}

// TestBurlerHalfEntries_FileSetErrorsNameTheirOwnMap pins the entry/field qualification on profile.target and profile.fasit errors.
// The two maps carry identical key sets, so without it a recipe author with a typo in one of them is told which key and never which map.
// The two error strings are byte-identical.
func TestBurlerHalfEntries_FileSetErrorsNameTheirOwnMap(t *testing.T) {
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
			cfg := Config{"run_subdir": "review-segment", "profile": tt.profile}

			_, err := burlerReviewEntry("review-row", cfg, newTestEnv(t))
			if err == nil {
				t.Fatal("burlerReviewEntry() error = nil; want non-nil")
			}
			assertErrContains(t, err, tt.want)
			assertErrContains(t, err, "BurlerReview")
		})
	}

	t.Run("FixTargetWrongType", func(t *testing.T) {
		cfg := minimalFixConfig()
		cfg["profile"].(map[string]any)["target"] = map[string]any{"paths": "not-a-list"}

		_, err := burlerFixEntry("fix-row", cfg, newTestEnv(t))
		assertErrContains(t, err, "target")
		assertErrContains(t, err, "BurlerFix")
	})
}
