// consoletitle_test.go pins the title forms Title and Workspace give.

package consoletitle_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/consoletitle"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestTitle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name                                      string
		lead, shortname, projectHandle, titleName string
		want                                      string
	}{
		{"EveryPart", "T2", "ly", "P1", "adopt-worker-record", "T2 [ly:P1] adopt-worker-record"},
		{"AnOrchLead", consoletitle.OrchLead, "ly", "P0", "root", "ORCH [ly:P0] root"},
		{"AnUpperCaseShortnameIsLowered", "T5", "LY", "P0", "machine-name-file", "T5 [ly:P0] machine-name-file"},
		{"NoShortname", "T7", "", "P3", "slug", "T7 [P3] slug"},
		{"NoLeadNorProject", "", "ly", "", "stray", "[ly] stray"},
		{"TheNameAlone", "", "", "", "tapestry", "tapestry"},
		{"NoName", consoletitle.OrchLead, "ly", "P0", "", "ORCH [ly:P0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := consoletitle.Title(tt.lead, tt.shortname, tt.projectHandle, tt.titleName); got != tt.want {
				t.Errorf("Title(%q, %q, %q, %q) = %q; want %q", tt.lead, tt.shortname, tt.projectHandle, tt.titleName, got, tt.want)
			}
		})
	}
}

func TestWorkspace(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name            string
		shortname, base string
		want            string
	}{
		{"AShortnameOpensTheTitle", "LY", "loomyard", "[ly] loomyard workspace"},
		{"NoShortname", "", "bm", "bm workspace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := consoletitle.Workspace(tt.shortname, tt.base); got != tt.want {
				t.Errorf("Workspace(%q, %q) = %q; want %q", tt.shortname, tt.base, got, tt.want)
			}
		})
	}
}
