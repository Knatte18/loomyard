// template_test.go pins builder.yaml's seed template: it must parse as
// YAML, round-trip through LoadConfig into the documented defaults, and
// carry every Config yaml tag — so a struct field added without a matching
// template line fails here rather than surfacing as a silent "missing
// keys" error the first time an operator's file omits it.

package builderengine_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
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
