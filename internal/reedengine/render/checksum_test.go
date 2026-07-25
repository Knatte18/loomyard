// checksum_test.go pins a known-good layoutChecksum fixture from live psmux
// testing, asserting that the tmux/psmux layout-checksum algorithm stays
// correct, and also asserts the general four-lowercase-hex-digit shape every
// checksum must have.

package render

import "testing"

func TestLayoutChecksum(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		// Pinned value from live psmux testing, asserting the tmux/psmux
		// layout-checksum algorithm stays proven-correct.
		{"MatchesPsmuxFixture", "220x50,0,0[220x15,0,0,1,220x15,0,16,4,220x18,0,32,3]", "acd7"},
		{"ArbitraryInputIsFourHexDigits", "anything", ""},
		{"EmptyInputIsFourHexDigits", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := layoutChecksum(tt.body)

			// Shape assertion: any input must checksum to exactly four
			// lowercase hex characters, since callers concatenate this
			// directly into the layout string's checksum field.
			if len(got) != 4 {
				t.Errorf("layoutChecksum(%q) = %q, want length 4", tt.body, got)
			}
			for _, c := range got {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("layoutChecksum(%q) = %q, want lowercase hex only", tt.body, got)
				}
			}

			if tt.want != "" && got != tt.want {
				t.Errorf("layoutChecksum(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}
