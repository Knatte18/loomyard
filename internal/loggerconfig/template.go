// template.go — logger.yaml template accessor.
//
// ConfigTemplate provides the default YAML template for the logger's trace-retention configuration,
// embedded directly from template.yaml at build time, matching batcher's own embed-and-accessor
// pattern.

package loggerconfig

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for logger configuration: the two trace-retention
// bounds.
func ConfigTemplate() string {
	return configTemplate
}
