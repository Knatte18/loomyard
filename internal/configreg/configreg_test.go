// configreg_test.go — tests for the module registry.

package configreg

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/weftengine"
)

func TestNames(t *testing.T) {
	got := Names()
	want := []string{"board", "builder", "models", "mux", "perch", "shuttle", "warp", "weft"}
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

// TestModules_SeedOnly pins the seed-only flag: today only the "models"
// module carries an open-ended, operator-owned key set, so it is the only
// entry with SeedOnly == true.
func TestModules_SeedOnly(t *testing.T) {
	for _, m := range Modules() {
		want := m.Name == "models"
		if m.SeedOnly != want {
			t.Errorf("Modules(): module %q SeedOnly = %v; want %v", m.Name, m.SeedOnly, want)
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
