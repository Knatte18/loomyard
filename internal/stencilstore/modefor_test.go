// modefor_test.go pins ModeFor's truth table: it is the only place a build's channel and stamp become a Mode.

package stencilstore

import "testing"

func TestModeFor(t *testing.T) {
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
