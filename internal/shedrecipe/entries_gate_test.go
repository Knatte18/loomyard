//go:build tier0

package shedrecipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// reworkTestEnv returns newTestEnv(t) with every Rework seam filled.
func reworkTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	dir := t.TempDir()
	env.Rework = loomshed.PRReworkDeps{
		PlanDir:        filepath.Join(dir, "plan"),
		ReworkDir:      filepath.Join(dir, "rework"),
		ReworkDirRel:   "_lym/loom/rework",
		ReviewsDir:     filepath.Join(dir, "reviews"),
		ReadCommitted:  func(string) ([]byte, bool, error) { return nil, false, nil },
		ArchiveWebster: func(string) error { return nil },
	}
	return env
}

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
		"DarnWrite":       darnWriteEntry,
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
	for _, name := range []string{"discussion", "plan", "description"} {
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

	// The plan gate checks a symbol Rename against the env's rename tools, so a site passing a zero PlanContext fails here.
	for _, tt := range []struct {
		name  string
		tools []string
		want  bool
	}{
		{"no rename tool", nil, true},
		{"a rename tool", []string{"gopls rename"}, false},
	} {
		t.Run("plan with "+tt.name, func(t *testing.T) {
			env := reworkTestEnv(t)
			env.PlanIndex = noFindingsPlanIndex{}
			env.RenameTools = tt.tools
			plan := symbolRenamePlan()
			plankit.Write(t, planparser.PlanDir(env.AnchorPath), plan)

			spec, err := resolveGateSpec("Row", gatesCfg("plan", 4), env)
			if err != nil {
				t.Fatalf("resolveGateSpec() error = %v; want nil", err)
			}
			result, err := spec[0].Gate()
			if err != nil {
				t.Fatalf("Gate() error = %v; want nil", err)
			}
			if got := strings.Contains(result.Findings, "rename-tool-missing"); got != tt.want {
				t.Errorf("Gate() = %+v; want rename-tool-missing: %v", result, tt.want)
			}
		})
	}

	// The "verify" gate reads the call of the row's own gate key: its name in lower case, else the "gate_key" the row names.
	for _, tt := range []struct {
		name    string
		gateKey any
		want    string
	}{
		{"verify of a row named for its key", nil, "code-burler-fix"},
		{"verify of a row naming its key", "custom", "custom"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			var asked []string
			env.VerifyCommand = func(gateKey string) (string, error) {
				asked = append(asked, gateKey)
				return "", nil
			}
			cfg := gatesCfg("verify", 3)
			if tt.gateKey != nil {
				cfg["gate_key"] = tt.gateKey
			}
			spec, err := resolveRowGateSpec("BurlerFix", "Code-Burler-Fix", cfg, env)
			if err != nil {
				t.Fatalf("resolveRowGateSpec() error = %v; want nil", err)
			}
			if _, err := spec[0].Gate(); err != nil {
				t.Fatalf("Gate() error = %v; want nil", err)
			}
			if len(asked) != 1 || asked[0] != tt.want {
				t.Errorf("VerifyCommand asked for %q; want one ask for %q", asked, tt.want)
			}
		})
	}

	// A history seam selects the darn form of the "discussion" gate: it fails on an entry the record dropped and commits only on a pass.
	t.Run("discussion with a history seam", func(t *testing.T) {
		const entry = "\n### Added after Discussion (parent, 2026-03-04): Keep the cache\n\nDecision: d.\n\nRationale: r.\n"
		record := "## Goal\n\ng\n\n## Scope\n\ns\n\n## Decisions\n\nd\n\n## Constraints\n\nc\n\n## Auto-mode assumptions\n\na\n\n## Open risks\n\no\n\n## Acceptance criteria\n\nac\n"

		env := newTestEnv(t)
		for path, content := range map[string]string{env.DecisionRecordPath: record, env.SupportLogPath: "support log"} {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) = %v", path, err)
			}
		}
		var history [][]byte
		commits := 0
		env.DiscussionHistory = func() ([][]byte, error) { return history, nil }
		env.CommitDiscussion = func() error { commits++; return nil }

		spec, err := resolveGateSpec("Row", gatesCfg("discussion", 4), env)
		if err != nil {
			t.Fatalf("resolveGateSpec() error = %v; want nil", err)
		}

		history = [][]byte{[]byte(record + entry)}
		result, err := spec[0].Gate()
		if err != nil || result.Passed || !strings.Contains(result.Findings, "### Added after Discussion (parent, 2026-03-04): Keep the cache") || commits != 0 {
			t.Fatalf("Gate() with a dropped entry = (%+v, %v) after %d commits; want a failure naming the heading and no commit", result, err, commits)
		}

		history = nil
		result, err = spec[0].Gate()
		if err != nil || !result.Passed || commits != 1 {
			t.Errorf("Gate() with an empty history = (%+v, %v) after %d commits; want a pass and one commit", result, err, commits)
		}
	})

	// The loom form of the "discussion" gate checks the relevant list, so a missing path fails loud at resolve time.
	t.Run("discussion without a relevant path", func(t *testing.T) {
		env := newTestEnv(t)
		env.RelevantPath = ""
		_, err := resolveGateSpec("Row", gatesCfg("discussion", 4), env)
		assertErrContains(t, err, "RelevantPath")
	})
}

func TestResolveGateSpec_PlanGatesRequirePlanIndex(t *testing.T) {
	t.Parallel()

	env := reworkTestEnv(t)
	env.PlanIndex = nil
	_, err := resolveGateSpec("Row", gatesCfg("plan", 1), env)
	assertErrContains(t, err, "PlanIndex")
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
		{"GateKeyNotAString", Config{"gates": []any{map[string]any{"name": "verify", "attempts": 1}}, "gate_key": 3}, "gate_key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveGateSpec("TheRow", tt.cfg, newTestEnv(t))
			assertErrContains(t, err, "TheRow")
			assertErrContains(t, err, tt.want)
			if tt.name == "GateKeyNotAString" {
				assertErrContains(t, err, "way forward:")
			}
		})
	}
}

func TestGateCapableEntries_RejectLeftoverOldKeys(t *testing.T) {
	for name, entry := range gateCapableEntries() {
		for _, key := range []string{"gate", "gate_attempts"} {
			t.Run(name+"/"+key, func(t *testing.T) {
				_, err := entry(reviewRow("Row"), Config{key: "plan"}, reworkTestEnv(t))
				assertErrContains(t, err, "unrecognized config key")
				assertErrContains(t, err, key)
			})
		}
	}

	for _, key := range []string{"gate", "gate_attempts"} {
		t.Run("BurlerFix/"+key, func(t *testing.T) {
			cfg := minimalFixConfig()
			cfg[key] = "plan"
			_, err := burlerFixEntry(reviewRow("Row"), cfg, newTestEnv(t))
			assertErrContains(t, err, "unrecognized config key")
			assertErrContains(t, err, key)
		})
	}
}

func TestResolveGateSpec_OtherNamesCarryNoFinal(t *testing.T) {
	env := reworkTestEnv(t)
	env.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
	for _, name := range []string{"discussion", "plan", "description"} {
		spec, err := resolveGateSpec("Row", gatesCfg(name, 1), env)
		if err != nil {
			t.Fatalf("%s: resolveGateSpec() error = %v", name, err)
		}
		if spec[0].Final != nil || spec[0].Expired != nil {
			t.Errorf("%s: Final or Expired is set; want both nil", name)
		}
	}
}

// The retired rework-plan and parent-review names are unknown names like any other.
func TestResolveGateSpec_UnknownNameListsTheLegalValues(t *testing.T) {
	for _, unknown := range []string{"bogus", "rework-plan", "parent-review"} {
		t.Run(unknown, func(t *testing.T) {
			_, err := resolveGateSpec("Row", gatesCfg(unknown, 1), newTestEnv(t))
			for _, want := range []string{"discussion", "plan", "description", "verify"} {
				assertErrContains(t, err, want)
			}
		})
	}
}

func TestResolveGateSpec_VerifyRequiresAbsoluteEnvRoots(t *testing.T) {
	for _, field := range []string{"VerifyCommand", "WorktreeRoot", "VerifyDir"} {
		t.Run(field, func(t *testing.T) {
			env := newTestEnv(t)
			switch field {
			case "VerifyCommand":
				env.VerifyCommand = nil
			case "WorktreeRoot":
				env.WorktreeRoot = "relative/worktree"
			case "VerifyDir":
				env.VerifyDir = ""
			}
			_, err := resolveGateSpec("Row", gatesCfg("verify", 3), env)
			assertErrContains(t, err, field)
		})
	}

	// newTestEnv tells no VerifyMergeBase, so this resolution also pins that the seam is optional.
	spec, err := resolveGateSpec("Row", gatesCfg("verify", 3), newTestEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 1 || spec[0].Name != "verify" || spec[0].Attempts != 3 || spec[0].Gate == nil || spec[0].PassOnCap {
		t.Errorf("resolveGateSpec() = %+v; want one must-pass verify entry with 3 attempts", spec)
	}
}
