// registry.go implements the package's name-keyed Batcher registry: library
// members self-register via their own init() (see identity.go), and webster
// resolves the config-chosen active batcher back out by name via the exported
// Select.

package batcher

import "fmt"

// DefaultName is the registry key the active batcher resolves to when
// webster.yaml's batcher: config key is absent or empty. It names the identity
// batcher (identity.go) — one card, one batch — webster's baseline execution
// policy.
const DefaultName = "identity"

// registry holds every library member registered so far, keyed by its own
// Name(). Populated exclusively by register calls from each library member's
// init(), never mutated after package initialization.
var registry = make(map[string]Batcher)

// register adds b to the registry under its own Name(). Called only from a
// library member's init(), so registration is complete before any lookup or
// Select call can observe the registry — package init() order guarantees this
// within a single package.
func register(b Batcher) {
	registry[b.Name()] = b
}

// lookup resolves name against the registry, reporting whether a batcher was
// registered under that exact name.
func lookup(name string) (Batcher, bool) {
	b, ok := registry[name]
	return b, ok
}

// Select resolves the active batcher by name: an empty name resolves to
// DefaultName (the identity batcher), and a registered name returns its own
// batcher. An unregistered name returns a wrapped "batcher:"-prefixed error
// naming the unknown key — this is the load-time error webstercli surfaces
// when webster.yaml's batcher: key names a batchifier that was never
// registered (batch 7 adds the config key; batch 9 wires Select at
// config-load).
func Select(name string) (Batcher, error) {
	if name == "" {
		name = DefaultName
	}
	b, ok := lookup(name)
	if !ok {
		return nil, fmt.Errorf("batcher: unknown batcher %q", name)
	}
	return b, nil
}
