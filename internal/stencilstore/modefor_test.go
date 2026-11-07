// modefor_test.go pins the truth table of ModeFor, which maps a build's channel and stamp to a Mode.

package stencilstore

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

func TestModeFor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name                        string
		dev, production, cleanStamp bool
		want                        Mode
	}{
		{"production and clean", false, true, true, ModeProduction},
		{"production but not clean", false, true, false, ModeUnstamped},
		{"production not clean, also dev", true, true, false, ModeDev},
		{"dev", true, false, false, ModeDev},
		{"dev with clean stamp", true, false, true, ModeDev},
		{"neither channel, clean", false, false, true, ModeUnstamped},
		{"neither channel, not clean", false, false, false, ModeUnstamped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ModeFor(tt.dev, tt.production, tt.cleanStamp); got != tt.want {
				t.Errorf("ModeFor(%v, %v, %v) = %v; want %v", tt.dev, tt.production, tt.cleanStamp, got, tt.want)
			}
		})
	}

	if got := Mode(0); got != ModeUnstamped {
		t.Errorf("zero Mode = %v; want ModeUnstamped", got)
	}
}
