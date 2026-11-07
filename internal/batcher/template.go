// template.go holds the default batcher.yaml template and the accessor that returns it.

package batcher

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default batcher.yaml template, embedded from template.yaml at build time.
func ConfigTemplate() string {
	return configTemplate
}
