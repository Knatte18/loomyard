// batcher_test.go covers Select against the package's real, package-init-populated registry
// (identity.go's init() has already run by the time these tests execute, unlike registry_test.go's
// isolated-registry probes).
// Tier-1 (pure logic, no git, no TestMain), per the go-test-tiers-and-hermetic-git Shared Decision.

package batcher

import "testing"

func TestSelect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		key      string
		wantName string
		wantErr  bool
	}{
		{"emptyResolvesToDefault", "", DefaultName, false},
		{"identityByName", "identity", "identity", false},
		{"unknownReturnsError", "does-not-exist", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Select(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Select(%q) returned nil error; want a batcher: error naming the unknown key", tt.key)
				}
				return
			}
			if err != nil {
				t.Fatalf("Select(%q) returned error %v; want nil", tt.key, err)
			}
			if got.Name() != tt.wantName {
				t.Errorf("Select(%q).Name() = %q; want %q", tt.key, got.Name(), tt.wantName)
			}
		})
	}
}
