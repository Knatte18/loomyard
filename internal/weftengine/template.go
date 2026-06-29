// template.go — weft.yaml template accessor.
//
// Provides the default YAML template for weft configuration, embedded
// directly from template.yaml at build time. Weft templates use literal
// values with no ${env:...} markers.

package weftengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for weft configuration.
// The template uses literal values that are passed through unchanged by
// yamlengine.Resolve (no ${env:...} markers are present).
func ConfigTemplate() string {
	return configTemplate
}
