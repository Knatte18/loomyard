// plantemplate_test.go pins loom-template-plan.md's verify-coverage rule:
// the plan-level `## verify:` section must run the tests of every package a card targets, so Plan-Review's matching check has something the writer was told about.

package stencils

import (
	"strings"
	"testing"
)

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
