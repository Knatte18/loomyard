// envscrub.go defines EnvScrub, the filter that removes environment keys before a tmux server spawns.
// It strips the keys the registered engines declare, so no child inherits a stale session identity from its launcher.

package reedengine

import "strings"

// EnvScrub is the set of environment keys a spawned process must not inherit.
// The zero value strips nothing, and reed refuses to build an engine over it.
type EnvScrub struct {
	// Names lists keys stripped only on an exact match.
	Names []string
	// Prefixes lists key prefixes; every key starting with one of them is stripped.
	Prefixes []string
}

// Apply returns environ without the entries whose key matches a name or starts with a prefix.
// An entry's key is the part before its first "=".
// It also returns the stripped keys, in environ order.
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
