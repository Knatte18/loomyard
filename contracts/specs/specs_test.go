// specs_test.go pins the specs registry's names, defaults, RelPath placement, and stamp round-trip.
// Unlike contracts/stencils/registry_test.go, this package's registry is deliberately not
// cross-checked against an on-disk tree walk: the specs registry's two entries come from two
// different directories, one of which is not this package's own, so a directory walk here would
// have nothing meaningful to compare against.

package specs

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// TestRegistry_NamesAreStableAndComplete pins Registry().Names() to the exact ordered slice: the
// order is the order `lyx stencil list` prints them in, and the names are what
// stencilstore.RelPath derives the deployed family directory from.
func TestRegistry_NamesAreStableAndComplete(t *testing.T) {
	want := []string{"loom-plan-spec", "loom-plan-card-format"}
	got := Registry().Names()

	if len(got) != len(want) {
		t.Fatalf("Registry().Names() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Registry().Names()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// TestRegistry_DefaultReturnsNonEmptyBytes verifies every registered name resolves to known,
// non-empty default bytes, and that an unregistered name resolves to nil, false.
func TestRegistry_DefaultReturnsNonEmptyBytes(t *testing.T) {
	reg := Registry()

	for _, name := range reg.Names() {
		def, known := reg.Default(name)
		if !known {
			t.Errorf("Registry().Default(%q) = _, false; want true", name)
			continue
		}
		if len(def) == 0 {
			t.Errorf("Registry().Default(%q) = <empty>, true; want non-empty bytes", name)
		}
	}

	if def, known := reg.Default("no-such-spec"); known || def != nil {
		t.Errorf("Registry().Default(%q) = %v, %v; want nil, false", "no-such-spec", def, known)
	}
}

// TestRegistry_RelPathPlacesBothUnderLoomFamily verifies stencilstore.RelPath places both
// registered names under one loom/ family directory -- the assertion that fails loudly if a future
// rename reintroduces a one-file family directory.
func TestRegistry_RelPathPlacesBothUnderLoomFamily(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"loom-plan-spec", "loom/loom-plan-spec.md"},
		{"loom-plan-card-format", "loom/loom-plan-card-format.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stencilstore.RelPath(tt.name)
			if got != tt.want {
				t.Errorf("stencilstore.RelPath(%q) = %q; want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestRegistry_DefaultsRoundTripTheStamp verifies stamping a registered default leaves its body
// hash untouched. Both travelling docs open with a "# Heading" rather than a leading HTML comment,
// so ApplyStamp prepends a fresh one-line banner; this test pins that the prepend leaves the body
// hash untouched, so the deployed copy is never reclassified as edited on the next pass.
func TestRegistry_DefaultsRoundTripTheStamp(t *testing.T) {
	reg := Registry()

	for _, name := range reg.Names() {
		t.Run(name, func(t *testing.T) {
			c, known := reg.Default(name)
			if !known {
				t.Fatalf("Registry().Default(%q) = _, false; want true", name)
			}

			before := stencilstore.BodyHash(c)
			stamped := stencilstore.ApplyStamp(c, before)
			after := stencilstore.BodyHash(stamped)

			if after != before {
				t.Errorf("BodyHash(ApplyStamp(c, BodyHash(c))) = %q; want %q", after, before)
			}
		})
	}
}
