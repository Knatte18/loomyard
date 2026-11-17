// stencilpins_test.go covers the stencil pins record's write and read.

package shedrun

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestStencilPins_RoundTrip asserts that a run with no record reads as not found,
// and that a written record reads back unchanged.
// It also asserts that the record's anchor-relative path, the one a commit takes, names the file the write made.
func TestStencilPins_RoundTrip(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	l := syntheticLocation(t)

	if _, found, err := ReadStencilPins(l, SelfRunID); err != nil || found {
		t.Fatalf("ReadStencilPins() before any write = found %v, err %v; want found false, nil", found, err)
	}

	want := StencilPins{Stencils: map[string]string{"loom-one": "aaaa", "loom-two": "bbbb"}}
	if err := WriteStencilPins(l, SelfRunID, want); err != nil {
		t.Fatalf("WriteStencilPins() error: %v", err)
	}
	got, found, err := ReadStencilPins(l, SelfRunID)
	if err != nil || !found {
		t.Fatalf("ReadStencilPins() after the write = found %v, err %v; want found true, nil", found, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadStencilPins() = %+v; want %+v", got, want)
	}
	if gotPath, wantPath := filepath.Join(l.AnchorPath(), StencilPinsRel(l, SelfRunID)), StencilPinsFile(l, SelfRunID); gotPath != wantPath {
		t.Errorf("anchor path joined with StencilPinsRel() = %q; want StencilPinsFile() = %q", gotPath, wantPath)
	}
}
