// mouse_test.go exercises mouseOption's validate/normalize contract: valid
// on/off inputs (in every case/whitespace variant) resolve to the canonical
// lowercase form, and every other input — including the empty string —
// errors rather than silently defaulting.

package muxengine

import "testing"

func TestMouseOption(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"on", "on", "on", false},
		{"off", "off", "off", false},
		{"upper_ON", "ON", "on", false},
		{"mixed_Off", "Off", "off", false},
		{"whitespace_on", " on ", "on", false},
		{"invalid_yes", "yes", "", true},
		{"invalid_numeric", "1", "", true},
		{"invalid_garbage", "banana", "", true},
		{"invalid_empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mouseOption(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Errorf("mouseOption(%q) = %q, nil; want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Errorf("mouseOption(%q) = %q, %v; want %q, nil", tt.raw, got, err, tt.want)
				return
			}
			if got != tt.want {
				t.Errorf("mouseOption(%q) = %q; want %q", tt.raw, got, tt.want)
			}
		})
	}
}
