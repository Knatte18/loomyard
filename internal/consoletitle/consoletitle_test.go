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
		name                                           string
		shortname, projectHandle, ownHandle, titleName string
		want                                           string
	}{
		{"EveryPart", "ly", "P3", "T7", "gate-shard-slow-package", "ly: P3: T7: gate-shard-slow-package"},
		{"NoProject", "ly", "", "T12", "adopt-fix-root-t1", "ly: T12: adopt-fix-root-t1"},
		{"ALabelAfterAProject", "mo", "P2", "", consoletitle.OrchLabel, "mo: P2: orch"},
		{"AnUpperCaseShortnameIsLowered", "LY", "", "", consoletitle.PrimeLabel, "ly: prime"},
		{"NoShortname", "", "P3", "T7", "slug", "P3: T7: slug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := consoletitle.Title(tt.shortname, tt.projectHandle, tt.ownHandle, tt.titleName); got != tt.want {
				t.Errorf("Title(%q, %q, %q, %q) = %q; want %q", tt.shortname, tt.projectHandle, tt.ownHandle, tt.titleName, got, tt.want)
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
		{"AShortnameOpensTheTitle", "LY", "loomyard", "ly: loomyard workspace"},
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
