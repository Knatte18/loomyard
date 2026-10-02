// plantemplate_test.go pins loom-template-plan.md's verify-coverage rule:
// the plan-level `## verify:` section must run the tests of every package a card targets, so Plan-Review's matching check has something the writer was told about.

package stencils

import (
	"strings"
	"testing"
)

// TestLoomTemplatePlan_NamesPriorPlanSection asserts the template tells the agent to act on a trailing `Prior plan` section before writing.
func TestLoomTemplatePlan_NamesPriorPlanSection(t *testing.T) {
	const phrase = "ends with a `Prior plan` section"
	if !strings.Contains(string(LoomTemplatePlan), phrase) {
		t.Errorf("LoomTemplatePlan does not contain %q", phrase)
	}
}

// TestLoomTemplatePlan_StatesAttackSurfaceBound asserts the template tells the plan writer to state, in the introducing card's `**Intent:**`, what a new edge or weakened guard can skip or let through and what bounds it.
func TestLoomTemplatePlan_StatesAttackSurfaceBound(t *testing.T) {
	text := string(LoomTemplatePlan)
	for _, phrase := range []string{"skip or let through", "what bounds it"} {
		if !strings.Contains(text, phrase) {
			t.Errorf("LoomTemplatePlan does not contain %q", phrase)
		}
	}
}

// TestLoomTemplatePlan_StatesVerifyCoverage asserts the template tells the plan writer the verify section covers every targeted package, hermetic build-tagged tests included, and compiles rather than runs live-substrate tags.
// Each assertion is a short, distinctive substring rather than a whole paragraph, following discussiontemplate_test.go's precedent, so ordinary prose edits do not break this test.
func TestLoomTemplatePlan_StatesVerifyCoverage(t *testing.T) {
	text := string(LoomTemplatePlan)

	tests := []struct {
		name   string
		phrase string
	}{
		{"covers every targeted package", "must cover every package any card targets"},
		{"includes hermetic build-tagged tests", "hermetic build-tagged tests"},
		{"compiles rather than runs live-substrate tags", "compiled rather than run"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(text, tt.phrase) {
				t.Errorf("LoomTemplatePlan does not contain %q", tt.phrase)
			}
		})
	}
}
