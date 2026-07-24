// keyvalidation_test.go covers validSnapshotKey — pure string-matching logic
// with no git spawn, no filesystem I/O, and no lyxtest fixture. It is
// deliberately untagged (no //go:build constraint) and in the internal
// package so it reaches unexported validSnapshotKey directly, keeping it in
// Tier 1 alongside plain `go test` per the Test Tier Purity Invariant.

package gitrepo

import "testing"

func TestValidSnapshotKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"SimpleAlphanumeric", "raddle", true},
		{"HyphenatedWord", "codeintel-go", true},
		{"AnotherHyphenatedWord", "codeintel-py", true},
		{"Empty", "", false},
		{"ContainsSpace", "has space", false},
		{"ContainsTilde", "bad~key", false},
		{"ContainsColon", "a:b", false},
		{"ContainsDoubleDot", "a..b", false},
		{"LeadingSlash", "/lead", false},
		{"TrailingSlash", "trail/", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validSnapshotKey(tt.key); got != tt.want {
				t.Errorf("validSnapshotKey(%q) = %v; want %v", tt.key, got, tt.want)
			}
		})
	}
}
