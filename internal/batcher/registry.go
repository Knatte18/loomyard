// registry.go implements the package's name-keyed Batcher registry: library
// members self-register via their own init() (see identity.go), and webster
// resolves the config-chosen active batcher back out by name via the exported
// Select (added once register/lookup exist).

package batcher

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
