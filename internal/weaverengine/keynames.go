// keynames.go declares the weaver.yaml key paths the package's messages cite and renders a key's full name.

package weaverengine

import "github.com/Knatte18/loomyard/internal/configengine"

// configModule is the config module whose file is weaver.yaml.
const configModule = "weaver"

// The weaver.yaml key paths that messages cite.
const (
	ladderKey          = "ladder"
	fixerTimeoutMinKey = "fixer_timeout_min"
	verifyGateKey      = "verify_gate"
)

// keyName returns the full name of the weaver.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}
