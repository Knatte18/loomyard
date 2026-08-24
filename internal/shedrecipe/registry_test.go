// registry_test.go covers Lookup and Names.

package shedrecipe

import (
	"sort"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	t.Run("EveryRegisteredNameResolves", func(t *testing.T) {
		for _, name := range Names() {
			c, err := Lookup(name)
			if err != nil {
				t.Errorf("Lookup(%q) error = %v; want nil", name, err)
			}
			if c == nil {
				t.Errorf("Lookup(%q) = nil Constructor; want non-nil", name)
			}
		}
	})

	t.Run("UnknownName", func(t *testing.T) {
		_, err := Lookup("NoSuchEngine")
		if err == nil {
			t.Fatalf("Lookup() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), "NoSuchEngine") {
			t.Errorf("Lookup() error = %v; want it to name the offending string %q", err, "NoSuchEngine")
		}
	})

	t.Run("EmptyName", func(t *testing.T) {
		_, err := Lookup("")
		if err == nil {
			t.Fatalf("Lookup() error = nil; want non-nil")
		}
		_, unknownErr := Lookup("NoSuchEngine")
		if err.Error() == unknownErr.Error() {
			t.Errorf("Lookup(\"\") error = %v; want a message distinct from the unknown-name error %v, not a silent default", err, unknownErr)
		}
	})
}

func TestNames(t *testing.T) {
	t.Run("Sorted", func(t *testing.T) {
		names := Names()
		if !sort.StringsAreSorted(names) {
			t.Errorf("Names() = %v; want sorted", names)
		}
	})

	t.Run("MatchesRegistryKeys", func(t *testing.T) {
		names := Names()
		if len(names) != len(registry) {
			t.Fatalf("Names() returned %d names; want %d (len(registry))", len(names), len(registry))
		}
		for _, name := range names {
			if _, ok := registry[name]; !ok {
				t.Errorf("Names() returned %q, which is not a key of registry", name)
			}
		}
	})

	t.Run("MutatingResultDoesNotAffectLaterCalls", func(t *testing.T) {
		first := Names()
		if len(first) == 0 {
			t.Fatalf("Names() returned no names; cannot exercise the mutation case")
		}
		original := first[0]
		first[0] = "mutated-name-that-should-not-stick"

		second := Names()
		if second[0] != original {
			t.Errorf("Names() second call = %v; want first element %q unaffected by the first call's caller mutating its result", second, original)
		}
	})
}

// TestRegistry_ShipsFourteenEntries asserts Names() returns exactly the sorted fourteen engine
// names this task's registry ships. What is unique to this test relative to TestNames above is the
// exact-contents pin -- TestNames covers Names()<->registry key agreement and sortedness alone -- and
// the pin belongs beside the registry rather than with any one consumer of it.
func TestRegistry_ShipsFourteenEntries(t *testing.T) {
	want := []string{
		"Batchifier",
		"Bouncer",
		"BurlerRound",
		"DiscussionValidate",
		"DiscussionWrite",
		"Finalize",
		"LoomPreflight",
		"PlanValidate",
		"PlanWrite",
		"Preflight",
		"Publish",
		"SingleLLM",
		"Stub",
		"Webster",
	}

	got := Names()
	if len(got) != len(want) {
		t.Fatalf("Names() = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], name)
		}
	}
}
