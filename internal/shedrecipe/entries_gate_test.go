package shedrecipe

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// parentReviewCfg is a one-entry "gates" list naming parent-review with the given attempts and no pass_on_cap.
func parentReviewCfg(attempts int) Config {
	return Config{"gates": []any{map[string]any{"name": "parent-review", "attempts": attempts}}}
}

func TestResolveGateSpec_ParentReviewResolvesMustPassMayHold(t *testing.T) {
	spec, err := resolveGateSpec("Row", parentReviewCfg(3), parentReviewEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 1 || spec[0].Gate == nil || spec[0].Final == nil || spec[0].PassOnCap || !spec[0].MayHold || spec[0].Attempts != 3 {
		t.Fatalf("resolveGateSpec() = %+v; want one must-pass MayHold entry with Gate and Final set and Attempts 3", spec)
	}
}

// TestResolveGateSpec_ParentReviewCapEqualsAttempts asserts the closure's reject cap is the entry's Attempts: two rejects are non-terminal failures and the third is terminal.
func TestResolveGateSpec_ParentReviewCapEqualsAttempts(t *testing.T) {
	env := parentReviewEnv(t)
	spec, err := resolveGateSpec("Row", parentReviewCfg(3), env)
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	gate := spec[0].Gate
	review := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(review, []byte("fix this"), 0o644); err != nil {
		t.Fatal(err)
	}
	eval := func() shuttleengine.GateResult {
		t.Helper()
		res, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v", err)
		}
		return res
	}
	if res := eval(); !res.Pending {
		t.Fatalf("first arrival = %+v; want pending", res)
	}
	for round := 1; round <= spec[0].Attempts; round++ {
		if err := env.ParentReview.Store.RecordVerdict(parentreview.VerdictReject, review); err != nil {
			t.Fatalf("RecordVerdict() error = %v", err)
		}
		res := eval()
		if res.Passed || res.Pending {
			t.Fatalf("reject %d = %+v; want a failure", round, res)
		}
		if wantTerminal := round == spec[0].Attempts; res.Terminal != wantTerminal {
			t.Fatalf("reject %d Terminal = %v; want %v", round, res.Terminal, wantTerminal)
		}
		if round < spec[0].Attempts {
			if res := eval(); !res.Pending {
				t.Fatalf("rewrite after reject %d = %+v; want pending", round, res)
			}
		}
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

// TestResolveGateSpec_ParentReviewPassOnCapLetsRewriteThrough asserts "pass_on_cap: true" on parent-review reaches the gate, not the engine:
// the entry stays must-pass MayHold, the cap's reject goes back to the writer as a non-terminal failure, and the rewrite after it passes.
func TestResolveGateSpec_ParentReviewPassOnCapLetsRewriteThrough(t *testing.T) {
	env := parentReviewEnv(t)
	cfg := Config{"gates": []any{map[string]any{"name": "parent-review", "attempts": 1, "pass_on_cap": true}}}
	spec, err := resolveGateSpec("Row", cfg, env)
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 1 || spec[0].PassOnCap || !spec[0].MayHold || spec[0].Attempts != 1 {
		t.Fatalf("resolveGateSpec() = %+v; want one must-pass MayHold entry with Attempts 1", spec)
	}
	review := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(review, []byte("out of scope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res, err := spec[0].Gate(); err != nil || !res.Pending {
		t.Fatalf("first arrival = %+v, %v; want pending", res, err)
	}
	if err := env.ParentReview.Store.RecordVerdict(parentreview.VerdictReject, review); err != nil {
		t.Fatalf("RecordVerdict() error = %v", err)
	}
	if res, err := spec[0].Gate(); err != nil || res.Passed || res.Pending || res.Terminal {
		t.Fatalf("reject = %+v, %v; want a non-terminal failure", res, err)
	}
	if res, err := spec[0].Gate(); err != nil || !res.Passed {
		t.Fatalf("rewrite after the reject = %+v, %v; want passed", res, err)
	}
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
			_, err := resolveGateSpec("Row", parentReviewCfg(1), env)
			assertErrContains(t, err, tt.field)
		})
	}
}

func TestResolveGateSpec_UnknownNameListsSixValues(t *testing.T) {
	_, err := resolveGateSpec("Row", gatesCfg("bogus", 1), newTestEnv(t))
	for _, want := range []string{"discussion", "plan", "rework-plan", "description", "verify", "parent-review"} {
		assertErrContains(t, err, want)
	}
}

func TestResolveGateSpec_VerifyRequiresAbsoluteEnvRoots(t *testing.T) {
	for _, field := range []string{"AnchorPath", "WorktreeRoot", "VerifyDir"} {
		t.Run(field, func(t *testing.T) {
			env := newTestEnv(t)
			switch field {
			case "AnchorPath":
				env.AnchorPath = ""
			case "WorktreeRoot":
				env.WorktreeRoot = "relative/worktree"
			case "VerifyDir":
				env.VerifyDir = ""
			}
			_, err := resolveGateSpec("Row", gatesCfg("verify", 3), env)
			assertErrContains(t, err, field)
		})
	}

	spec, err := resolveGateSpec("Row", gatesCfg("verify", 3), newTestEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 1 || spec[0].Name != "verify" || spec[0].Attempts != 3 || spec[0].Gate == nil || spec[0].PassOnCap {
		t.Errorf("resolveGateSpec() = %+v; want one must-pass verify entry with 3 attempts", spec)
	}
}
