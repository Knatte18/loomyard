// keynames.go renders the full name of a claude.yaml key.

package claudeengine

import "github.com/Knatte18/loomyard/internal/configengine"

// configModule is the config module whose file is claude.yaml.
const configModule = "claude"

// keyName returns the full name of the claude.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}
