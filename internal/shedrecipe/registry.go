// registry.go implements the registry's public surface: the single map[string]Constructor literal
// every engine name is declared in, plus Lookup and Names, the only two ways a caller reaches it.

package shedrecipe

import (
	"fmt"
	"sort"
)

// registry is the single place every engine name is declared, mapping each recipe row's engine
// name to the Constructor that builds it.
//
// This batch ships the nine value-only entries; batch 4 adds "SingleLLM" and batch 5 adds
// "Bouncer" and "BurlerRound" to this same literal.
//
// init() self-registration was rejected: the entries span four packages
// (internal/shedrecipe, internal/preflightshed, internal/landingshed, internal/loomshed), and
// registration would then depend on link-time blank imports of packages this package already
// imports directly -- an indirection with no benefit here.
var registry = map[string]Constructor{
	"Preflight":          preflightEntry,
	"Publish":            publishEntry,
	"Finalize":           finalizeEntry,
	"LoomPreflight":      loomPreflightEntry,
	"Batchifier":         batchifierEntry,
	"DiscussionValidate": discussionValidateEntry,
	"PlanValidate":       planValidateEntry,
	"Stub":               stubEntry,
	"Webster":            websterEntry,
}

// Lookup resolves name against the registry, returning its Constructor.
//
// An empty name is an error in its own right, not a default: unlike internal/batcher's Select,
// which defaults an empty name to DefaultName, there is no sensible default engine here -- every
// recipe row must name one explicitly.
func Lookup(name string) (Constructor, error) {
	if name == "" {
		return nil, fmt.Errorf("shedrecipe: engine name must not be empty")
	}
	c, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("shedrecipe: unknown engine %q", name)
	}
	return c, nil
}

// Names returns every registered engine name, sorted, in a freshly allocated slice each call --
// mutating the returned slice never affects a later call or the registry itself.
//
// It exists so the coverage guard and the future recipe loader both have a stable enumeration and
// neither reaches into registry directly.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
