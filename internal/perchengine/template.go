// template.go — perch.yaml template accessor.
//
// Provides the default YAML template for perch configuration, embedded
// directly from template.yaml at build time, mirroring reedengine's and
// shuttleengine's embed-and-accessor pattern. The three ephemeral-LLM-
// utility prompt templates (judge-circling, judge-milestone, triage) moved
// to internal/treadleengine along with the judge/triage machinery that
// renders them; perch never reads them directly.
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
