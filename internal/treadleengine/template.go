// template.go embeds the four ephemeral-LLM-utility prompt templates
// (judge-circling, judge-milestone, triage, targeting) directly from their
// .md files at build time, mirroring internal/burlerengine/template.go's
// //go:embed-directly-into-a-package-variable pattern; judge.go's and
// targeting.go's fill helpers render them via internal/stencil.

package treadleengine

import _ "embed"

//go:embed judge-circling-template.md
var judgeCirclingTemplate []byte

//go:embed judge-milestone-template.md
var judgeMilestoneTemplate []byte

//go:embed triage-template.md
var triageTemplate []byte

//go:embed targeting-template.md
var targetingTemplate []byte
