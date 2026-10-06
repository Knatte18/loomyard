// registry_test.go covers Lookup and Names.

package shedrecipe

import (
	"path/filepath"
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

//testtiming:keep pins the sorted, registry-keyed, non-aliased and exact set of engine names, which its covering test does not
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

	// The exact-contents pin belongs beside the registry rather than with any one consumer of it.
	t.Run("ShipsExpectedEntries", func(t *testing.T) {
		want := []string{
			"Batchifier",
			"Bouncer",
			"BurlerRound",
			"Describe",
			"DiscussionWrite",
			"Finalize",
			"FrictionReflect",
			"InnerRun",
			"LoomPreflight",
			"PRGate",
			"PRRework",
			"PlanWrite",
			"Preflight",
			"Publish",
			"SeedChild",
			"SingleLLM",
			"Stub",
			"Webster",
			"WorktreeCreate",
			"WorktreeTeardown",
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
	})
}

func TestRegistry_DescribeAndDescriptionGate(t *testing.T) {
	if _, err := Lookup("Describe"); err != nil {
		t.Fatalf("Lookup(%q) error = %v; want nil", "Describe", err)
	}
	env := newTestEnv(t)
	env.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
	spec, err := resolveGateSpec("Describe", gatesCfg("description", 3), env)
	if err != nil {
		t.Fatalf("resolveGateSpec(description) error = %v; want nil", err)
	}
	if len(spec) != 1 || spec[0].Gate == nil {
		t.Errorf("resolveGateSpec(description) = %+v; want one entry carrying the description gate", spec)
	}
	_, err = resolveGateSpec("Describe", gatesCfg("bogus", 3), env)
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Errorf("resolveGateSpec(bogus) error = %v; want it to name the description value", err)
	}
}
