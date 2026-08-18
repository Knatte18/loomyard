// modefor_test.go pins ModeFor's two rows: it is the only place a dev bool becomes a Mode.

package stencilstore

import "testing"

func TestModeFor(t *testing.T) {
	if got := ModeFor(true); got != ModeDev {
		t.Errorf("ModeFor(true) = %v; want %v", got, ModeDev)
	}
	if got := ModeFor(false); got != ModeProduction {
		t.Errorf("ModeFor(false) = %v; want %v", got, ModeProduction)
	}
}
