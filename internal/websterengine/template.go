// template.go — webster.yaml template accessor.
//
// ConfigTemplate provides the default YAML template for webster
// configuration, embedded directly from template.yaml at build time,
// mirroring builderengine's (and perchengine's/reedengine's) embed-and-
// accessor pattern.

package websterengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for webster
// configuration: the two role model-specs (master, recovery), the
// batchifier selection (batcher), and the numeric knobs the Master
// session's bracket verbs consult.
func ConfigTemplate() string {
	return configTemplate
}
