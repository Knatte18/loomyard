// template.go — perch.yaml template accessor.
//
// Provides the default YAML template for perch configuration, embedded
// directly from template.yaml at build time, mirroring muxengine's and
// shuttleengine's embed-and-accessor pattern.

package perchengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for perch configuration:
// judge_model, judge_effort (empty = provider default), and round_caps, the
// default milestone cap ladder.
func ConfigTemplate() string {
	return configTemplate
}
