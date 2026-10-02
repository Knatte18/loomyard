package shedrecipe

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/parentreview"
)

// gatesCfg builds a row Config carrying a one-entry "gates" list, the shape every shipped gated row has.
func gatesCfg(name string, attempts int) Config {
	return Config{"gates": []any{map[string]any{"name": name, "attempts": attempts}}}
}

// gateCapableEntries lists every gate-capable Constructor with a Config that is otherwise valid, for the leftover-key rejection.
func gateCapableEntries() map[string]Constructor {
	return map[string]Constructor{
		"DiscussionWrite": discussionWriteEntry,
		"PlanWrite":       planWriteEntry,
		"Describe":        describeEntry,
		"PRRework":        prReworkEntry,
		"Webster":         websterEntry,
	}
}

func TestResolveGateSpec_AbsentKeyIsUngated(t *testing.T) {
	spec, err := resolveGateSpec("Row", Config{}, newTestEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 0 {
		t.Errorf("resolveGateSpec() = %+v; want the empty GateSpec", spec)
	}
}

func TestResolveGateSpec_ValidOneEntryPerVocabularyName(t *testing.T) {
	for _, name := range []string{"discussion", "plan", "rework-plan", "description"} {
		t.Run(name, func(t *testing.T) {
			env := reworkTestEnv(t)
			env.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
			spec, err := resolveGateSpec("Row", gatesCfg(name, 4), env)
			if err != nil {
				t.Fatalf("resolveGateSpec() error = %v; want nil", err)
			}
			if len(spec) != 1 {
				t.Fatalf("resolveGateSpec() = %+v; want one entry", spec)
			}
			if spec[0].Gate == nil || spec[0].Name != name || spec[0].Attempts != 4 || spec[0].PassOnCap {
				t.Errorf("resolveGateSpec()[0] = %+v; want closure set, Name %q, Attempts 4, PassOnCap false", spec[0], name)
			}
		})
	}
}

func TestResolveGateSpec_ListOrderAndPassOnCap(t *testing.T) {
	cfg := Config{"gates": []any{
		map[string]any{"name": "plan", "attempts": 2},
		map[string]any{"name": "discussion", "attempts": 1, "pass_on_cap": true},
	}}
	spec, err := resolveGateSpec("Row", cfg, newTestEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 2 || spec[0].Name != "plan" || spec[1].Name != "discussion" {
		t.Fatalf("resolveGateSpec() = %+v; want plan then discussion", spec)
	}
	if spec[0].PassOnCap || !spec[1].PassOnCap {
		t.Errorf("PassOnCap = %v, %v; want false, true", spec[0].PassOnCap, spec[1].PassOnCap)
	}
}

func TestResolveGateSpec_ZeroAttemptsEntryStillResolves(t *testing.T) {
	spec, err := resolveGateSpec("Row", gatesCfg("plan", 0), newTestEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 1 || spec[0].Attempts != 0 || spec[0].Gate == nil {
		t.Errorf("resolveGateSpec() = %+v; want one off entry with its closure resolved", spec)
	}

	// An off gate still checks its Env requirements, so a typo fails loud.
	env := newTestEnv(t)
	env.AnchorPath = ""
	_, err = resolveGateSpec("Row", gatesCfg("plan", 0), env)
	assertErrContains(t, err, "AnchorPath")
}

func TestResolveGateSpec_Rejections(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"EmptyList", Config{"gates": []any{}}, "gates"},
		{"NotAList", Config{"gates": "plan"}, "gates"},
		{"ElementNotAMap", Config{"gates": []any{"plan"}}, "element 0"},
		{"DuplicateName", Config{"gates": []any{
			map[string]any{"name": "plan", "attempts": 1},
			map[string]any{"name": "plan", "attempts": 2},
		}}, "repeats gate name"},
		{"NegativeAttempts", gatesCfg("plan", -1), "negative"},
		{"MissingName", Config{"gates": []any{map[string]any{"attempts": 1}}}, "name"},
		{"MissingAttempts", Config{"gates": []any{map[string]any{"name": "plan"}}}, "attempts"},
		{"UnknownElementKey", Config{"gates": []any{map[string]any{"name": "plan", "attempts": 1, "bogus": true}}}, "bogus"},
		{"UnknownName", gatesCfg("bogus", 1), "bogus"},
		{"UnknownNameNamesElement", Config{"gates": []any{
			map[string]any{"name": "plan", "attempts": 1},
			map[string]any{"name": "bogus", "attempts": 1},
		}}, `"gates" element 1`},
		{"BadPassOnCap", Config{"gates": []any{map[string]any{"name": "plan", "attempts": 1, "pass_on_cap": "yes"}}}, "pass_on_cap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveGateSpec("TheRow", tt.cfg, newTestEnv(t))
			assertErrContains(t, err, "TheRow")
			assertErrContains(t, err, tt.want)
		})
	}
}

func TestGateCapableEntries_RejectLeftoverOldKeys(t *testing.T) {
	for name, entry := range gateCapableEntries() {
		for _, key := range []string{"gate", "gate_attempts"} {
			t.Run(name+"/"+key, func(t *testing.T) {
				_, err := entry("Row", Config{key: "plan"}, reworkTestEnv(t))
				assertErrContains(t, err, "unrecognized config key")
				assertErrContains(t, err, key)
			})
		}
	}

	for _, key := range []string{"gate", "gate_attempts"} {
		t.Run("BurlerRound/"+key, func(t *testing.T) {
			cfg := minimalBurlerConfig()
			cfg[key] = "plan"
			_, err := burlerRoundEntry("Row", cfg, newTestEnv(t))
			assertErrContains(t, err, "unrecognized config key")
			assertErrContains(t, err, key)
		})
	}
}

// parentReviewEnv fills Env.ParentReview with the absolute paths and seams the "parent-review" gate requires.
func parentReviewEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	env.ParentReview = parentreview.GateConfig{
		Store:          parentreview.Store{Root: filepath.Join(dir, "reviews"), LockDir: filepath.Join(dir, "locks")},
		Slug:           "slug",
		Reviewer:       "hub:orch",
		DecisionRecord: filepath.Join(dir, "decision-record.md"),
		SupportLog:     filepath.Join(dir, "support-log.md"),
		WaitBound:      time.Hour,
		RenderDelivery: func(string) (string, error) { return "deliver", nil },
		RenderBrief:    func() (string, error) { return "brief", nil },
	}
	return env
}

// parentReviewCfg is a one-entry "gates" list naming parent-review with pass_on_cap set as given.
func parentReviewCfg(attempts int, passOnCap bool) Config {
	return Config{"gates": []any{map[string]any{"name": "parent-review", "attempts": attempts, "pass_on_cap": passOnCap}}}
}

func TestResolveGateSpec_ParentReviewResolvesWithFinal(t *testing.T) {
	spec, err := resolveGateSpec("Row", parentReviewCfg(3, true), parentReviewEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 1 || spec[0].Gate == nil || spec[0].Final == nil || !spec[0].PassOnCap || spec[0].Attempts != 3 {
		t.Fatalf("resolveGateSpec() = %+v; want one pass_on_cap entry with Gate and Final set", spec)
	}
}

func TestResolveGateSpec_OtherNamesCarryNoFinal(t *testing.T) {
	env := reworkTestEnv(t)
	env.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
	for _, name := range []string{"discussion", "plan", "rework-plan", "description"} {
		spec, err := resolveGateSpec("Row", gatesCfg(name, 1), env)
		if err != nil {
			t.Fatalf("%s: resolveGateSpec() error = %v", name, err)
		}
		if spec[0].Final != nil {
			t.Errorf("%s: Final is set; want nil", name)
		}
	}
}

func TestResolveGateSpec_ParentReviewRequiresPassOnCap(t *testing.T) {
	_, err := resolveGateSpec("TheRow", parentReviewCfg(3, false), parentReviewEnv(t))
	assertErrContains(t, err, "TheRow")
	assertErrContains(t, err, "pass_on_cap")
}

func TestResolveGateSpec_ParentReviewMissingEnv(t *testing.T) {
	tests := []struct {
		field string
		clear func(*Env)
	}{
		{"ParentReview.Store.Root", func(e *Env) { e.ParentReview.Store.Root = "" }},
		{"ParentReview.Store.LockDir", func(e *Env) { e.ParentReview.Store.LockDir = "" }},
		{"ParentReview.Slug", func(e *Env) { e.ParentReview.Slug = "" }},
		{"ParentReview.RenderDelivery", func(e *Env) { e.ParentReview.RenderDelivery = nil }},
		{"ParentReview.RenderBrief", func(e *Env) { e.ParentReview.RenderBrief = nil }},
		{"ParentReview.DecisionRecord", func(e *Env) { e.ParentReview.DecisionRecord = "" }},
		{"ParentReview.SupportLog", func(e *Env) { e.ParentReview.SupportLog = "relative.md" }},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			env := parentReviewEnv(t)
			tt.clear(&env)
			_, err := resolveGateSpec("Row", parentReviewCfg(1, true), env)
			assertErrContains(t, err, tt.field)
		})
	}
}

func TestResolveGateSpec_UnknownNameListsFiveValues(t *testing.T) {
	_, err := resolveGateSpec("Row", gatesCfg("bogus", 1), newTestEnv(t))
	for _, want := range []string{"discussion", "plan", "rework-plan", "description", "parent-review"} {
		assertErrContains(t, err, want)
	}
}
