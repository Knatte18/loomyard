// template.go — burler round prompt template accessor.
//
// Embeds the generic review-prompt-template.md asset directly at build time,
// mirroring internal/shuttleengine's ConfigTemplate accessor pattern. The
// template carries the round discipline (sequencing, cluster rules,
// fix-everything, review-file format, source-grounding, fixer-report,
// never-push) as static prose around nine top-level markers that
// prompt.go's composePrompt fills via internal/stencil.

package burlerengine

import _ "embed"

//go:embed review-prompt-template.md
var reviewPromptTemplate []byte
