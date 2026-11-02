// exportname_test.go covers CheckExportName over the handles a parts file exports.

package planparser_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricvocab"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCheckExportName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	side := fabricvocab.SideTokens()[0]
	tests := []struct {
		name   string
		handle string
		want   []string
	}{
		{name: "a package-unit handle passes", handle: "plan:internal/x#NewThing"},
		{name: "a handle with no unit", handle: "plan:internal/x", want: []string{"handle-malformed"}},
		{name: "a handle with a file as its unit", handle: "plan:internal/x/x.go#NewThing", want: []string{"handle-malformed"}},
		{name: "a member carrying a fabric side token", handle: "plan:internal/x#" + side + "Thing", want: []string{"fabric-vocabulary-name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			findings := planparser.CheckExportName(tt.handle)
			checks := make([]string, len(findings))
			for i, finding := range findings {
				checks[i] = finding.Check
				if finding.Ref != tt.handle || finding.Card != "" || !strings.Contains(finding.Detail, "way forward:") {
					t.Errorf("CheckExportName(%q) finding = %+v; want Ref %q, no Card and a way forward", tt.handle, finding, tt.handle)
				}
			}
			if !slices.Equal(checks, tt.want) {
				t.Errorf("CheckExportName(%q) checks = %v; want %v", tt.handle, checks, tt.want)
			}
		})
	}
}
