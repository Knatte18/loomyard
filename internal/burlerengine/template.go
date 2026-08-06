// template.go — burler round prompt template accessor.
//
// Embeds the four round prompt assets directly at build time, mirroring internal/shuttleengine's ConfigTemplate accessor pattern.
// The orchestrator (round-orchestrator-template.md) owns ordering — the two-jobs framing, the BLOCKING sequencing rule, and the ordered list of the three instruction file paths — while each instruction file carries exactly one step's rules as static prose around its own top-level markers: instruction 1 (explore) the target/fasit/rubric/tool-use rules, instruction 2 (review) the cluster rules and the review-file format, and instruction 3 (fix) the fix-everything rule and the fixer-report rule.
// prompt.go's composePrompt fills all four via internal/stencil.

package burlerengine

import _ "embed"

//go:embed round-orchestrator-template.md
var roundOrchestratorTemplate []byte

//go:embed instruction-1-explore-template.md
var instruction1Template []byte

//go:embed instruction-2-review-template.md
var instruction2Template []byte

//go:embed instruction-3-fix-template.md
var instruction3Template []byte
