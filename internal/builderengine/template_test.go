//go:build integration

// template_test.go pins builder.yaml's seed template: it must parse as
// YAML, round-trip through LoadConfig into the documented defaults, and
// carry every Config yaml tag — so a struct field added without a matching
// template line fails here rather than surfacing as a silent "missing
// keys" error the first time an operator's file omits it. It also pins the
// embedded implementer prompt template (implementer-template.md): its
// load-bearing contract statements as substring assertions (the burler
// TestTemplate_StatesRoundDiscipline pattern), and that it actually fills
// through stencil with every required marker.

package builderengine_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/stencil"
	"gopkg.in/yaml.v3"
)

// TestConfigTemplate_ParsesAsYAML asserts the embedded template is
// well-formed YAML on its own, independent of LoadConfig's strict-decode
// path.
func TestConfigTemplate_ParsesAsYAML(t *testing.T) {
	var out map[string]any
	if err := yaml.Unmarshal([]byte(builderengine.ConfigTemplate()), &out); err != nil {
		t.Fatalf("ConfigTemplate() does not parse as YAML: %v", err)
	}
}

// TestConfigTemplate_RoundTripsThroughLoadConfig seeds the template
// verbatim and asserts LoadConfig resolves it into the documented defaults
// — the same expectation config_test.go's
// TestLoadConfig_TemplateDefaultsResolve already exercises via LoadConfig
// directly; this test instead pins the template's raw content against the
// same defaults so a template edit that silently changes a default value
// is caught here too.
func TestConfigTemplate_RoundTripsThroughLoadConfig(t *testing.T) {
	fixture := lyxtest.CopyWeft(t)
	lyxtest.SeedConfig(t, fixture.WeftPath, map[string]string{
		"builder": builderengine.ConfigTemplate(),
	})

	cfg, err := builderengine.LoadConfig(fixture.WeftPath, "builder")
	if err != nil {
		t.Fatalf("LoadConfig(template) = _, %v; want nil error", err)
	}

	want := builderengine.Config{
		Orchestrator:           "sonnet",
		Implementer:            "sonnet",
		ImplementerOversized:   "sonnet",
		Recovery:               "opus[effort=high]",
		SelfFixCap:             2,
		PollWaitS:              480,
		BatchTimeoutMin:        60,
		OrchestratorTimeoutMin: 480,
		BatchContextCapTokens:  100000,
		BatchCardCap:           10,
	}
	if cfg != want {
		t.Errorf("LoadConfig(template) = %+v; want %+v", cfg, want)
	}
}

// TestConfigTemplate_ContainsEveryConfigYAMLTag walks Config's fields via
// reflection and asserts every yaml tag appears in the template text — so a
// struct field added without a matching template line is caught
// mechanically rather than relying on review to notice the gap.
func TestConfigTemplate_ContainsEveryConfigYAMLTag(t *testing.T) {
	text := builderengine.ConfigTemplate()

	typ := reflect.TypeOf(builderengine.Config{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		if tag == "" {
			t.Fatalf("Config field %q has no yaml tag", typ.Field(i).Name)
		}
		if !containsKey(text, tag) {
			t.Errorf("ConfigTemplate() does not contain key %q for field %q", tag, typ.Field(i).Name)
		}
	}
}

// containsKey reports whether text contains a "<key>:" line-start token,
// the shape every one of this template's keys takes.
func containsKey(text, key string) bool {
	needle := key + ":"
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			return true
		}
	}
	return false
}

// implementerTemplateMarkerValues returns a values map with every one of
// ImplementerTemplate's five required top-level markers set to a
// non-empty placeholder, so a test can fill the template cleanly or delete
// one key at a time to prove stencil.Fill's per-marker error.
func implementerTemplateMarkerValues() map[string]string {
	return map[string]string{
		"batch_file":    "/plan/02-list-tests.md",
		"batch_name":    "02-list-tests",
		"report_path":   "/builder/reports/02-list-tests.yaml",
		"self_fix_cap":  "2",
		"worktree_root": "/worktree",
	}
}

// TestImplementerTemplate_StatesBatchDiscipline asserts the embedded
// implementer template's bytes carry the load-bearing batch-discipline
// phrases in prose — the burler TestTemplate_StatesRoundDiscipline pattern
// applied to builder's implementer prompt — so an edit that silently waters
// down the commit-per-card shape, the bounded self-fix cap, the
// report-as-final-action rule, or the never-touch-the-weft rule fails this
// test rather than only a human review.
func TestImplementerTemplate_StatesBatchDiscipline(t *testing.T) {
	text := string(builderengine.ImplementerTemplate())

	// Commit-per-card, in the exact "NN.C: <short what>" subject shape
	// plan-format.md pins.
	requireContains(t, text, "NN.C: <short what>")
	requireContains(t, text, "One commit per card is the norm")

	// The bounded self-fix cap: at most {{.self_fix_cap}} attempts, then
	// stop and report stuck naming both the blocker and what was tried.
	requireContains(t, text, "in-session fix attempts")
	requireContains(t, text, "stop trying and report")
	requireContains(t, text, "names BOTH the blocker")

	// Report-as-final-action, quoting the exact batch-report schema keys
	// plan-format.md pins.
	requireContains(t, text, "final action")
	requireContains(t, text, "batch:")
	requireContains(t, text, "status:")
	requireContains(t, text, "tests:")
	requireContains(t, text, "stuck_reason:")
	requireContains(t, text, "out_of_scope:")

	// Never touch the weft.
	requireContains(t, text, "Never touch the weft")
	requireContains(t, text, "You never run git against the weft repo")

	// plan-format v2: the implementer now reads its batch file AND the
	// overview (framing, Batch Index, Shared Decisions), but still never
	// another batch's own file.
	requireContains(t, text, "plan-format v2")
	requireContains(t, text, "also read\n`00-overview.md`")
	requireContains(t, text, "Never read another\nbatch's own file")

	// The five typed file-op field names a card carries.
	requireContains(t, text, "**Edits:**")
	requireContains(t, text, "**Creates:**")
	requireContains(t, text, "**Deletes:**")
	requireContains(t, text, "**Moves:**")
	requireContains(t, text, "**Context:**")

	// Rename-mechanic compliance: git mv FIRST, then only surgical edits,
	// never rewrite-and-recreate.
	requireContains(t, text, "run\n`git mv <old> <new>` FIRST")
	requireContains(t, text, "never rewrite\nthe relocated file from scratch and delete the original")

	// Commit-subject rule: the card's own **Commit:** value wins verbatim
	// when present; otherwise the NN.C fallback is derived.
	requireContains(t, text, `carries a "**Commit:**" field, use its value verbatim`)
}

// requireContains fails the test, naming the missing needle, if text does
// not contain it. Mirrors burlerengine/template_test.go's helper of the
// same name (package-local — builderengine and burlerengine deliberately
// do not share a test-helper package).
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// orchestratorTemplateMarkerValues returns a values map with every one of
// OrchestratorTemplate's five required top-level markers set to a
// non-empty placeholder, so a test can fill the template cleanly or delete
// one key at a time to prove stencil.Fill's per-marker error.
func orchestratorTemplateMarkerValues() map[string]string {
	return map[string]string{
		"batch_index":  "01 — json-flag — add the --json flag",
		"progress":     "none",
		"outcome_path": "/lyx/builder/outcome.yaml",
		"self_fix_cap": "2",
		"poll_wait_s":  "480",
	}
}

// TestOrchestratorTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds
// when every one of OrchestratorTemplate's five required markers is
// supplied, and fails — naming the marker — when any single one is absent.
func TestOrchestratorTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(builderengine.OrchestratorTemplate(), orchestratorTemplateMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"batch_index", "progress", "outcome_path", "self_fix_cap", "poll_wait_s"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := orchestratorTemplateMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(builderengine.OrchestratorTemplate(), values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestOrchestratorTemplate_NamesTheThreeVerbsItDrives asserts the embedded
// orchestrator template names every one of the three `lyx builder` verbs its
// loop touches: spawn-batch and poll drive the blocking loop itself, and
// status is named even though the template explicitly forbids using it as a
// substitute for that loop.
func TestOrchestratorTemplate_NamesTheThreeVerbsItDrives(t *testing.T) {
	text := string(builderengine.OrchestratorTemplate())

	requireContains(t, text, "lyx builder spawn-batch <NN>")
	requireContains(t, text, "lyx builder poll")
	requireContains(t, text, "lyx builder status")
}

// digestSectionHeading and outcomeKeysHeading name the two headings whose
// bullet lists TestOrchestratorTemplate_QuotesDigestFieldsAndNoOthers and
// TestOrchestratorTemplate_QuotesOutcomeSchemaKeys scope their extraction
// to — never the whole template body, since prose elsewhere legitimately
// backtick-quotes a subset of the same field names (e.g. `stuck_reason`
// names both a digest field and an outcome-file key) without that being an
// "other" field leaking into either pinned set.
const (
	digestSectionHeading  = "## Read ONLY the digest fields — quoted here, exactly"
	outcomeKeysHeadingSub = "It carries exactly these three keys, quoted here, exactly:"
)

// extractBacktickBullets returns, in order, the single backtick-quoted token
// from every "- `token`" bullet line appearing strictly between heading
// (matched by trimmed equality) and the next "## " heading or EOF — the
// shape both the digest-field and outcome-key bullet lists take in
// orchestrator-template.md.
func extractBacktickBullets(text, heading string) []string {
	lines := strings.Split(text, "\n")

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil
	}

	bulletRe := regexp.MustCompile("^-\\s+`([^`]+)`$")
	var tokens []string
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "## ") {
			break
		}
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			tokens = append(tokens, m[1])
		}
	}
	return tokens
}

// TestOrchestratorTemplate_QuotesDigestFieldsAndNoOthers asserts the
// template's digest-field bullet list names exactly the ten pinned digest
// field names (docs/modules/plan-format.md's poll digest contract) — no
// fewer, no extras — the mechanical half of the discussion's "the
// orchestrator reads only distilled digests" decision.
func TestOrchestratorTemplate_QuotesDigestFieldsAndNoOthers(t *testing.T) {
	text := string(builderengine.OrchestratorTemplate())

	want := []string{
		"batch", "status", "tests", "stuck_reason", "out_of_scope",
		"drift_unreported", "files_changed", "dirty", "dead_reason", "elapsed_s",
	}
	got := extractBacktickBullets(text, digestSectionHeading)

	if len(got) != len(want) {
		t.Fatalf("digest field bullets = %v (%d); want %v (%d)", got, len(got), want, len(want))
	}
	for i, field := range want {
		if got[i] != field {
			t.Errorf("digest field bullet %d = %q; want %q", i, got[i], field)
		}
	}
}

// TestOrchestratorTemplate_QuotesOutcomeSchemaKeys asserts the template's
// outcome-file bullet list names exactly the three outcome.yaml schema keys
// the discussion's outcome-contract decision pins, immediately followed by
// the literal yaml block spelling out their values.
func TestOrchestratorTemplate_QuotesOutcomeSchemaKeys(t *testing.T) {
	text := string(builderengine.OrchestratorTemplate())

	requireContains(t, text, outcomeKeysHeadingSub)

	want := []string{"outcome", "stuck_reason", "batches_done"}
	got := extractBacktickBullets(text, outcomeKeysHeadingSub)
	if len(got) != len(want) {
		t.Fatalf("outcome schema key bullets = %v (%d); want %v (%d)", got, len(got), want, len(want))
	}
	for i, key := range want {
		if got[i] != key {
			t.Errorf("outcome schema key bullet %d = %q; want %q", i, got[i], key)
		}
	}

	requireContains(t, text, "outcome: done | stuck | paused")
	requireContains(t, text, `stuck_reason: null | "<one line>"`)
	requireContains(t, text, "batches_done: <int>")
}

// TestOrchestratorTemplate_ForbidsWeftGitAndSelfEditing asserts the embedded
// template's bytes carry the load-bearing never-touch-the-weft and
// never-edit-code-yourself statements in prose, so an edit that silently
// waters down either rule fails this test rather than only a human review —
// the Weft Git Invariant's prompt-template half.
func TestOrchestratorTemplate_ForbidsWeftGitAndSelfEditing(t *testing.T) {
	text := string(builderengine.OrchestratorTemplate())

	requireContains(t, text, "NEVER run any git command against the weft")
	requireContains(t, text, "NEVER edit, create, or delete a target file yourself")
	requireContains(t, text, "NEVER use a `/model` switch")
}

// TestOrchestratorTemplate_StatesBatchOrderAndRecoveryLadder asserts the
// embedded template's bytes carry the strict-ordering rule and every rung of
// the discussion's recovery ladder (dead -> fresh respawn; stuck -> recovery
// role; stuck chain member -> whole-chain restart) in prose.
func TestOrchestratorTemplate_StatesBatchOrderAndRecoveryLadder(t *testing.T) {
	text := string(builderengine.OrchestratorTemplate())

	requireContains(t, text, "Drive it STRICTLY in order")
	requireContains(t, text, "respawn the SAME batch fresh, once")
	requireContains(t, text, "--role recovery")
	requireContains(t, text, "--restart-chain")
	// The in-flight refusal is Go-emitted (ErrBatchInFlight) and the template
	// must keep telling a resumed orchestrator to poll through it, not treat
	// it as a hard error — the co-versioning rule's template half.
	requireContains(t, text, "already in flight")
}

// TestImplementerTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds
// when every one of ImplementerTemplate's five required markers is
// supplied, and fails — naming the marker — when any single one is absent.
func TestImplementerTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(builderengine.ImplementerTemplate(), implementerTemplateMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"batch_file", "batch_name", "report_path", "self_fix_cap", "worktree_root"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := implementerTemplateMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(builderengine.ImplementerTemplate(), values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}
