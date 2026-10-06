// stencilsdir_test.go covers StencilsDir's derivation from BoardDir and lyxdirs.LyxDirName.

package fabricengine

import (
	"path/filepath"
	"strings"
	"testing"
)

//testtiming:keep StencilsDir deriving _board/_lyx/stencils and being a child of BoardDir; coverage of its blocks by other tests does not show an assertion of this
func TestStencilsDir(t *testing.T) {
	hub := "/h"
	got := StencilsDir(hub)
	want := filepath.Join(hub, "_board", "_lyx", "stencils")
	if got != want {
		t.Errorf("StencilsDir(%q) = %q; want %q", hub, got, want)
	}

	board := BoardDir(hub)
	if !strings.HasPrefix(got, board+string(filepath.Separator)) {
		t.Errorf("StencilsDir(%q) = %q; want it to be a child of BoardDir(%q) = %q", hub, got, hub, board)
	}
}
