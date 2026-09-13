// specsdir_test.go covers SpecsDir's derivation from BoardDir and lyxdirs.LyxDirName, and its
// sibling relationship to StencilsDir.

package fabricengine

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecsDir(t *testing.T) {
	got := SpecsDir("/h")
	want := filepath.Join("/h", "_board", "_lyx", "specs")
	if got != want {
		t.Errorf("SpecsDir(%q) = %q; want %q", "/h", got, want)
	}
}

func TestSpecsDir_IsChildOfBoardDir(t *testing.T) {
	hub := "/h"
	board := BoardDir(hub)
	got := SpecsDir(hub)

	if !strings.HasPrefix(got, board+string(filepath.Separator)) {
		t.Errorf("SpecsDir(%q) = %q; want it to be a child of BoardDir(%q) = %q", hub, got, hub, board)
	}
}

func TestSpecsDir_IsSiblingOfStencilsDir(t *testing.T) {
	hub := "/h"
	specsParent := filepath.Dir(SpecsDir(hub))
	stencilsParent := filepath.Dir(StencilsDir(hub))

	if specsParent != stencilsParent {
		t.Errorf("filepath.Dir(SpecsDir(%q)) = %q; want it to equal filepath.Dir(StencilsDir(%q)) = %q", hub, specsParent, hub, stencilsParent)
	}
	if SpecsDir(hub) == StencilsDir(hub) {
		t.Errorf("SpecsDir(%q) and StencilsDir(%q) must not be equal, got both = %q", hub, hub, SpecsDir(hub))
	}
}
