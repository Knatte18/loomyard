// seed_test.go covers seed.go's two testable workers: writeSeed and parseSeedParams. Both perform
// no lyxcwd.Resolve and no cwd read of their own (writeSeed's own doc comment), so this file drives
// them directly against a hand-built *lyxcwd.Location, with no real git repository behind it, and
// stays Tier 1.

package shedcli

import (
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestWriteSeed_SucceedsWithNoSeedPresent covers the pre-run exemption first: "lyx shed seed <run-id> --recipe <name>" succeeds with no seed present and no recipe armed -- writeSeed itself never calls Arm at all, structurally, and this is the case resolvePersistentPreRun would otherwise refuse three different ways (no seed to read, an unresolvable recipe, and a verb gate with nothing to gate).
// The seed reads back carrying the recipe and the driver: the default when none is passed, the typed llm driver otherwise.
func TestWriteSeed_SucceedsWithNoSeedPresent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		driver     string
		wantDriver string
	}{
		{"default driver", "", shedrun.DriverLLM},
		{"typed llm driver", shedrun.DriverLLM, shedrun.DriverLLM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}

			if _, found, err := shedrun.ReadSeed(loc, "some-slug"); err != nil || found {
				t.Fatalf("precondition: ReadSeed = (found=%v, err=%v); want (false, nil)", found, err)
			}

			// loom, not batten: batten's own seed-location rule reaches a git worktree listing, which a
			// hand-built Location cannot answer and this untagged suite may not spawn.
			if _, err := writeSeed(loc, "some-slug", "loom", tt.driver, nil); err != nil {
				t.Fatalf("writeSeed = %v; want nil", err)
			}

			seed, found, err := shedrun.ReadSeed(loc, "some-slug")
			if err != nil || !found {
				t.Fatalf("ReadSeed after writeSeed = (found=%v, err=%v); want (true, nil)", found, err)
			}
			if seed.Recipe != "loom" {
				t.Errorf("seed.Recipe = %q; want %q", seed.Recipe, "loom")
			}
			if seed.Driver != tt.wantDriver {
				t.Errorf("seed.Driver = %q; want %q", seed.Driver, tt.wantDriver)
			}
		})
	}
}

// TestResolveSeedDriver pins the driver default against the recipe's bootstrap-verb capability: an empty flag defaults to llm where a bootstrap verb can boot the driver session and to go where none can, and a typed value passes through unchanged either way.
//
//testtiming:keep pins the driver default and pass-through per bootstrap-verb capability, which its covering tests do not
func TestResolveSeedDriver(t *testing.T) {
	tests := []struct {
		name             string
		flagVal          string
		hasBootstrapVerb bool
		want             string
	}{
		{"EmptyWithBootstrapVerbIsLLM", "", true, shedrun.DriverLLM},
		{"EmptyWithoutBootstrapVerbIsGo", "", false, shedrun.DriverGo},
		{"TypedGoPassesThrough", shedrun.DriverGo, true, shedrun.DriverGo},
		{"TypedLLMPassesThrough", shedrun.DriverLLM, false, shedrun.DriverLLM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSeedDriver(tt.flagVal, tt.hasBootstrapVerb); got != tt.want {
				t.Errorf("resolveSeedDriver(%q, %v) = %q; want %q", tt.flagVal, tt.hasBootstrapVerb, got, tt.want)
			}
		})
	}
}

// TestWriteSeed_UnknownRecipeRefusesWithTheAvailableNames asserts an unresolvable --recipe value
// refuses via this package's own lookup, naming the available recipes.
func TestWriteSeed_UnknownRecipeRefusesWithTheAvailableNames(t *testing.T) {
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}

	_, err := writeSeed(loc, "some-slug", "bogus-recipe", "", nil)
	if err == nil {
		t.Fatal("writeSeed(bogus-recipe) = nil; want a refusal")
	}
	for _, name := range names() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("writeSeed(bogus-recipe) error = %q; want it to name recipe %q", err.Error(), name)
		}
	}
}

// TestWriteSeed_RecipeLocationRuleGatesTheWrite pins the per-recipe location rule as a predicate: a
// table entry whose RefuseSeedAt refuses leaves no seed behind, and one with a nil rule writes --
// so the assertion cannot degenerate into "is it spelled batten". The real batten rule (prime only)
// needs a git worktree listing and is proven at the integration tier.
func TestWriteSeed_RecipeLocationRuleGatesTheWrite(t *testing.T) {
	original := recipes
	t.Cleanup(func() { recipes = original })

	refusal := errors.New("this recipe seeds from prime only")
	recipes = map[string]entry{
		shedrun.RecipeBatten: {Arm: original[shedrun.RecipeBatten].Arm, Verbs: original[shedrun.RecipeBatten].Verbs, RefuseSeedAt: func(*lyxcwd.Location) error { return refusal }},
		shedrun.RecipeLoom:   {Arm: original[shedrun.RecipeLoom].Arm, Verbs: original[shedrun.RecipeLoom].Verbs, BootstrapVerb: "start"},
	}

	t.Run("RefusingRuleWritesNothing", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
		_, err := writeSeed(loc, "some-slug", shedrun.RecipeBatten, "", nil)
		if !errors.Is(err, refusal) {
			t.Fatalf("writeSeed(batten) = %v; want the recipe's own refusal", err)
		}
		if _, found, readErr := shedrun.ReadSeed(loc, "some-slug"); readErr != nil || found {
			t.Errorf("ReadSeed after a refused write = (found=%v, err=%v); want (false, nil): a refused seed must leave nothing behind", found, readErr)
		}
	})

	t.Run("NilRuleWrites", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
		if _, err := writeSeed(loc, "some-slug", shedrun.RecipeLoom, "", nil); err != nil {
			t.Fatalf("writeSeed(loom) = %v; want nil", err)
		}
	})
}

// TestWriteSeed_LLMDriverGatedOnBootstrapVerbCapability pins the capability predicate itself, not
// the recipe name: a test-local table entry with an empty bootstrap verb refuses the llm driver,
// and one with a non-empty verb accepts it, so this assertion cannot silently degenerate into "is
// it spelled loom".
func TestWriteSeed_LLMDriverGatedOnBootstrapVerbCapability(t *testing.T) {
	original := recipes
	t.Cleanup(func() { recipes = original })

	// The table entries are keyed by shedrun's own closed recipe vocabulary -- RecipeBatten and
	// RecipeLoom -- because shedrun.WriteSeed validates the recipe name itself, on top of this
	// package's own lookup; a test-local recipe name outside that vocabulary would refuse there
	// instead of exercising the capability predicate this test targets.
	recipes = map[string]entry{
		shedrun.RecipeBatten: {Arm: original[shedrun.RecipeBatten].Arm, Verbs: original[shedrun.RecipeBatten].Verbs, BootstrapVerb: ""},
		shedrun.RecipeLoom:   {Arm: original[shedrun.RecipeLoom].Arm, Verbs: original[shedrun.RecipeLoom].Verbs, BootstrapVerb: "start"},
	}

	t.Run("EmptyBootstrapVerbRefuses", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
		_, err := writeSeed(loc, "some-slug", shedrun.RecipeBatten, shedrun.DriverLLM, nil)
		if err == nil {
			t.Fatal("writeSeed(driver=llm) = nil; want a refusal naming the missing bootstrap verb")
		}
		if !strings.Contains(err.Error(), "no bootstrap verb") {
			t.Errorf("writeSeed(driver=llm) error = %q; want it to name the missing bootstrap verb", err.Error())
		}
		if !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "--driver go") {
			t.Errorf("writeSeed(driver=llm) error = %q; want a way forward naming --driver go", err.Error())
		}
		if _, err := writeSeed(loc, "some-slug", shedrun.RecipeBatten, shedrun.DriverGo, nil); err != nil {
			t.Errorf("writeSeed(driver=go) after the refusal = %v; want nil", err)
		}
	})

	t.Run("NonEmptyBootstrapVerbAccepts", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
		if _, err := writeSeed(loc, "some-slug", shedrun.RecipeLoom, shedrun.DriverLLM, nil); err != nil {
			t.Fatalf("writeSeed(driver=llm) = %v; want nil", err)
		}
	})

	t.Run("EmptyBootstrapVerbDefaultsToGo", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
		driver, err := writeSeed(loc, "some-slug", shedrun.RecipeBatten, "", nil)
		if err != nil {
			t.Fatalf("writeSeed(no driver) = %v; want nil", err)
		}
		if driver != shedrun.DriverGo {
			t.Errorf("writeSeed(no driver) recorded %q; want %q for a recipe with no bootstrap verb", driver, shedrun.DriverGo)
		}
	})
}

// TestParseSeedParams asserts a well-formed set of "key=value" entries parses into the expected map, including a value containing its own "=" (split only on the first), and that an entry with no "=" or an empty key refuses.
func TestParseSeedParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     []string
		want    map[string]string
		wantErr bool
	}{
		{"well-formed entries", []string{"slug=some-slug", "child_driver=go", "extra=a=b"}, map[string]string{"slug": "some-slug", "child_driver": "go", "extra": "a=b"}, false},
		{"NoEquals", []string{"no-equals-sign"}, nil, true},
		{"EmptyKey", []string{"=value"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSeedParams(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseSeedParams(%v) error = %v; want error = %v", tt.raw, err, tt.wantErr)
			}
			if !tt.wantErr && !maps.Equal(got, tt.want) {
				t.Errorf("parseSeedParams(%v) = %v; want exactly %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestWriteSeed_IdempotentAgainstAnIdenticalSeedAndRefusesADisagreeingOne asserts calling writeSeed twice with the identical seed is a no-op the second time, and that it refuses once an existing seed disagrees with the incoming one.
func TestWriteSeed_IdempotentAgainstAnIdenticalSeedAndRefusesADisagreeingOne(t *testing.T) {
	t.Parallel()
	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	params := map[string]string{"slug": "some-slug"}

	if _, err := writeSeed(loc, "some-slug", "loom", shedrun.DriverGo, params); err != nil {
		t.Fatalf("writeSeed (first) = %v; want nil", err)
	}
	if _, err := writeSeed(loc, "some-slug", "loom", shedrun.DriverGo, params); err != nil {
		t.Fatalf("writeSeed (second, identical) = %v; want nil -- idempotent", err)
	}
	if _, err := writeSeed(loc, "some-slug", "loom", shedrun.DriverGo, map[string]string{"parent": "main"}); err == nil {
		t.Fatal("writeSeed (disagreeing params) = nil; want a refusal")
	}
}

// TestWriteSeed_RefusesFabricsOwnCheckouts asserts seed refuses the Board checkout and a pair's
// other side before writing anything, the same refusal every other shed verb over a batten seed
// already applies, so a stray seed can never land in a checkout no run is driven from.
func TestWriteSeed_RefusesFabricsOwnCheckouts(t *testing.T) {
	hub := t.TempDir()
	siblingName := filepath.Base(fabricengine.WeftWorktree(&lyxcwd.Location{HubPath: hub, WorktreeName: "pair", AnchorRel: "."}))
	tests := []struct {
		name         string
		worktreeName string
	}{
		{name: "BoardCheckout", worktreeName: "_board"},
		{name: "PairSibling", worktreeName: siblingName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: hub, WorktreeName: tt.worktreeName, AnchorRel: "."}
			_, err := writeSeed(loc, "a-run", shedrun.RecipeLoom, "", nil)
			if err == nil {
				t.Fatalf("writeSeed(%q) error = nil; want a refusal", tt.worktreeName)
			}
			if !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lyx shed seed") {
				t.Errorf("writeSeed(%q) error = %q; want a way forward naming lyx shed seed", tt.worktreeName, err.Error())
			}
			if _, found, readErr := shedrun.ReadSeed(loc, "a-run"); readErr != nil || found {
				t.Errorf("ReadSeed after a refused writeSeed = found %v, err %v; want no seed written", found, readErr)
			}
		})
	}
}
