// template.go — batcher.yaml template accessor.
//
// ConfigTemplate provides the default YAML template for batcher configuration, embedded directly
// from template.yaml at build time, matching reedengine's and websterengine's own
// embed-and-accessor pattern.

package batcher

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for batcher configuration: the name of the
// active batchifier.
func ConfigTemplate() string {
	return configTemplate
}
