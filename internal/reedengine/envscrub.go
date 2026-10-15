// envscrub.go implements the module's env-hygiene chokepoint: EnvScrub strips the environment keys the registered engines declare before a tmux server spawns, so that no child inherits a stale session identity from its own launcher.

package reedengine

import "strings"

// EnvScrub is the set of environment keys a spawned process must not inherit.
// The zero value strips nothing; reed refuses to build an Engine over it.
type EnvScrub struct {
	// Names are exact keys.
	Names []string
	// Prefixes strip every key that starts with one of them.
	Prefixes []string
}

// Apply returns environ without every entry whose key equals a name or starts with a prefix, the key being the part before "=".
// It also returns the stripped keys in environ order.
func (s EnvScrub) Apply(environ []string) (clean []string, stripped []string) {
	clean = []string{}
	stripped = []string{}
	for _, entry := range environ {
		key := strings.SplitN(entry, "=", 2)[0]
		if s.matches(key) {
			stripped = append(stripped, key)
			continue
		}
		clean = append(clean, entry)
	}
	return clean, stripped
}

// matches reports whether key is one of the scrub's names or starts with one of its prefixes.
func (s EnvScrub) matches(key string) bool {
	for _, name := range s.Names {
		if key == name {
			return true
		}
	}
	for _, prefix := range s.Prefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
