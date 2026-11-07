// template.go — webster.yaml template accessor.
//
// It embeds template.yaml at build time.

package websterengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for webster configuration.
// The template sets the role model-specs and the Master session configuration.
func ConfigTemplate() string {
	return configTemplate
}
