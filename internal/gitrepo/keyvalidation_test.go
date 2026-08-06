// keyvalidation_test.go covers validSHA — pure string-matching logic with no git spawn, no
// filesystem I/O, and no lyxtest fixture.
// It is deliberately untagged (no //go:build constraint) and in the internal package so it reaches
// the unexported validator directly, keeping it in Tier 1 alongside plain `go test` per the Test
// Tier Purity Invariant.

package gitrepo

import "testing"

func TestValidSHA(t *testing.T) {
	tests := []struct {
		name string
		sha  string
		want bool
	}{
		{"FullSHA1", "0123456789abcdef0123456789abcdef01234567", true},
		{"FullSHA256", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"UppercaseHex", "ABCDEF01", true},
		{"MinimumAbbreviation", "0a1b", true},
		{"TooShort", "abc", false},
		{"TooLong", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0", false},
		{"Empty", "", false},
		{"UpdateRefDeleteFlag", "-d", false},
		{"LongOption", "--help", false},
		{"OrderfileOption", "-O/dev/null", false},
		{"SymbolicRevision", "HEAD", false},
		{"NonHexCharacter", "0123456g", false},
		{"EmbeddedRange", "abcd..ef01", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validSHA(tt.sha); got != tt.want {
				t.Errorf("validSHA(%q) = %v; want %v", tt.sha, got, tt.want)
			}
		})
	}
}
