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
