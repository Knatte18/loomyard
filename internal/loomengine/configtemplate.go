// configtemplate.go holds the default loom.yaml template for the config module.
//
// The template is embedded from template.yaml at build time.

package loomengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for loom's config module.
func ConfigTemplate() string {
	return configTemplate
}
