// template.go embeds the three ephemeral-LLM-utility prompt templates
// (judge-circling, judge-milestone, triage) directly from their .md files at
// build time, mirroring internal/burlerengine/template.go's
// //go:embed-directly-into-a-package-variable pattern; judge.go's fill
// helper renders them via internal/stencil.

package treadleengine

import _ "embed"

//go:embed judge-circling-template.md
var judgeCirclingTemplate []byte

//go:embed judge-milestone-template.md
var judgeMilestoneTemplate []byte

//go:embed triage-template.md
var triageTemplate []byte
