// registry.go implements the package's name-keyed Batcher registry: library members self-register via their own init() (see identity.go),
// and webster resolves the config-chosen active batcher back out by name via the exported Select.

package batcher

import "fmt"

// DefaultName is the default batcher key when no config is specified.
const DefaultName = "identity"

// registry holds all registered batchers, keyed by Name().
var registry = make(map[string]Batcher)

// register adds b to the registry under its own Name().
func register(b Batcher) {
	registry[b.Name()] = b
}

// lookup resolves name against the registry, reporting whether a batcher was
// registered under that exact name.
func lookup(name string) (Batcher, bool) {
	b, ok := registry[name]
	return b, ok
}

// Select resolves the active batcher by name, defaulting to DefaultName if empty.
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
