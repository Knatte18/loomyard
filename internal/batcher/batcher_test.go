// batcher_test.go covers Select against the package's real, package-init-populated
// registry (identity.go's init() has already run by the time these tests execute,
// unlike registry_test.go's isolated-registry probes). Tier-1 (pure logic, no git,
// no TestMain), per the go-test-tiers-and-hermetic-git Shared Decision.

package batcher

import "testing"

func TestSelect_EmptyNameResolvesToIdentity(t *testing.T) {
	got, err := Select("")
	if err != nil {
		t.Fatalf("Select(\"\") returned error %v; want nil", err)
	}
	if got.Name() != DefaultName {
		t.Errorf("Select(\"\").Name() = %q; want %q", got.Name(), DefaultName)
	}
}

func TestSelect_IdentityByName(t *testing.T) {
	got, err := Select("identity")
	if err != nil {
		t.Fatalf("Select(%q) returned error %v; want nil", "identity", err)
	}
	if got.Name() != "identity" {
		t.Errorf("Select(%q).Name() = %q; want %q", "identity", got.Name(), "identity")
	}
}

func TestSelect_UnknownNameReturnsError(t *testing.T) {
	_, err := Select("does-not-exist")
	if err == nil {
		t.Fatalf("Select(%q) returned nil error; want a batcher: error naming the unknown key", "does-not-exist")
	}
}
