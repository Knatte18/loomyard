// stencilsdir_test.go covers StencilsDir's derivation from BoardDir and lyxdirs.LyxDirName.

package fabricengine

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStencilsDir(t *testing.T) {
	got := StencilsDir("/h")
	want := filepath.Join("/h", "_board", "_lyx", "stencils")
	if got != want {
		t.Errorf("StencilsDir(%q) = %q; want %q", "/h", got, want)
	}
}

func TestStencilsDir_IsChildOfBoardDir(t *testing.T) {
	hub := "/h"
	board := BoardDir(hub)
	got := StencilsDir(hub)

	if !strings.HasPrefix(got, board+string(filepath.Separator)) {
		t.Errorf("StencilsDir(%q) = %q; want it to be a child of BoardDir(%q) = %q", hub, got, hub, board)
	}
}
