// configreg_test.go — tests for the module registry.

package configreg

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/weftengine"
)

func TestNames(t *testing.T) {
	got := Names()
	want := []string{"board", "mux", "shuttle", "warp", "weft"}
	if len(got) != len(want) {
		t.Errorf("Names() = %v; want %v", got, want)
		return
	}
	for i, name := range got {
		if name != want[i] {
			t.Errorf("Names()[%d] = %q; want %q", i, name, want[i])
		}
	}
}

func TestTemplate_Found(t *testing.T) {
	got, ok := Template("weft")
	if !ok {
		t.Error("Template(\"weft\") = _, false; want _, true")
		return
	}
	if got == nil {
		t.Error("Template(\"weft\") returned nil function; want non-nil")
		return
	}
	// Verify the template function returns the expected content.
	want := weftengine.ConfigTemplate()
	if got() != want {
		t.Errorf("Template(\"weft\")() = %q; want %q", got(), want)
	}
}

func TestTemplate_NotFound(t *testing.T) {
	_, ok := Template("nope")
	if ok {
		t.Error("Template(\"nope\") = _, true; want _, false")
	}
}
