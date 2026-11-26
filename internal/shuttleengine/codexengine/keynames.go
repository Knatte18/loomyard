// keynames.go renders the full name of a codex.yaml key.

package codexengine

import "github.com/Knatte18/loomyard/internal/configengine"

// configModule is the config module whose file is codex.yaml.
const configModule = "codex"

// The codex.yaml key paths that more than one message cites.
const (
	sandboxModeConfigKey = "sandbox_mode"
	effortsKey           = "efforts"
	writableRootsKey     = "writable_roots"
)

// otherKeyName returns the full name of the key at path of a module other than codex.
func otherKeyName(module, path string) string {
	return configengine.KeyName(module, path)
}

// keyName returns the full name of the codex.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}
