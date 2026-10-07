// registry_test.go covers the unexported register/lookup pair in isolation from package-level init() state: the test builds its own registry map so a probe batcher's registration can never leak into — or be masked by — the real identity registration identity.go's init() performs on package load.
// Tier-1 (pure logic, no git, no TestMain), per the go-test-tiers-and-hermetic-git Shared Decision.

package batcher

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// probeBatcher is a minimal Batcher stand-in for testing register/lookup.
type probeBatcher struct{ name string }

func (p probeBatcher) Batch(*planparser.Plan, []planparser.Card, SizeSource) ([]Batch, error) {
	return nil, nil
}
func (p probeBatcher) Name() string { return p.name }

// withEmptyRegistry temporarily replaces the registry for isolated testing.
func withEmptyRegistry(t *testing.T) {
	t.Helper()
	original := registry
	registry = make(map[string]Batcher)
	t.Cleanup(func() { registry = original })
}

// TestRegister_Lookup_RoundTrip swaps the package-level registry, so it stays serial.
// The parallel tests of this package only resume after every serial test and its cleanup have finished.
func TestRegister_Lookup_RoundTrip(t *testing.T) {
	withEmptyRegistry(t)

	b := probeBatcher{name: "probe"}
	register(b)

	got, ok := lookup("probe")
	if !ok {
		t.Fatalf("lookup(%q) reported not found after register", "probe")
	}
	if got.Name() != "probe" {
		t.Errorf("lookup(%q).Name() = %q; want %q", "probe", got.Name(), "probe")
	}

	if _, ok := lookup("does-not-exist"); ok {
		t.Errorf("lookup(%q) reported found; want not-found for an unregistered name", "does-not-exist")
	}
}
