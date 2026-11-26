// keynames.go renders the full name of a webster.yaml key, or of the shuttle and language keys webster's refusals cite.

package websterengine

import "github.com/Knatte18/loomyard/internal/configengine"

// configModule is the config module whose file is webster.yaml.
const configModule = "webster"

// keyName returns the full name of the webster.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}

// shuttleEngineKeyName returns the full name of the shuttle.yaml key choosing the default engine.
func shuttleEngineKeyName() string {
	return configengine.KeyName("shuttle", "engine")
}

// renameToolsKeyName returns the full name of the language.yaml key listing the hub's rename tools.
func renameToolsKeyName() string {
	return configengine.KeyName("language", "rename_tools")
}
