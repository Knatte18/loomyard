// template.go — perch.yaml template accessor, plus the judge/triage prompt
// template embeds.
//
// Provides the default YAML template for perch configuration, embedded
// directly from template.yaml at build time, mirroring muxengine's and
// shuttleengine's embed-and-accessor pattern. It also embeds the three
// ephemeral-LLM-utility prompt templates (judge-circling, judge-milestone,
// triage), mirroring internal/burlerengine/template.go's
// //go:embed-directly-into-a-package-variable pattern; judge.go's fill
// helper renders them via internal/stencil.
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

//go:embed judge-circling-template.md
var judgeCirclingTemplate []byte

//go:embed judge-milestone-template.md
var judgeMilestoneTemplate []byte

//go:embed triage-template.md
var triageTemplate []byte
