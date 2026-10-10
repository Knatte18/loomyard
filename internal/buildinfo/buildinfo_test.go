// buildinfo_test.go pins the exact-match semantics of IsDev and IsProduction:
// only the literal Channel value classifies, never a prefix or case-insensitive match.

package buildinfo

import "testing"

func TestChannelAccessors(t *testing.T) {
	tests := []struct {
		name           string
		channel        string
		wantDev        bool
		wantProduction bool
	}{
		{"empty", "", false, false},
		{"dev", "dev", true, false},
		{"production", "production", false, true},
		{"prod", "prod", false, false},
		{"capitalized_Dev", "Dev", false, false},
		{"uppercase_DEV", "DEV", false, false},
		{"capitalized_Production", "Production", false, false},
		{"leading_space", " dev", false, false},
		{"trailing_space", "dev ", false, false},
		{"production_trailing_space", "production ", false, false},
		{"development_prefix", "development", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := Channel
			t.Cleanup(func() { Channel = previous })

			Channel = tt.channel
			if got := IsDev(); got != tt.wantDev {
				t.Errorf("IsDev() with Channel = %q = %v; want %v", tt.channel, got, tt.wantDev)
			}
			if got := IsProduction(); got != tt.wantProduction {
				t.Errorf("IsProduction() with Channel = %q = %v; want %v", tt.channel, got, tt.wantProduction)
			}
		})
	}
}
