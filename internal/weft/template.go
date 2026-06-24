// template.go — weft.yaml template generator.
//
// Provides the default YAML template for weft configuration via embedded
// template.yaml file. Weft templates use literal values (no env-marker substitution).

package weft

import _ "embed"

// configTemplate is the embedded YAML template for weft configuration.
// It contains a literal pathspec value with no environment variable placeholders.
//
//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for weft configuration.
// The template uses literal values that are passed through unchanged by
// yamlengine.Resolve (no ${env:...} markers are present).
func ConfigTemplate() string {
	return configTemplate
}
