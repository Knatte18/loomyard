// template.go — builder.yaml template accessor.
//
// Provides the default YAML template for builder configuration, embedded
// directly from template.yaml at build time, mirroring perchengine's and
// muxengine's embed-and-accessor pattern.

package builderengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for builder
// configuration: the four role model-specs (orchestrator, implementer,
// implementer_oversized, recovery) and the numeric knobs the batch loop and
// its validation gate consult.
func ConfigTemplate() string {
	return configTemplate
}
