// buildinfo_test.go pins IsDev's exact-match semantics: only the literal "dev" value of Channel
// classifies as a dev build, never a prefix or case-insensitive match.

package buildinfo

import "testing"

func TestIsDev(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		want    bool
	}{
		{"empty", "", false},
		{"dev", "dev", true},
		{"prod", "prod", false},
		{"capitalized_Dev", "Dev", false},
		{"uppercase_DEV", "DEV", false},
		{"leading_space", " dev", false},
		{"trailing_space", "dev ", false},
		{"development_prefix", "development", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := Channel
			t.Cleanup(func() { Channel = previous })

			Channel = tt.channel
			if got := IsDev(); got != tt.want {
				t.Errorf("IsDev() with Channel = %q = %v; want %v", tt.channel, got, tt.want)
			}
		})
	}
}
