// template.go embeds the default logger.yaml template from template.yaml at build time.
//
// ConfigTemplate returns the template as a string.

package loggerconfig

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for logger configuration, which sets the trace-retention bounds.
func ConfigTemplate() string {
	return configTemplate
}
