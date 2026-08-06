// geometry_test.go covers the pure geometry constructor this module still owns --
// weftname.SiblingPath's join shape. BoardDir, HubPath and IsReservedHubName
// relocated to internal/fabricengine in this batch (alongside BoardDirName and
// HubSuffix); their coverage lives in fabricengine/junctionnames_test.go now.

package lyxcwd_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestWeftSiblingPath verifies that weftname.SiblingPath joins hub and slug with weftname.Suffix.
func TestWeftSiblingPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hub  string
		slug string
		want string
	}{
		{
			name: "simple slug",
			hub:  "/h",
			slug: "feat",
			want: filepath.Join("/h", "feat"+weftname.Suffix),
		},
		{
			name: "nested hub",
			hub:  "/repos/loomyard-HUB",
			slug: "main",
			want: filepath.Join("/repos/loomyard-HUB", "main"+weftname.Suffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weftname.SiblingPath(tt.hub, tt.slug)
			if got != tt.want {
				t.Errorf("SiblingPath(%q, %q) = %q; want %q", tt.hub, tt.slug, got, tt.want)
			}
		})
	}
}
