// specsdir_test.go covers SpecsDir's derivation from BoardDir and lyxdirs.LyxDirName, and its
// sibling relationship to StencilsDir.

package fabricengine

import (
	"path/filepath"
	"strings"
	"testing"
)

//testtiming:keep SpecsDir deriving _board/_lyx/specs, being a child of BoardDir and a sibling of StencilsDir; coverage of its blocks by other tests does not show an assertion of this
func TestSpecsDir(t *testing.T) {
	hub := "/h"
	got := SpecsDir(hub)
	want := filepath.Join(hub, "_board", "_lyx", "specs")
	if got != want {
		t.Errorf("SpecsDir(%q) = %q; want %q", hub, got, want)
	}

	board := BoardDir(hub)
	if !strings.HasPrefix(got, board+string(filepath.Separator)) {
		t.Errorf("SpecsDir(%q) = %q; want it to be a child of BoardDir(%q) = %q", hub, got, hub, board)
	}

	specsParent := filepath.Dir(got)
	stencilsParent := filepath.Dir(StencilsDir(hub))
	if specsParent != stencilsParent {
		t.Errorf("filepath.Dir(SpecsDir(%q)) = %q; want it to equal filepath.Dir(StencilsDir(%q)) = %q", hub, specsParent, hub, stencilsParent)
	}
	if got == StencilsDir(hub) {
		t.Errorf("SpecsDir(%q) and StencilsDir(%q) must not be equal, got both = %q", hub, hub, got)
	}
}
